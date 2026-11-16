package dist

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
)

const MaxIndexBytes = 1024 * 1024

type RegistryIndex struct {
	SchemaVersion int          `json:"schemaVersion"`
	Packages      []IndexEntry `json:"packages"`
}

type IndexEntry struct {
	ID            string `json:"id"`
	Version       string `json:"version"`
	URL           string `json:"url"`
	ArchiveSHA256 string `json:"archiveSha256"`
	PackageDigest string `json:"packageDigest,omitempty"`
	MinCLIVersion string `json:"minCliVersion,omitempty"`
}

// CacheMode explicitly declares whether source resolution may persist fetched archives to cache.
type CacheMode uint8

const (
	// CacheReadOnly reads and verifies existing cache entries without creating directories or writing/deleting files.
	CacheReadOnly CacheMode = iota
	// CachePersist persists newly downloaded and verified remote archives into cache.
	CachePersist
)

type ResolvedPackage struct {
	Package       catalog.Package
	Source        fs.FS
	PackageDigest string
	ArchiveSHA256 string
	SourceKind    string // "bundled" | "local" | "archive" | "remote"

	archiveBytes []byte
	fromCache    bool
}

// HasPendingCacheWrite reports whether resolved carries newly fetched remote archive bytes that are not yet verified in cache.
func (r ResolvedPackage) HasPendingCacheWrite() bool {
	return len(r.archiveBytes) > 0 && !r.fromCache
}

// PersistCache stores newly downloaded remote archive bytes in cache when needed.
// Cache hits, no-ops, bundled packages, and local packages do not write or update timestamps.
func (r ResolvedPackage) PersistCache(cache Cache) (bool, error) {
	if !r.HasPendingCacheWrite() {
		return false, nil
	}
	_, wrote, err := cache.putChecked(r.archiveBytes)
	return wrote, err
}

// ParseKitSpec splits `<id>` or `<id>@<version>` into components.
func ParseKitSpec(spec string) (id, version string, err error) {
	if spec == "" {
		return "", "", apperr.Usage("missing_kit", "kit identifier is required")
	}
	if !strings.Contains(spec, "@") {
		if !slugPattern.MatchString(spec) || len(spec) > 32 {
			return "", "", apperr.Usage("invalid_kit_id", fmt.Sprintf("invalid kit identifier %q", spec))
		}
		return spec, "", nil
	}
	parts := strings.Split(spec, "@")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", apperr.Usage("invalid_kit_spec", fmt.Sprintf("invalid kit spec %q (expected <id>@<version>)", spec))
	}
	if !slugPattern.MatchString(parts[0]) || len(parts[0]) > 32 {
		return "", "", apperr.Usage("invalid_kit_id", fmt.Sprintf("invalid kit identifier %q", parts[0]))
	}
	if _, err := parseSemverTuple(parts[1]); err != nil {
		return "", "", apperr.Usage("invalid_version", fmt.Sprintf("invalid version %q in %q", parts[1], spec))
	}
	return parts[0], parts[1], nil
}

// ResolveFromPath loads a package from a local directory or a .tar.gz archive file.
func ResolveFromPath(fromPath, kitID, wantVersion, expectedSHA256 string) (ResolvedPackage, error) {
	var out ResolvedPackage
	abs, err := filepath.Abs(fromPath)
	if err != nil {
		return out, err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return out, apperr.WithPath(apperr.Usage("source_not_found", "--from path does not exist"), abs)
		}
		return out, err
	}
	if info.IsDir() {
		if expectedSHA256 != "" {
			return out, apperr.Usage("invalid_flag_combination", "--sha256 applies to archives, not directories")
		}
		pkg, snap, dig, err := catalog.LoadDir(abs)
		if err != nil {
			return out, err
		}
		if err := verifyIDAndVersion(pkg, kitID, wantVersion); err != nil {
			return out, err
		}
		return ResolvedPackage{
			Package:       pkg,
			Source:        snap,
			PackageDigest: dig,
			SourceKind:    "local",
		}, nil
	}
	pkg, snap, dig, archSHA, err := ReadArchiveFile(abs, expectedSHA256)
	if err != nil {
		return out, err
	}
	if err := verifyIDAndVersion(pkg, kitID, wantVersion); err != nil {
		return out, err
	}
	return ResolvedPackage{
		Package:       pkg,
		Source:        snap,
		PackageDigest: dig,
		ArchiveSHA256: archSHA,
		SourceKind:    "archive",
	}, nil
}

