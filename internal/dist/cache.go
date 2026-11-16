package dist

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

const (
	// DefaultStalePartAge is the minimum idle age required before unowned partial download files are pruned.
	DefaultStalePartAge = 15 * time.Minute
	// DefaultHTTPTimeout caps total duration of a single HTTP request (headers + body).
	DefaultHTTPTimeout = 30 * time.Second
	// MaxRedirects limits HTTP redirect hops per remote fetch.
	MaxRedirects = 10
	// MaxMetadataBytes bounds cached sidecar metadata files.
	MaxMetadataBytes = 64 * 1024
)

type downloadLock struct {
	PID int `json:"pid"`
}

// CacheMetadata persists verified CLI compatibility floor and digests alongside a cached archive.
type CacheMetadata struct {
	SchemaVersion int    `json:"schemaVersion"`
	ArchiveSHA256 string `json:"archiveSha256"`
	PackageDigest string `json:"packageDigest,omitempty"`
	MinCLIVersion string `json:"minCliVersion,omitempty"`
}

// CacheHooks allows tests to observe cache write and cleanup boundaries without side effects.
type CacheHooks struct {
	OnWrite func(digest string, wrote bool)
	OnClean func(path string)
}

// Cache stores verified package archives keyed by SHA-256 hex digest.
type Cache struct {
	Root  string
	Hooks *CacheHooks
}

// DefaultHTTPClient returns an HTTP client configured with finite request timeout.
func DefaultHTTPClient() *http.Client {
	return &http.Client{Timeout: DefaultHTTPTimeout}
}

func OpenCache(env paths.Environment) (Cache, error) {
	if env.UserCacheDir != nil {
		if base, err := env.UserCacheDir(); err == nil && base != "" {
			return Cache{Root: filepath.Join(base, "dtkit")}, nil
		}
	}
	if env.UserHomeDir != nil {
		if home, err := env.UserHomeDir(); err == nil && home != "" {
			return Cache{Root: filepath.Join(home, ".dtkit", "cache")}, nil
		}
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return Cache{}, err
	}
	return Cache{Root: filepath.Join(base, "dtkit")}, nil
}

func (c Cache) archivePath(shaHex string) (string, error) {
	lower, err := normalizeSHA256(shaHex)
	if err != nil {
		return "", err
	}
	return paths.ResolveTarget(c.Root, "packages/sha256/"+lower+".tar.gz")
}

func (c Cache) metaPath(shaHex string) (string, error) {
	lower, err := normalizeSHA256(shaHex)
	if err != nil {
		return "", err
	}
	return paths.ResolveTarget(c.Root, "packages/sha256/"+lower+".meta.json")
}

func normalizeSHA256(shaHex string) (string, error) {
	lower := strings.ToLower(shaHex)
	b, err := hex.DecodeString(lower)
	if err != nil || len(b) != 32 {
		return "", apperr.Usage("invalid_sha256", fmt.Sprintf("invalid SHA-256 digest %q", shaHex))
	}
	return lower, nil
}

// Get reads a cached archive and verifies its integrity against shaHex without creating or mutating files.
// Returns (nil, false, nil) if not cached; fails closed on cache corruption.
func (c Cache) Get(shaHex string) ([]byte, bool, error) {
	p, err := c.archivePath(shaHex)
	if err != nil {
		return nil, false, err
	}
	info, err := os.Lstat(p)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	raw, verifyErr := verifyRegularArchive(p, info, shaHex)
	if verifyErr != nil {
		return nil, true, verifyErr
	}
	return raw, true, nil
}

// GetMetadata reads optional cached sidecar metadata without creating or mutating files.
// Fails closed if sidecar metadata exists but is corrupt, non-regular, or mismatched.
func (c Cache) GetMetadata(shaHex string) (CacheMetadata, bool, error) {
	var meta CacheMetadata
	p, err := c.metaPath(shaHex)
	if err != nil {
		return meta, false, err
	}
	info, err := os.Lstat(p)
	if os.IsNotExist(err) {
		return meta, false, nil
	}
	if err != nil {
		return meta, false, err
	}
	return verifyMetadataFile(p, info, shaHex)
}

