package dist

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
)

func buildCustomKitArchiveBytes(t *testing.T, id, version, minCLI string) (catalog.Package, []byte, string, string) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, err := CreateKit(base, id, "Kit "+id)
	if err != nil {
		t.Fatal(err)
	}
	pkgPath := filepath.Join(created.Dir, "package.json")
	rawPkg, err := os.ReadFile(pkgPath)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(rawPkg, &m); err != nil {
		t.Fatal(err)
	}
	m["version"] = version
	if minCLI != "" {
		m["minCliVersion"] = minCLI
	}
	updated, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pkgPath, append(updated, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	packRes, err := Pack(created.Dir, filepath.Join(base, id+"-"+version+".tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(packRes.ArchivePath)
	if err != nil {
		t.Fatal(err)
	}
	pkg, _, _, _, err := ReadArchiveBytes(raw, packRes.ArchiveSHA256)
	if err != nil {
		t.Fatal(err)
	}
	return pkg, raw, packRes.ArchiveSHA256, packRes.PackageDigest
}

func TestFetchRemoteBytesTimeoutCancellationRedirectsAndRedaction(t *testing.T) {
	// 1. Slow server must abort within bounded client timeout / context cancellation.
	slowSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
			_, _ = w.Write([]byte("late"))
		case <-r.Context().Done():
		}
	}))
	defer slowSrv.Close()

	shortClient := slowSrv.Client()
	shortClient.Timeout = 60 * time.Millisecond
	if _, err := FetchRemoteBytes(context.Background(), shortClient, slowSrv.URL+"/slow.json?token=supersecret99", MaxIndexBytes); err == nil {
		t.Fatalf("expected timeout error from slow TLS server")
	} else if strings.Contains(err.Error(), "supersecret99") {
		t.Fatalf("timeout error leaked URL query secret: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := FetchRemoteBytes(ctx, slowSrv.Client(), slowSrv.URL+"/slow.json?api_key=leakcheck1", MaxIndexBytes); err == nil {
		t.Fatalf("expected canceled context error")
	} else if strings.Contains(err.Error(), "leakcheck1") {
		t.Fatalf("canceled context error leaked query secret: %v", err)
	}

	// 2. Credential userinfo and query parameters never leak in errors.
	if err := ValidateRemoteURL("https://user:s3cr3tpass@example.com/index.json?token=qsecret"); err == nil {
		t.Fatalf("expected credentials_in_url error")
	} else if strings.Contains(err.Error(), "s3cr3tpass") || strings.Contains(err.Error(), "qsecret") {
		t.Fatalf("URL validation leaked credentials/query: %v", err)
	}

	// 3. TLS server with custom CA (srv.Client(), no InsecureSkipVerify) succeeds across valid HTTPS redirect.
	var hops atomic.Int32
	var tlsSrv *httptest.Server
	tlsSrv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok-redirect":
			http.Redirect(w, r, tlsSrv.URL+"/target", http.StatusFound)
		case "/target":
			_, _ = w.Write([]byte("hello-tls"))
		case "/loop":
			n := hops.Add(1)
			http.Redirect(w, r, fmt.Sprintf("%s/loop?n=%d", tlsSrv.URL, n), http.StatusFound)
		case "/downgrade-ext":
			http.Redirect(w, r, "http://example.com/evil.tar.gz", http.StatusFound)
		case "/downgrade-local":
			http.Redirect(w, r, "http://127.0.0.1:1/local.tar.gz", http.StatusFound)
		case "/file-scheme":
			http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
		case "/secret-404":
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer tlsSrv.Close()

	// Unverified default client without tlsSrv's RootCAs must fail TLS verification (never bypassed).
	if _, err := FetchRemoteBytes(context.Background(), DefaultHTTPClient(), tlsSrv.URL+"/target", MaxIndexBytes); err == nil {
		t.Fatalf("expected untrusted TLS cert failure when custom CA is not configured")
	}
	// Custom CA client succeeds cleanly.
	got, err := FetchRemoteBytes(context.Background(), tlsSrv.Client(), tlsSrv.URL+"/ok-redirect", MaxIndexBytes)
	if err != nil || string(got) != "hello-tls" {
		t.Fatalf("expected valid TLS redirect with custom CA to succeed, got %q err=%v", string(got), err)
	}
	// HTTP 404 redacts query parameters.
	if _, err := FetchRemoteBytes(context.Background(), tlsSrv.Client(), tlsSrv.URL+"/secret-404?sig=dontleakme", MaxIndexBytes); err == nil || strings.Contains(err.Error(), "dontleakme") {
		t.Fatalf("expected redacted 404 error without query secret, got %v", err)
	}
	// Redirect limits & unsafe schemes/downgrades blocked.
	for _, tc := range []string{"/loop", "/downgrade-ext", "/downgrade-local", "/file-scheme"} {
		if _, err := FetchRemoteBytes(context.Background(), tlsSrv.Client(), tlsSrv.URL+tc, MaxIndexBytes); err == nil {
			t.Fatalf("expected redirect policy rejection for %s", tc)
		}
	}

	// 4. Cross-host redirect strips Authorization and Cookie headers.
	var gotAuth, gotCookie string
	destSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCookie = r.Header.Get("Cookie")
		_, _ = w.Write([]byte("dest-ok"))
	}))
	defer destSrv.Close()
	srcSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destSrv.URL+"/final", http.StatusFound)
	}))
	defer srcSrv.Close()

	// Combine RootCAs from destSrv and srcSrv using srcSrv's transport cloned cleanly? Note stdlib NewTLSServer creates separate certs;
	// Verify CheckRedirect header stripping directly via loopback HTTP redirect across two localhost ports.
	var plainAuth, plainCookie string
	plainDest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		plainAuth = r.Header.Get("Authorization")
		plainCookie = r.Header.Get("Cookie")
		_, _ = w.Write([]byte("ok"))
	}))
	defer plainDest.Close()
	plainSrc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plainDest.URL+"/final", http.StatusFound)
	}))
	defer plainSrc.Close()

	reqClient := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &roundTripHeaderInjector{
			base:   http.DefaultTransport,
			auth:   "Bearer topsecret",
			cookie: "session=abc",
		},
	}
	if _, err := FetchRemoteBytes(context.Background(), reqClient, plainSrc.URL+"/start", MaxIndexBytes); err != nil {
		t.Fatalf("cross-port loopback redirect failed: %v", err)
	}
	if plainAuth != "" || plainCookie != "" {
		t.Fatalf("expected Authorization & Cookie stripped on cross-host redirect, got auth=%q cookie=%q", plainAuth, plainCookie)
	}
	_ = gotAuth
	_ = gotCookie
}