// ResolveRemote resolves a kit from a remote archive/registry URL or verified offline cache
// using an explicit CacheMode read/write policy.
func ResolveRemote(ctx context.Context, client *http.Client, cache Cache, kitID, wantVersion, sourceURL, wantArchiveSHA256, cliVersion string, offline bool, mode CacheMode) (ResolvedPackage, error) {
	var out ResolvedPackage

	// Offline mode never performs network I/O and never creates or mutates cache.
	if offline {
		if wantArchiveSHA256 == "" {
			return out, apperr.WithHint(
				apperr.Usage("offline_digest_required", "offline remote lookup requires a pinned archive SHA-256 digest"),
				"pass --sha256 <hex> or omit --offline to fetch metadata",
			)
		}
		raw, found, err := cache.Get(wantArchiveSHA256)
		if err != nil {
			return out, err
		}
		if !found {
			return out, apperr.Conflict("offline_cache_miss", fmt.Sprintf("archive %s is not present in offline cache", wantArchiveSHA256))
		}
		pkg, snap, dig, archSHA, err := ReadArchiveBytes(raw, wantArchiveSHA256)
		if err != nil {
			return out, err
		}
		if err := verifyIDAndVersion(pkg, kitID, wantVersion); err != nil {
			return out, err
		}
		return ResolvedPackage{
			Package:       pkg,
			Source:        snap,
			PackageDigest: dig,
			ArchiveSHA256: archSHA,
			SourceKind:    "remote",
			fromCache:     true,
		}, nil
	}

	if err := ValidateRemoteURL(sourceURL); err != nil {
		return out, err
	}

	// If cached by explicit archive SHA-256, reuse verified cache without hitting network or rewriting cache.
	if wantArchiveSHA256 != "" {
		if raw, found, err := cache.Get(wantArchiveSHA256); err != nil {
			return out, err
		} else if found {
			pkg, snap, dig, archSHA, err := ReadArchiveBytes(raw, wantArchiveSHA256)
			if err != nil {
				return out, err
			}
			if err := verifyIDAndVersion(pkg, kitID, wantVersion); err != nil {
				return out, err
			}
			return ResolvedPackage{
				Package:       pkg,
				Source:        snap,
				PackageDigest: dig,
				ArchiveSHA256: archSHA,
				SourceKind:    "remote",
				fromCache:     true,
			}, nil
		}
	}

	parsed, err := url.Parse(sourceURL)
	if err != nil {
		return out, err
	}
	isArchiveURL := strings.HasSuffix(strings.ToLower(parsed.Path), ".tar.gz") || strings.HasSuffix(strings.ToLower(parsed.Path), ".tgz")
	if isArchiveURL {
		raw, err := FetchRemoteBytes(ctx, client, sourceURL, MaxArchiveBytes)
		if err != nil {
			return out, err
		}
		pkg, snap, dig, archSHA, err := ReadArchiveBytes(raw, wantArchiveSHA256)
		if err != nil {
			return out, err
		}
		if err := verifyIDAndVersion(pkg, kitID, wantVersion); err != nil {
			return out, err
		}
		res := ResolvedPackage{
			Package:       pkg,
			Source:        snap,
			PackageDigest: dig,
			ArchiveSHA256: archSHA,
			SourceKind:    "remote",
			archiveBytes:  raw,
		}
		if mode == CachePersist {
			if _, err := res.PersistCache(cache); err != nil {
				return out, err
			}
			res.archiveBytes = nil
			res.fromCache = true
		}
		return res, nil
	}

	// Registry index JSON resolution.
	indexBytes, err := FetchRemoteBytes(ctx, client, sourceURL, MaxIndexBytes)
	if err != nil {
		return out, err
	}
	var idx RegistryIndex
	if err := json.Unmarshal(indexBytes, &idx); err != nil || idx.SchemaVersion != 1 {
		return out, apperr.Usage("invalid_registry_index", "remote registry index must be valid JSON with schemaVersion 1")
	}

	entry, err := selectIndexEntry(idx.Packages, kitID, wantVersion)
	if err != nil {
		return out, err
	}
	if entry.MinCLIVersion != "" {
		if ok, cmpErr := satisfiesMinCLI(cliVersion, entry.MinCLIVersion); cmpErr != nil {
			return out, cmpErr
		} else if !ok {
			return out, apperr.Usage("cli_upgrade_required", fmt.Sprintf("%s %s requires dtkit >= %s (current %s)", entry.ID, entry.Version, entry.MinCLIVersion, cliVersion))
		}
	}

	archiveURL := entry.URL
	if rel, rErr := url.Parse(entry.URL); rErr == nil && !rel.IsAbs() {
		archiveURL = parsed.ResolveReference(rel).String()
	}
	if wantArchiveSHA256 != "" && !strings.EqualFold(wantArchiveSHA256, entry.ArchiveSHA256) {
		return out, apperr.Conflict("archive_digest_mismatch", fmt.Sprintf("pinned SHA-256 %s conflicts with registry entry %s", wantArchiveSHA256, entry.ArchiveSHA256))
	}
	expectedArchiveSHA := entry.ArchiveSHA256
	if expectedArchiveSHA == "" {
		return out, apperr.Usage("missing_registry_digest", "registry entry missing archiveSha256")
	}

	var raw []byte
	fromCache := false
	if cachedRaw, found, cErr := cache.Get(expectedArchiveSHA); cErr != nil {
		return out, cErr
	} else if found {
		raw = cachedRaw
		fromCache = true
	} else {
		raw, err = FetchRemoteBytes(ctx, client, archiveURL, MaxArchiveBytes)
		if err != nil {
			return out, err
		}
	}

	pkg, snap, pkgDigest, archSHA, err := ReadArchiveBytes(raw, expectedArchiveSHA)
	if err != nil {
		return out, err
	}
	if err := verifyIDAndVersion(pkg, kitID, entry.Version); err != nil {
		return out, err
	}
	if entry.PackageDigest != "" && !strings.EqualFold(entry.PackageDigest, pkgDigest) {
		return out, apperr.Conflict("package_digest_mismatch", fmt.Sprintf("registry packageDigest %s does not match archive content %s", entry.PackageDigest, pkgDigest))
	}
	res := ResolvedPackage{
		Package:       pkg,
		Source:        snap,
		PackageDigest: pkgDigest,
		ArchiveSHA256: archSHA,
		SourceKind:    "remote",
		fromCache:     fromCache,
	}
	if !fromCache {
		res.archiveBytes = raw
		if mode == CachePersist {
			if _, err := res.PersistCache(cache); err != nil {
				return out, err
			}
			res.archiveBytes = nil
			res.fromCache = true
		}
	}
	return res, nil
}