func verifyMetadataFile(p string, info os.FileInfo, shaHex string) (CacheMetadata, bool, error) {
	var meta CacheMetadata
	if paths.IsReparseOrSymlink(info) || !info.Mode().IsRegular() || info.Size() > MaxMetadataBytes {
		return meta, true, apperr.WithPath(apperr.Conflict("cache_corrupted", "cached metadata is not a bounded regular file"), p)
	}
	f, err := os.Open(p)
	if err != nil {
		return meta, true, err
	}
	raw, readErr := io.ReadAll(io.LimitReader(f, MaxMetadataBytes+1))
	closeErr := f.Close()
	if readErr != nil {
		return meta, true, readErr
	}
	if closeErr != nil {
		return meta, true, closeErr
	}
	if len(raw) > MaxMetadataBytes {
		return meta, true, apperr.WithPath(apperr.Conflict("cache_corrupted", "cached metadata exceeds size limit"), p)
	}
	parsed, err := parseMetadataBytes(raw, shaHex)
	if err != nil {
		return meta, true, apperr.WithPath(apperr.Conflict("cache_corrupted", fmt.Sprintf("invalid cached metadata: %v", err)), p)
	}
	return parsed, true, nil
}

func parseMetadataBytes(raw []byte, expectedSHA string) (CacheMetadata, error) {
	var meta CacheMetadata
	if err := catalog.CheckStrictJSON(raw); err != nil {
		return meta, err
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return meta, err
	}
	if err := catalog.CheckObjectKeys(obj, []string{"schemaVersion", "archiveSha256"}, []string{"packageDigest", "minCliVersion"}); err != nil {
		return meta, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&meta); err != nil {
		return meta, err
	}
	if meta.SchemaVersion != 1 || !strings.EqualFold(meta.ArchiveSHA256, expectedSHA) {
		return meta, fmt.Errorf("metadata schema or archiveSha256 mismatch")
	}
	if meta.PackageDigest != "" {
		if _, err := normalizeSHA256(meta.PackageDigest); err != nil {
			return meta, fmt.Errorf("invalid packageDigest in cache metadata")
		}
	}
	if meta.MinCLIVersion != "" && !catalog.IsValidVersion(meta.MinCLIVersion) {
		return meta, fmt.Errorf("invalid minCliVersion %q in cache metadata", meta.MinCLIVersion)
	}
	return meta, nil
}