type roundTripHeaderInjector struct {
	base   http.RoundTripper
	auth   string
	cookie string
}

func (rt *roundTripHeaderInjector) RoundTrip(r *http.Request) (*http.Response, error) {
	if len(r.Header.Values("Authorization")) == 0 && r.URL.Path == "/start" {
		r.Header.Set("Authorization", rt.auth)
		r.Header.Set("Cookie", rt.cookie)
	}
	return rt.base.RoundTrip(r)
}

func TestRegistryIndexStrictValidationAndDeterministicSelection(t *testing.T) {
	validSHA := strings.Repeat("a", 64)
	validBase := "https://registry.example.com/kits/index.json"

	invalidPayloads := map[string]string{
		"duplicate_key":       fmt.Sprintf(`{"schemaVersion":1,"schemaVersion":1,"packages":[]}`),
		"duplicate_entry_key": fmt.Sprintf(`{"schemaVersion":1,"packages":[{"id":"a","id":"b","version":"0.1.0","url":"a.tar.gz","archiveSha256":%q}]}`, validSHA),
		"unknown_field":       fmt.Sprintf(`{"schemaVersion":1,"extra":true,"packages":[]}`),
		"null_packages":       `{"schemaVersion":1,"packages":null}`,
		"trailing_garbage":    `{"schemaVersion":1,"packages":[]} {}`,
		"bad_id":              fmt.Sprintf(`{"schemaVersion":1,"packages":[{"id":"Bad_ID","version":"0.1.0","url":"a.tar.gz","archiveSha256":%q}]}`, validSHA),
		"bad_semver":          fmt.Sprintf(`{"schemaVersion":1,"packages":[{"id":"eng","version":"01.0.0","url":"a.tar.gz","archiveSha256":%q}]}`, validSHA),
		"plain_http":          fmt.Sprintf(`{"schemaVersion":1,"packages":[{"id":"eng","version":"0.1.0","url":"http://example.com/a.tar.gz","archiveSha256":%q}]}`, validSHA),
		"bad_sha":             `{"schemaVersion":1,"packages":[{"id":"eng","version":"0.1.0","url":"a.tar.gz","archiveSha256":"xyz"}]}`,
		"bad_min_cli":         fmt.Sprintf(`{"schemaVersion":1,"packages":[{"id":"eng","version":"0.1.0","url":"a.tar.gz","archiveSha256":%q,"minCliVersion":"0.4"}]}`, validSHA),
		"duplicate_id_ver":    fmt.Sprintf(`{"schemaVersion":1,"packages":[{"id":"eng","version":"0.1.0","url":"a.tar.gz","archiveSha256":%q},{"id":"eng","version":"0.1.0","url":"b.tar.gz","archiveSha256":%q}]}`, validSHA, strings.Repeat("b", 64)),
	}
	for name, body := range invalidPayloads {
		if _, err := ParseRegistryIndex([]byte(body), validBase); err == nil {
			t.Fatalf("expected invalid index rejection for %s", name)
		}
	}

	// Over MaxIndexEntries rejected.
	entries := make([]IndexEntry, 0, MaxIndexEntries+1)
	for i := 0; i < MaxIndexEntries+1; i++ {
		entries = append(entries, IndexEntry{
			ID:            "eng",
			Version:       fmt.Sprintf("0.0.%d", i),
			URL:           "a.tar.gz",
			ArchiveSHA256: validSHA,
		})
	}
	tooMany, _ := json.Marshal(RegistryIndex{SchemaVersion: 1, Packages: entries})
	if _, err := ParseRegistryIndex(tooMany, validBase); err == nil {
		t.Fatalf("expected index exceeding MaxIndexEntries=%d to fail", MaxIndexEntries)
	}

	// Deterministic selection picks highest valid semver regardless of order.
	idxBytes, _ := json.Marshal(RegistryIndex{
		SchemaVersion: 1,
		Packages: []IndexEntry{
			{ID: "eng", Version: "0.2.0", URL: "v020.tar.gz", ArchiveSHA256: validSHA},
			{ID: "eng", Version: "0.10.0", URL: "v010.tar.gz", ArchiveSHA256: strings.Repeat("b", 64)},
			{ID: "eng", Version: "0.1.9", URL: "v019.tar.gz", ArchiveSHA256: strings.Repeat("c", 64)},
		},
	})
	parsedIdx, err := ParseRegistryIndex(idxBytes, validBase)
	if err != nil {
		t.Fatalf("valid index failed: %v", err)
	}
	best, err := selectIndexEntry(parsedIdx.Packages, "eng", "")
	if err != nil || best.Version != "0.10.0" || best.URL != "https://registry.example.com/kits/v010.tar.gz" {
		t.Fatalf("expected highest semver 0.10.0 with resolved URL, got %+v err=%v", best, err)
	}
}