func selectIndexEntry(entries []IndexEntry, kitID, wantVersion string) (IndexEntry, error) {
	var best IndexEntry
	found := false
	for _, e := range entries {
		if e.ID != kitID {
			continue
		}
		if wantVersion != "" {
			if e.Version == wantVersion {
				return e, nil
			}
			continue
		}
		if !found {
			best = e
			found = true
			continue
		}
		cNew, err1 := parseSemverTuple(e.Version)
		cBest, err2 := parseSemverTuple(best.Version)
		if err1 == nil && err2 == nil && compareTuples(cNew, cBest) > 0 {
			best = e
		}
	}
	if !found {
		return IndexEntry{}, apperr.Usage("kit_version_not_found", fmt.Sprintf("kit %q (%q) not found in remote index", kitID, wantVersion))
	}
	return best, nil
}

func verifyIDAndVersion(pkg catalog.Package, wantID, wantVersion string) error {
	if pkg.ID != wantID {
		return apperr.Usage("kit_id_mismatch", fmt.Sprintf("package ID %q does not match requested %q", pkg.ID, wantID))
	}
	if wantVersion != "" && pkg.Version != wantVersion {
		return apperr.Usage("kit_version_mismatch", fmt.Sprintf("package version %q does not match requested %q", pkg.Version, wantVersion))
	}
	return nil
}

func satisfiesMinCLI(current, min string) (bool, error) {
	baseCurrent := strings.SplitN(current, "-", 2)[0]
	cur, err := parseSemverTuple(baseCurrent)
	if err != nil {
		return false, err
	}
	req, err := parseSemverTuple(min)
	if err != nil {
		return false, err
	}
	return compareTuples(cur, req) >= 0, nil
}

func parseSemverTuple(v string) ([3]int, error) {
	var out [3]int
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, fmt.Errorf("invalid semver %q", v)
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, fmt.Errorf("invalid semver %q", v)
		}
		out[i] = n
	}
	return out, nil
}

func compareTuples(a, b [3]int) int {
	for i := 0; i < 3; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}