func verifyRegularArchive(p string, info os.FileInfo, shaHex string) ([]byte, error) {
	if paths.IsReparseOrSymlink(info) || !info.Mode().IsRegular() {
		return nil, apperr.WithPath(apperr.Conflict("cache_corrupted", "cached archive is not a regular file"), p)
	}
	if info.Size() > MaxArchiveBytes {
		return nil, apperr.WithPath(apperr.Conflict("cache_corrupted", "cached archive exceeds size limit"), p)
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	raw, readErr := io.ReadAll(io.LimitReader(f, MaxArchiveBytes+1))
	closeErr := f.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(raw) > MaxArchiveBytes {
		return nil, apperr.WithPath(apperr.Conflict("cache_corrupted", "cached archive exceeds size limit"), p)
	}
	if !strings.EqualFold(sha256Hex(raw), shaHex) {
		return nil, apperr.WithPath(apperr.Conflict("cache_corrupted", "cached archive checksum mismatch; cache entry may be corrupted"), p)
	}
	return raw, nil
}

// Put stores verified archive bytes under their SHA-256 cache path atomically and idempotently.
func (c Cache) Put(raw []byte) (string, error) {
	dig, _, err := c.PutArchive(raw, nil)
	return dig, err
}

// PutArchive stores raw archive bytes and optional monotonic metadata atomically and idempotently.
func (c Cache) PutArchive(raw []byte, meta *CacheMetadata) (string, bool, error) {
	var dig string
	var wroteBlob bool
	if len(raw) > 0 {
		dig = sha256Hex(raw)
		dest, err := c.archivePath(dig)
		if err != nil {
			return "", false, err
		}
		wrote, err := c.writeCacheFileIdempotent(dest, "dl-*.part", raw, func(p string, info os.FileInfo) error {
			_, vErr := verifyRegularArchive(p, info, dig)
			return vErr
		}, false)
		if err != nil {
			return "", false, err
		}
		wroteBlob = wrote
		c.recordWrite(dig, wroteBlob)
	} else if meta != nil && meta.ArchiveSHA256 != "" {
		norm, err := normalizeSHA256(meta.ArchiveSHA256)
		if err != nil {
			return "", false, err
		}
		dig = norm
	} else {
		return "", false, apperr.Usage("empty_cache_payload", "cache payload cannot be empty")
	}

	if meta != nil && meta.MinCLIVersion != "" {
		wroteMeta, err := c.putMetadata(dig, *meta)
		if err != nil {
			return "", false, err
		}
		wroteBlob = wroteBlob || wroteMeta
	}
	return dig, wroteBlob, nil
}

func (c Cache) putMetadata(shaHex string, incoming CacheMetadata) (bool, error) {
	metaDest, err := c.metaPath(shaHex)
	if err != nil {
		return false, err
	}
	merged := CacheMetadata{
		SchemaVersion: 1,
		ArchiveSHA256: strings.ToLower(shaHex),
		PackageDigest: strings.ToLower(incoming.PackageDigest),
		MinCLIVersion: incoming.MinCLIVersion,
	}
	replace := false
	if existing, found, gErr := c.GetMetadata(shaHex); gErr != nil {
		return false, gErr
	} else if found {
		if existing.PackageDigest != "" && merged.PackageDigest == "" {
			merged.PackageDigest = existing.PackageDigest
		}
		stricter, sErr := maxMinCLIVersion(existing.MinCLIVersion, merged.MinCLIVersion)
		if sErr != nil {
			return false, sErr
		}
		merged.MinCLIVersion = stricter
		if existing == merged {
			return false, nil
		}
		replace = true
	}
	data, err := json.Marshal(merged)
	if err != nil {
		return false, err
	}
	return c.writeCacheFileIdempotent(metaDest, "meta-*.part", append(data, '\n'), func(p string, info os.FileInfo) error {
		cur, _, vErr := verifyMetadataFile(p, info, shaHex)
		if vErr != nil {
			return vErr
		}
		if cur.MinCLIVersion != "" && merged.MinCLIVersion != "" {
			ok, _ := SatisfiesMinCLI(cur.MinCLIVersion, merged.MinCLIVersion)
			if !ok {
				return fmt.Errorf("concurrent metadata has lower minCliVersion %s < %s", cur.MinCLIVersion, merged.MinCLIVersion)
			}
		}
		return nil
	}, replace)
}

func (c Cache) writeCacheFileIdempotent(dest, partPattern string, raw []byte, verifyExisting func(string, os.FileInfo) error, allowReplace bool) (bool, error) {
	if !allowReplace {
		if info, statErr := os.Lstat(dest); statErr == nil {
			if verifyErr := verifyExisting(dest, info); verifyErr != nil {
				return false, verifyErr
			}
			return false, nil
		} else if !os.IsNotExist(statErr) {
			return false, statErr
		}
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return false, err
	}
	tmpDir, err := paths.ResolveTarget(c.Root, "tmp")
	if err != nil {
		return false, err
	}
	if err := os.MkdirAll(tmpDir, 0700); err != nil {
		return false, err
	}
	part, err := os.CreateTemp(tmpDir, partPattern)
	if err != nil {
		return false, err
	}
	partPath := part.Name()
	lockPath := partPath + ".lock"
	defer os.Remove(partPath)

	if lockRaw, mErr := json.Marshal(downloadLock{PID: os.Getpid()}); mErr == nil {
		if lf, lErr := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600); lErr == nil {
			_, _ = lf.Write(lockRaw)
			_ = lf.Close()
			defer os.Remove(lockPath)
		}
	}

	if err = part.Chmod(0600); err == nil {
		_, err = part.Write(raw)
	}
	if err == nil {
		err = part.Sync()
	}
	if closeErr := part.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return false, err
	}

	var renameErr error
	for attempt := 0; attempt < 5; attempt++ {
		renameErr = os.Rename(partPath, dest)
		if renameErr == nil {
			return true, nil
		}
		if info, statErr := os.Lstat(dest); statErr == nil {
			if verifyErr := verifyExisting(dest, info); verifyErr == nil {
				return false, nil
			}
		}
		time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
	}
	return false, renameErr
}

