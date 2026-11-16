package dist

import (
	"context"
	"encoding/hex"
	"encoding/json"
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
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

// DefaultStalePartAge is the minimum idle age required before unowned partial download files are pruned.
const DefaultStalePartAge = 15 * time.Minute

type downloadLock struct {
	PID int `json:"pid"`
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

// Put stores verified archive bytes under their SHA-256 cache path atomically.
func (c Cache) Put(raw []byte) (string, error) {
	dig, _, err := c.putChecked(raw)
	return dig, err
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

func (c Cache) putChecked(raw []byte) (string, bool, error) {
	dig := sha256Hex(raw)
	dest, err := c.archivePath(dig)
	if err != nil {
		return "", false, err
	}
	if info, statErr := os.Lstat(dest); statErr == nil {
		if _, verifyErr := verifyRegularArchive(dest, info, dig); verifyErr != nil {
			return "", false, verifyErr
		}
		c.recordWrite(dig, false)
		return dig, false, nil
	} else if !os.IsNotExist(statErr) {
		return "", false, statErr
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return "", false, err
	}
	tmpDir, err := paths.ResolveTarget(c.Root, "tmp")
	if err != nil {
		return "", false, err
	}
	if err := os.MkdirAll(tmpDir, 0700); err != nil {
		return "", false, err
	}
	part, err := os.CreateTemp(tmpDir, "dl-*.part")
	if err != nil {
		return "", false, err
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
		return "", false, err
	}
	if err := os.Rename(partPath, dest); err != nil {
		if info, statErr := os.Lstat(dest); statErr == nil {
			if _, verifyErr := verifyRegularArchive(dest, info, dig); verifyErr == nil {
				c.recordWrite(dig, false)
				return dig, false, nil
			}
		}
		return "", false, err
	}
	c.recordWrite(dig, true)
	return dig, true, nil
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
// enforcing strict byte limits without touching the local cache or filesystem.
func FetchRemoteBytes(ctx context.Context, client *http.Client, rawURL string, maxBytes int64) ([]byte, error) {
	if err := ValidateRemoteURL(rawURL); err != nil {
		return nil, err
	}
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, apperr.Wrap(apperr.ExitInternal, "remote_fetch_failed", fmt.Sprintf("download %s: %v", rawURL, err), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, apperr.Usage("remote_http_error", fmt.Sprintf("remote source %s returned HTTP %d", rawURL, resp.StatusCode))
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, apperr.Wrap(apperr.ExitInternal, "remote_fetch_failed", fmt.Sprintf("read response %s: %v", rawURL, err), err)
	}
	if int64(len(raw)) > maxBytes {
		return nil, apperr.Usage("remote_payload_too_large", fmt.Sprintf("remote response from %s exceeds %d byte limit", rawURL, maxBytes))
	}
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
