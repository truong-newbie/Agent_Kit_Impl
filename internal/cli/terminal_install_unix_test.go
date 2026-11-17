//go:build !windows

package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter/codex"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/installer"
)

func buildStubTarGz(t *testing.T, ver string, corrupt bool) ([]byte, string) {
	t.Helper()
	if corrupt {
		raw := []byte("not-a-valid-gzip-archive")
		sum := sha256.Sum256(raw)
		return raw, hex.EncodeToString(sum[:])
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	script := fmt.Sprintf("#!/bin/sh\necho '{\"version\":\"%s\"}'\n", ver)
	hdr := &tar.Header{
		Name: "dtkit",
		Mode: 0755,
		Size: int64(len(script)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(script)); err != nil {
		t.Fatal(err)
	}
	_ = tw.Close()
	_ = gz.Close()
	sum := sha256.Sum256(buf.Bytes())
	return buf.Bytes(), hex.EncodeToString(sum[:])
}

func TestTerminalInstallScriptUnixAndHookUpgradeSafety(t *testing.T) {
	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available on PATH")
	}

	archSuffix := runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz"
	if runtime.GOOS == "darwin" && runtime.GOARCH != "arm64" {
		t.Skip("unsupported darwin arch for prebuilt archive naming")
	}

	var mu sync.Mutex
	archives := map[string][]byte{}
	sumsBody := ""

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path == "/SHA256SUMS" {
			_, _ = w.Write([]byte(sumsBody))
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		if data, ok := archives[name]; ok {
			_, _ = w.Write(data)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	binDir := filepath.Join(base, "bin")
	destBin := filepath.Join(binDir, "dtkit")

	runInstaller := func(ver string, extraEnv ...string) (string, error) {
		cmd := exec.Command("bash", scriptPath, "--version", ver, "--bin-dir", binDir, "--base-url", srv.URL)
		cmd.Env = append(os.Environ(), extraEnv...)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	// 1. Fresh install 0.4.0-rc.1 with valid checksum succeeds cleanly.
	a1Name := "dtkit_0.4.0-rc.1_" + archSuffix
	a1Bytes, a1Hash := buildStubTarGz(t, "0.4.0-rc.1", false)
	mu.Lock()
	archives[a1Name] = a1Bytes
	sumsBody = fmt.Sprintf("%s  %s\n", a1Hash, a1Name)
	mu.Unlock()

	if out, err := runInstaller("0.4.0-rc.1"); err != nil {
		t.Fatalf("fresh install failed: %v\n%s", err, out)
	}
	verOut, err := exec.Command(destBin, "version", "--json").CombinedOutput()
	if err != nil || !strings.Contains(string(verOut), `"version":"0.4.0-rc.1"`) {
		t.Fatalf("installed stub verification failed: %v %s", err, verOut)
	}

	// 2. Enable Codex hook pointing at destBin, then test failing upgrade preserves destBin & hook validity.
	proj := filepath.Join(base, "proj")
	home := filepath.Join(base, "home")
	codexHome := filepath.Join(base, "codex-home")
	for _, d := range []string{proj, home, codexHome} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	pkg, src, err := catalog.Engineer()
	if err != nil {
		t.Fatal(err)
	}
	arts, _ := codex.Render(pkg, src, "project")
	projRoots := map[string]string{"project": proj}
	globRoots := map[string]string{"user-home": home, "codex-home": codexHome}
	if _, err := installer.FreshInstall("project", projRoots, pkg, src, arts, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := installer.EnableHook("project", "session-context", projRoots, globRoots, destBin, false, time.Now(), nil); err != nil {
		t.Fatalf("EnableHook failed: %v", err)
	}

	a2Name := "dtkit_0.4.0-rc.2_" + archSuffix
	a2Bytes, a2Hash := buildStubTarGz(t, "0.4.0-rc.2", false)

	// Failure A: network 404 (missing archive on server).
	mu.Lock()
	delete(archives, a2Name)
	sumsBody = fmt.Sprintf("%s  %s\n", a2Hash, a2Name)
	mu.Unlock()
	if _, err := runInstaller("0.4.0-rc.2"); err == nil {
		t.Fatal("expected install failure on 404 download")
	}

	// Failure B: checksum mismatch -> aborts before extract, old binary preserved.
	mu.Lock()
	archives[a2Name] = a2Bytes
	sumsBody = fmt.Sprintf("%s  %s\n", strings.Repeat("0", 64), a2Name)
	mu.Unlock()
	if _, err := runInstaller("0.4.0-rc.2"); err == nil {
		t.Fatal("expected install failure on SHA256 mismatch")
	}

	// Failure C: missing SHA256SUMS entry.
	mu.Lock()
	sumsBody = fmt.Sprintf("%s  other-file.tar.gz\n", a2Hash)
	mu.Unlock()
	if _, err := runInstaller("0.4.0-rc.2"); err == nil {
		t.Fatal("expected install failure on missing SHA256SUMS entry")
	}

	// Failure D: duplicate SHA256SUMS entry.
	mu.Lock()
	sumsBody = fmt.Sprintf("%s  %s\n%s  %s\n", a2Hash, a2Name, a2Hash, a2Name)
	mu.Unlock()
	if _, err := runInstaller("0.4.0-rc.2"); err == nil {
		t.Fatal("expected install failure on duplicate SHA256SUMS entries")
	}

	// Failure E: unsupported OS/architecture (e.g. Intel Mac / ARM Linux).
	if out, err := runInstaller("0.4.0-rc.2", "DTKIT_TEST_OS=Darwin", "DTKIT_TEST_ARCH=x86_64"); err == nil || !strings.Contains(out, "unsupported target") {
		t.Fatalf("expected unsupported target error on Darwin x86_64, got err=%v out=%s", err, out)
	}

	// Failure F: corrupt archive bytes matching checksum -> tar fails, old binary still intact.
	badName := "dtkit_0.4.0-rc.3_" + archSuffix
	badBytes, badHash := buildStubTarGz(t, "0.4.0-rc.3", true)
	mu.Lock()
	archives[badName] = badBytes
	sumsBody = fmt.Sprintf("%s  %s\n", badHash, badName)
	mu.Unlock()
	if _, err := runInstaller("0.4.0-rc.3"); err == nil {
		t.Fatal("expected install failure on corrupt tar archive")
	}

	// Confirm original 0.4.0-rc.1 binary & active Codex hook remain intact after all failures!
	verStill, err := exec.Command(destBin, "version", "--json").CombinedOutput()
	if err != nil || !strings.Contains(string(verStill), `"version":"0.4.0-rc.1"`) {
		t.Fatalf("previous binary corrupted after failed upgrades: %v %s", err, verStill)
	}
	inspBefore, err := installer.InspectHooks("project", projRoots)
	if err != nil || len(inspBefore) != 1 || inspBefore[0].Status != "configured_pending_trust" {
		t.Fatalf("hook broken after failed CLI upgrade: %+v err=%v", inspBefore, err)
	}

	// 3. Valid upgrade 0.4.0-rc.1 -> 0.4.0-rc.2 preserves active hook path & reports new version.
	mu.Lock()
	archives[a2Name] = a2Bytes
	sumsBody = fmt.Sprintf("%s  %s\n", a2Hash, a2Name)
	mu.Unlock()
	if out, err := runInstaller("0.4.0-rc.2"); err != nil {
		t.Fatalf("valid upgrade failed: %v\n%s", err, out)
	}
	verUp, err := exec.Command(destBin, "version", "--json").CombinedOutput()
	if err != nil || !strings.Contains(string(verUp), `"version":"0.4.0-rc.2"`) {
		t.Fatalf("upgraded binary mismatch: %v %s", err, verUp)
	}
	inspAfter, err := installer.InspectHooks("project", projRoots)
	if err != nil || len(inspAfter) != 1 || inspAfter[0].Status != "configured_pending_trust" {
		t.Fatalf("hook should remain valid after in-place binary upgrade: %+v err=%v", inspAfter, err)
	}
}