func (c Cache) recordWrite(dig string, wrote bool) {
	if c.Hooks != nil && c.Hooks.OnWrite != nil {
		c.Hooks.OnWrite(dig, wrote)
	}
}

func (c Cache) recordClean(path string) {
	if c.Hooks != nil && c.Hooks.OnClean != nil {
		c.Hooks.OnClean(path)
	}
}

// CleanStaleDownloads removes abandoned partial downloads inside cache/tmp that exceed DefaultStalePartAge or belong to dead processes.
func (c Cache) CleanStaleDownloads() error {
	return c.CleanStaleDownloadsOlderThan(DefaultStalePartAge, time.Now())
}

// CleanStaleDownloadsOlderThan removes partial download artifacts in cache/tmp only when confirmed stale (dead lock owner or older than maxAge without an active lock).
func (c Cache) CleanStaleDownloadsOlderThan(maxAge time.Duration, now time.Time) error {
	tmpDir, err := paths.ResolveTarget(c.Root, "tmp")
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(tmpDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".part") {
			continue
		}
		partPath := filepath.Join(tmpDir, entry.Name())
		lockPath := partPath + ".lock"
		if lockRaw, lErr := os.ReadFile(lockPath); lErr == nil {
			var dl downloadLock
			if jErr := json.Unmarshal(lockRaw, &dl); jErr != nil || paths.ProcessLiveness(dl.PID) != "dead" {
				continue
			}
			if os.Remove(partPath) == nil {
				c.recordClean(partPath)
			}
			_ = os.Remove(lockPath)
			continue
		} else if !os.IsNotExist(lErr) {
			continue
		}
		info, iErr := entry.Info()
		if iErr != nil {
			continue
		}
		if now.Sub(info.ModTime()) >= maxAge {
			if os.Remove(partPath) == nil {
				c.recordClean(partPath)
			}
		}
	}
	return nil
}