func TestUnifiedMinCLIVersionAndOfflineParity(t *testing.T) {
	// Verify documented prerelease/dev comparison semantics.
	for _, tc := range []struct {
		cli, min string
		wantOK   bool
		wantErr  bool
	}{
		{"0.5.0-dev", "0.5.0", true, false},
		{"0.5.0-rc.1", "0.5.0", true, false},
		{"0.5.0", "0.5.0", true, false},
		{"0.4.9", "0.5.0", false, false},
		{"0.4.0-dev", "0.5.0", false, false},
		{"dev", "0.5.0", false, true},
		{"0.5.0-bad space", "0.5.0", false, true},
	} {
		ok, err := SatisfiesMinCLI(tc.cli, tc.min)
		if (err != nil) != tc.wantErr || (err == nil && ok != tc.wantOK) {
			t.Fatalf("SatisfiesMinCLI(%q, %q) = (%v, %v), want (%v, err=%v)", tc.cli, tc.min, ok, err, tc.wantOK, tc.wantErr)
		}
	}

	// Build archive with no manifest minCliVersion, served via index specifying minCliVersion=0.5.0.
	_, archNoMin, shaNoMin, digNoMin := buildCustomKitArchiveBytes(t, "gated-kit", "0.1.0", "")
	// Build archive carrying manifest minCliVersion=0.6.0 directly in package.json.
	_, archWithMin, shaWithMin, _ := buildCustomKitArchiveBytes(t, "manifest-gated", "0.1.0", "0.6.0")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.json":
			_ = json.NewEncoder(w).Encode(RegistryIndex{
				SchemaVersion: 1,
				Packages: []IndexEntry{{
					ID:            "gated-kit",
					Version:       "0.1.0",
					URL:           "/gated.tar.gz",
					ArchiveSHA256: shaNoMin,
					PackageDigest: digNoMin,
					MinCLIVersion: "0.5.0",
				}},
			})
		case "/gated.tar.gz":
			_, _ = w.Write(archNoMin)
		case "/manifest-gated-0.1.0.tar.gz":
			_, _ = w.Write(archWithMin)
		}
	}))
	defer srv.Close()

	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cache := Cache{Root: filepath.Join(base, "cache")}

	// 1. CLI 0.5.0-rc.1 resolves and persists BOTH blob and <sha>.meta.json.
	if _, err := ResolveRemote(context.Background(), srv.Client(), cache, "gated-kit", "0.1.0", srv.URL+"/index.json", "", "0.5.0-rc.1", false, CachePersist); err != nil {
		t.Fatalf("expected 0.5.0-rc.1 to satisfy minCliVersion 0.5.0: %v", err)
	}

	// 2. Older CLI 0.4.0-dev CANNOT bypass online index requirement using --offline cache!
	if _, err := ResolveRemote(context.Background(), nil, cache, "gated-kit", "0.1.0", "", shaNoMin, "0.4.0-dev", true, CacheReadOnly); apperr.ExitCodeOf(err) != apperr.ExitUsage || !strings.Contains(err.Error(), "requires dtkit >= 0.5.0") {
		t.Fatalf("expected offline cli_upgrade_required for cached 0.5.0 package on 0.4.0-dev, got %v", err)
	}

	// 3. Direct archive URL carries manifest minCliVersion 0.6.0: rejected for 0.5.0, accepted & cached for 0.6.0-dev, then rejected offline for 0.5.0.
	if _, err := ResolveRemote(context.Background(), srv.Client(), cache, "manifest-gated", "0.1.0", srv.URL+"/manifest-gated-0.1.0.tar.gz", "", "0.5.0", false, CacheReadOnly); err == nil || !strings.Contains(err.Error(), "requires dtkit >= 0.6.0") {
		t.Fatalf("expected direct archive minCliVersion enforcement, got %v", err)
	}
	if _, err := ResolveRemote(context.Background(), srv.Client(), cache, "manifest-gated", "0.1.0", srv.URL+"/manifest-gated-0.1.0.tar.gz", "", "0.6.0-dev", false, CachePersist); err != nil {
		t.Fatalf("expected 0.6.0-dev direct archive install to succeed, got %v", err)
	}
	if _, err := ResolveRemote(context.Background(), nil, cache, "manifest-gated", "0.1.0", "", shaWithMin, "0.5.0", true, CacheReadOnly); err == nil || !strings.Contains(err.Error(), "requires dtkit >= 0.6.0") {
		t.Fatalf("expected offline direct archive minCliVersion rejection, got %v", err)
	}

	// 4. Corrupt metadata file must fail closed as cache_corrupted.
	metaPath, err := cache.metaPath(shaNoMin)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metaPath, []byte(`{"schemaVersion":1,"archiveSha256":"0000"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveRemote(context.Background(), nil, cache, "gated-kit", "0.1.0", "", shaNoMin, "0.5.0", true, CacheReadOnly); apperr.ExitCodeOf(err) != apperr.ExitConflict {
		t.Fatalf("expected cache_corrupted conflict on tampered meta.json, got %v", err)
	}
}

func TestCachePutConcurrentGoroutinesAndProcessesIdempotent(t *testing.T) {
	if os.Getenv("DTKIT_TEST_CACHE_CHILD") == "1" {
		cache := Cache{Root: os.Getenv("DTKIT_TEST_CACHE_ROOT")}
		raw, err := os.ReadFile(os.Getenv("DTKIT_TEST_CACHE_BLOB"))
		if err != nil {
			t.Fatal(err)
		}
		dig, _, err := cache.PutArchive(raw, &CacheMetadata{
			SchemaVersion: 1,
			ArchiveSHA256: sha256Hex(raw),
			MinCLIVersion: "0.2.0",
		})
		if err != nil || dig != sha256Hex(raw) {
			t.Fatalf("child PutArchive failed: dig=%s err=%v", dig, err)
		}
		return
	}

	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, raw, sha, _ := buildCustomKitArchiveBytes(t, "race-kit", "0.1.0", "0.2.0")
	cache := Cache{Root: filepath.Join(base, "cache")}

	// 1. 16 goroutines racing under -race to write the same digest + metadata.
	const n = 16
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			dig, _, pErr := cache.PutArchive(raw, &CacheMetadata{
				SchemaVersion: 1,
				ArchiveSHA256: sha,
				MinCLIVersion: "0.2.0",
			})
			if pErr != nil {
				errs <- pErr
			} else if dig != sha {
				errs <- fmt.Errorf("digest mismatch %s != %s", dig, sha)
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent goroutine PutArchive error: %v", err)
	}

	// 2. Concurrent OS child processes racing into fresh cache root.
	procCacheRoot := filepath.Join(base, "proc-cache")
	blobPath := filepath.Join(base, "blob.tar.gz")
	if err := os.WriteFile(blobPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var procs []*exec.Cmd
	for i := 0; i < 4; i++ {
		cmd := exec.Command(exe, "-test.run=^TestCachePutConcurrentGoroutinesAndProcessesIdempotent$")
		cmd.Env = append(os.Environ(),
			"DTKIT_TEST_CACHE_CHILD=1",
			"DTKIT_TEST_CACHE_ROOT="+procCacheRoot,
			"DTKIT_TEST_CACHE_BLOB="+blobPath,
		)
		procs = append(procs, cmd)
	}
	for _, cmd := range procs {
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
	}
	for _, cmd := range procs {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("concurrent child cache writer failed: %v", err)
		}
	}

	gotBytes, found, err := Cache{Root: procCacheRoot}.Get(sha)
	if err != nil || !found || !bytes.Equal(gotBytes, raw) {
		t.Fatalf("multi-process cache verify failed: found=%v len=%d err=%v", found, len(gotBytes), err)
	}
}

func TestArchiveHeaderFloodAndDuplicateEntriesRejected(t *testing.T) {
	// 1. Header-only directory flood exceeding MaxArchiveEntries.
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for i := 0; i < MaxArchiveEntries+10; i++ {
		_ = tw.WriteHeader(&tar.Header{
			Name:     fmt.Sprintf("dirs/d%04d/", i),
			Typeflag: tar.TypeDir,
			Mode:     0755,
		})
	}
	_ = tw.Close()
	_ = gz.Close()

	if _, _, _, _, err := ReadArchiveBytes(buf.Bytes(), ""); err == nil || !strings.Contains(err.Error(), "entry limit") {
		t.Fatalf("expected archive_entry_count_limit rejection for header flood, got %v", err)
	}

	// 2. Duplicate tar entry for same file path rejected.
	var dupBuf bytes.Buffer
	gz2 := gzip.NewWriter(&dupBuf)
	tw2 := tar.NewWriter(gz2)
	pkgBytes := []byte(`{"schemaVersion":1,"id":"dup","name":"Dup","version":"0.1.0","description":"d","runtimeTargets":["codex"],"scopes":["project"],"requiresCapabilities":["instructions"],"packageDependencies":[],"components":[]}`)
	for i := 0; i < 2; i++ {
		_ = tw2.WriteHeader(&tar.Header{Name: "package.json", Typeflag: tar.TypeReg, Size: int64(len(pkgBytes)), Mode: 0644})
		_, _ = tw2.Write(pkgBytes)
	}
	_ = tw2.Close()
	_ = gz2.Close()
	if _, _, _, _, err := ReadArchiveBytes(dupBuf.Bytes(), ""); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate tar entry rejection, got %v", err)
	}
	_ = sha256.Sum256
	_ = hex.EncodeToString
}
