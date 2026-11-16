package dist

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

// Cache stores verified package archives keyed by SHA-256 hex digest.
type Cache struct {
	Root string
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
	lower := strings.ToLower(shaHex)
	b, err := hex.DecodeString(lower)
	if err != nil || len(b) != 32 {
		return "", apperr.Usage("invalid_sha256", fmt.Sprintf("invalid SHA-256 digest %q", shaHex))
	}
	return paths.ResolveTarget(c.Root, "packages/sha256/"+lower+".tar.gz")
}

// Get reads a cached archive and verifies its integrity against shaHex.
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
	if paths.IsReparseOrSymlink(info) || !info.Mode().IsRegular() {
		return nil, true, apperr.WithPath(apperr.Conflict("cache_corrupted", "cached archive is not a regular file"), p)
	}
	if info.Size() > MaxArchiveBytes {
		return nil, true, apperr.WithPath(apperr.Conflict("cache_corrupted", "cached archive exceeds size limit"), p)
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, true, err
	}
	raw, readErr := io.ReadAll(io.LimitReader(f, MaxArchiveBytes+1))
	closeErr := f.Close()
	if readErr != nil {
		return nil, true, readErr
	}
	if closeErr != nil {
		return nil, true, closeErr
	}
	if len(raw) > MaxArchiveBytes {
		return nil, true, apperr.WithPath(apperr.Conflict("cache_corrupted", "cached archive exceeds size limit"), p)
	}
	if !strings.EqualFold(sha256Hex(raw), shaHex) {
		return nil, true, apperr.WithPath(apperr.Conflict("cache_corrupted", "cached archive checksum mismatch; cache entry may be corrupted"), p)
	}
	return raw, true, nil
}

// Put stores verified archive bytes under their SHA-256 cache path atomically.
func (c Cache) Put(raw []byte) (string, error) {
	dig := sha256Hex(raw)
	dest, err := c.archivePath(dig)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return "", err
	}
	tmpDir, err := paths.ResolveTarget(c.Root, "tmp")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(tmpDir, 0700); err != nil {
		return "", err
	}
	part, err := os.CreateTemp(tmpDir, "dl-*.part")
	if err != nil {
		return "", err
	}
	partPath := part.Name()
	defer os.Remove(partPath)

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
		return "", err
	}
	if err := os.Rename(partPath, dest); err != nil {
		return "", err
	}
	return dig, nil
}

// CleanStaleDownloads removes leftover partial downloads inside cache/tmp.
func (c Cache) CleanStaleDownloads() error {
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
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".part") {
			_ = os.Remove(filepath.Join(tmpDir, entry.Name()))
		}
	}
	return nil
}

// DownloadBytes fetches a remote resource over HTTPS (or loopback HTTP for tests),
// enforcing strict byte limits and atomic temp-file cleanup on failure.
func (c Cache) DownloadBytes(ctx context.Context, client *http.Client, rawURL string, maxBytes int64) ([]byte, error) {
	if err := ValidateRemoteURL(rawURL); err != nil {
		return nil, err
	}
	if client == nil {
		client = http.DefaultClient
	}
	tmpDir, err := paths.ResolveTarget(c.Root, "tmp")
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(tmpDir, 0700); err != nil {
		return nil, err
	}
	part, err := os.CreateTemp(tmpDir, "fetch-*.part")
	if err != nil {
		return nil, err
	}
	partPath := part.Name()
	cleanup := func() {
		_ = part.Close()
		_ = os.Remove(partPath)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		cleanup()
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		cleanup()
		return nil, apperr.Wrap(apperr.ExitInternal, "remote_fetch_failed", fmt.Sprintf("download %s: %v", rawURL, err), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		cleanup()
		return nil, apperr.Usage("remote_http_error", fmt.Sprintf("remote source %s returned HTTP %d", rawURL, resp.StatusCode))
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		cleanup()
		return nil, apperr.Wrap(apperr.ExitInternal, "remote_fetch_failed", fmt.Sprintf("read response %s: %v", rawURL, err), err)
	}
	if int64(len(raw)) > maxBytes {
		cleanup()
		return nil, apperr.Usage("remote_payload_too_large", fmt.Sprintf("remote response from %s exceeds %d byte limit", rawURL, maxBytes))
	}
	if _, err := part.Write(raw); err != nil {
		cleanup()
		return nil, err
	}
	if err := part.Close(); err != nil {
		_ = os.Remove(partPath)
		return nil, err
	}
	_ = os.Remove(partPath)
	return raw, nil
}

// ValidateRemoteURL requires HTTPS for remote sources, permitting plain HTTP only for loopback hosts.
func ValidateRemoteURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return apperr.Usage("invalid_source_url", fmt.Sprintf("invalid source URL %q", rawURL))
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return nil
	case "http":
		host := u.Hostname()
		if host == "localhost" {
			return nil
		}
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			return nil
		}
		return apperr.Usage("insecure_source_url", fmt.Sprintf("plain HTTP source %q is not allowed; use HTTPS", rawURL))
	default:
		return apperr.Usage("unsupported_source_scheme", fmt.Sprintf("unsupported URL scheme %q (use HTTPS or --from for local paths)", u.Scheme))
	}
}