// FetchRemoteBytes fetches a remote resource over HTTPS (or loopback HTTP for tests) purely in memory,
// enforcing finite timeouts, bounded redirects, hop-by-hop scheme validation, cross-host credential stripping,
// strict byte limits, and redacted URL error reporting.
func FetchRemoteBytes(ctx context.Context, client *http.Client, rawURL string, maxBytes int64) ([]byte, error) {
	if err := ValidateRemoteURL(rawURL); err != nil {
		return nil, err
	}
	safeDisplayURL := SanitizeURL(rawURL)
	initialParsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, apperr.Usage("invalid_source_url", fmt.Sprintf("invalid source URL %q", safeDisplayURL))
	}

	var effective http.Client
	if client != nil {
		effective = *client
	}
	if effective.Timeout == 0 {
		effective.Timeout = DefaultHTTPTimeout
	}
	origCheckRedirect := effective.CheckRedirect
	effective.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= MaxRedirects {
			return apperr.Usage("too_many_redirects", fmt.Sprintf("remote source %s exceeded %d redirect limit", safeDisplayURL, MaxRedirects))
		}
		if err := ValidateRemoteURL(req.URL.String()); err != nil {
			return err
		}
		if len(via) > 0 {
			prev := via[len(via)-1].URL
			if strings.EqualFold(prev.Scheme, "https") && !strings.EqualFold(req.URL.Scheme, "https") {
				return apperr.Usage("insecure_redirect", fmt.Sprintf("redirect from HTTPS to %s (%q) is not allowed", req.URL.Scheme, SanitizeURL(req.URL.String())))
			}
		}
		if !strings.EqualFold(req.URL.Host, initialParsed.Host) || !strings.EqualFold(req.URL.Scheme, initialParsed.Scheme) {
			req.Header.Del("Authorization")
			req.Header.Del("Proxy-Authorization")
			req.Header.Del("Cookie")
		}
		if origCheckRedirect != nil {
			return origCheckRedirect(req, via)
		}
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := effective.Do(req)
	if err != nil {
		var appErr *apperr.Error
		if errors.As(err, &appErr) {
			return nil, appErr
		}
		if errors.Is(err, context.Canceled) {
			return nil, apperr.Cancelled(fmt.Sprintf("download %s cancelled", safeDisplayURL))
		}
		cleanErr := sanitizeHTTPError(err)
		return nil, apperr.Wrap(apperr.ExitInternal, "remote_fetch_failed", fmt.Sprintf("download %s: %v", safeDisplayURL, cleanErr), cleanErr)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, apperr.Usage("remote_http_error", fmt.Sprintf("remote source %s returned HTTP %d", safeDisplayURL, resp.StatusCode))
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, apperr.Wrap(apperr.ExitInternal, "remote_fetch_failed", fmt.Sprintf("read response %s: %v", safeDisplayURL, sanitizeHTTPError(err)), err)
	}
	if int64(len(raw)) > maxBytes {
		return nil, apperr.Usage("remote_payload_too_large", fmt.Sprintf("remote response from %s exceeds %d byte limit", safeDisplayURL, maxBytes))
	}
	return raw, nil
}

func sanitizeHTTPError(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		ueCopy := *ue
		ueCopy.URL = SanitizeURL(ue.URL)
		return fmt.Errorf("%s %q: %w", ueCopy.Op, ueCopy.URL, ueCopy.Err)
	}
	return err
}

// SanitizeURL removes embedded userinfo, query parameters (such as signed tokens), and fragments for safe diagnostics.
func SanitizeURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "[invalid-url]"
	}
	cleaned := url.URL{
		Scheme: strings.ToLower(u.Scheme),
		Host:   u.Host,
		Path:   u.Path,
	}
	return cleaned.String()
}

// IsLoopbackHost reports whether host refers to a local loopback address or localhost.
func IsLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return true
	}
	return false
}

// ValidateRemoteURL requires HTTPS for remote sources (permitting plain HTTP only on loopback),
// and rejects embedded URL credentials.
func ValidateRemoteURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return apperr.Usage("invalid_source_url", fmt.Sprintf("invalid source URL %q", SanitizeURL(rawURL)))
	}
	if u.User != nil {
		return apperr.Usage("credentials_in_url", fmt.Sprintf("source URL %q must not contain embedded user credentials", SanitizeURL(rawURL)))
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return nil
	case "http":
		if IsLoopbackHost(u.Hostname()) {
			return nil
		}
		return apperr.Usage("insecure_source_url", fmt.Sprintf("plain HTTP source %q is not allowed; use HTTPS", SanitizeURL(rawURL)))
	default:
		return apperr.Usage("unsupported_source_scheme", fmt.Sprintf("unsupported URL scheme %q (use HTTPS or --from for local paths)", u.Scheme))
	}
}
