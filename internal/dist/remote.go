package dist

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
)

const (
	MaxIndexBytes   = 1024 * 1024
	MaxIndexEntries = 1024
)

var prereleasePattern = regexp.MustCompile(`^[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*$`)

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

	archiveBytes    []byte
	fromCache       bool
	pendingMeta     *CacheMetadata
	metaWriteNeeded bool
}

// HasPendingCacheWrite reports whether resolved carries newly fetched archive bytes or metadata updates not yet persisted in cache.
func (r ResolvedPackage) HasPendingCacheWrite() bool {
	return (len(r.archiveBytes) > 0 && !r.fromCache) || r.metaWriteNeeded
}

// PersistCache stores newly downloaded remote archive bytes and/or compatibility metadata in cache when needed.
// Cache hits without metadata changes, no-ops, bundled packages, and local packages do not write or update timestamps.
func (r ResolvedPackage) PersistCache(cache Cache) (bool, error) {
	if !r.HasPendingCacheWrite() {
		return false, nil
	}
	var payload []byte
	if !r.fromCache {
		payload = r.archiveBytes
	}
	_, wrote, err := cache.PutArchive(payload, r.pendingMeta)
	return wrote, err
}

// ParseKitSpec splits `<id>` or `<id>@<version>` into components.
func ParseKitSpec(spec string) (id, version string, err error) {
	if spec == "" {
		return "", "", apperr.Usage("missing_kit", "kit identifier is required")
	}
	if !strings.Contains(spec, "@") {
		if !catalog.IsValidID(spec, 32) {
			return "", "", apperr.Usage("invalid_kit_id", fmt.Sprintf("invalid kit identifier %q", spec))
		}
		return spec, "", nil
	}
	parts := strings.Split(spec, "@")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", apperr.Usage("invalid_kit_spec", fmt.Sprintf("invalid kit spec %q (expected <id>@<version>)", spec))
	}
	if !catalog.IsValidID(parts[0], 32) {
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

// ParseRegistryIndex strictly parses and validates a remote registry index JSON payload.
// It rejects duplicate JSON keys, unknown or null fields, excessive entry counts, invalid IDs/versions/digests/URLs,
// and duplicate (id, version) entries.
func ParseRegistryIndex(data []byte, baseURL string) (RegistryIndex, error) {
	var idx RegistryIndex
	if len(data) > MaxIndexBytes {
		return idx, apperr.Usage("remote_payload_too_large", fmt.Sprintf("registry index exceeds %d byte limit", MaxIndexBytes))
	}
	if err := catalog.CheckStrictJSON(data); err != nil {
		return idx, apperr.Wrap(apperr.ExitUsage, "invalid_registry_index", fmt.Sprintf("remote registry index JSON error: %v", err), err)
	}
	var topObj map[string]json.RawMessage
	if err := json.Unmarshal(data, &topObj); err != nil {
		return idx, apperr.Wrap(apperr.ExitUsage, "invalid_registry_index", "remote registry index must be a JSON object", err)
	}
	if err := catalog.CheckObjectKeys(topObj, []string{"schemaVersion", "packages"}, nil); err != nil {
		return idx, apperr.Wrap(apperr.ExitUsage, "invalid_registry_index", fmt.Sprintf("remote registry index schema: %v", err), err)
	}
	var rawEntries []map[string]json.RawMessage
	if err := json.Unmarshal(topObj["packages"], &rawEntries); err != nil {
		return idx, apperr.Wrap(apperr.ExitUsage, "invalid_registry_index", "remote registry index packages must be an array of objects", err)
	}
	if len(rawEntries) > MaxIndexEntries {
		return idx, apperr.Usage("invalid_registry_index", fmt.Sprintf("remote registry index exceeds %d entry limit", MaxIndexEntries))
	}
	for i, rawEntry := range rawEntries {
		if err := catalog.CheckObjectKeys(rawEntry, []string{"id", "version", "url", "archiveSha256"}, []string{"packageDigest", "minCliVersion"}); err != nil {
			return idx, apperr.Usage("invalid_registry_index", fmt.Sprintf("registry entry %d: %v", i, err))
		}
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&idx); err != nil || idx.SchemaVersion != 1 || idx.Packages == nil {
		return RegistryIndex{}, apperr.Usage("invalid_registry_index", "remote registry index must be valid JSON with schemaVersion 1")
	}

	var baseParsed *url.URL
	if baseURL != "" {
		u, uErr := url.Parse(baseURL)
		if uErr == nil {
			baseParsed = u
		}
	}

	seenVersions := make(map[string]struct{}, len(idx.Packages))
	for i := range idx.Packages {
		e := &idx.Packages[i]
		if !catalog.IsValidID(e.ID, 32) {
			return RegistryIndex{}, apperr.Usage("invalid_registry_entry", fmt.Sprintf("registry entry %d has invalid kit id %q", i, e.ID))
		}
		if _, err := parseSemverTuple(e.Version); err != nil {
			return RegistryIndex{}, apperr.Usage("invalid_registry_entry", fmt.Sprintf("registry entry %s has invalid semver %q", e.ID, e.Version))
		}
		verKey := e.ID + "@" + e.Version
		if _, dup := seenVersions[verKey]; dup {
			return RegistryIndex{}, apperr.Usage("duplicate_registry_entry", fmt.Sprintf("registry index contains duplicate entry %s", verKey))
		}
		seenVersions[verKey] = struct{}{}

		if e.URL == "" {
			return RegistryIndex{}, apperr.Usage("invalid_registry_entry", fmt.Sprintf("registry entry %s missing url", verKey))
		}
		resolvedURL := e.URL
		if rel, rErr := url.Parse(e.URL); rErr != nil {
			return RegistryIndex{}, apperr.Usage("invalid_registry_entry", fmt.Sprintf("registry entry %s has invalid url", verKey))
		} else if !rel.IsAbs() {
			if baseParsed == nil {
				return RegistryIndex{}, apperr.Usage("invalid_registry_entry", fmt.Sprintf("registry entry %s uses relative url without base source URL", verKey))
			}
			resolvedURL = baseParsed.ResolveReference(rel).String()
		}
		if err := ValidateRemoteURL(resolvedURL); err != nil {
			return RegistryIndex{}, err
		}
		e.URL = resolvedURL

		normArch, err := normalizeSHA256(e.ArchiveSHA256)
		if err != nil {
			return RegistryIndex{}, apperr.Usage("invalid_registry_entry", fmt.Sprintf("registry entry %s has invalid archiveSha256", verKey))
		}
		e.ArchiveSHA256 = normArch

		if e.PackageDigest != "" {
			normPkg, err := normalizeSHA256(e.PackageDigest)
			if err != nil {
				return RegistryIndex{}, apperr.Usage("invalid_registry_entry", fmt.Sprintf("registry entry %s has invalid packageDigest", verKey))
			}
			e.PackageDigest = normPkg
		}
		if e.MinCLIVersion != "" && !catalog.IsValidVersion(e.MinCLIVersion) {
			return RegistryIndex{}, apperr.Usage("invalid_registry_entry", fmt.Sprintf("registry entry %s has invalid minCliVersion %q", verKey, e.MinCLIVersion))
		}
	}
	return idx, nil
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
		meta, _, mErr := cache.GetMetadata(wantArchiveSHA256)
		if mErr != nil {
			return out, mErr
		}
		pkg, snap, dig, archSHA, err := ReadArchiveBytes(raw, wantArchiveSHA256)
		if err != nil {
			return out, err
		}
		if err := verifyIDAndVersion(pkg, kitID, wantVersion); err != nil {
			return out, err
		}
		effectiveMin, err := maxMinCLIVersion(meta.MinCLIVersion, pkg.MinCLIVersion)
		if err != nil {
			return out, err
		}
		if err := CheckMinCLIVersion(cliVersion, effectiveMin, pkg.ID+" "+pkg.Version); err != nil {
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
	parsed, err := url.Parse(sourceURL)
	if err != nil {
		return out, err
	}
	isArchiveURL := strings.HasSuffix(strings.ToLower(parsed.Path), ".tar.gz") || strings.HasSuffix(strings.ToLower(parsed.Path), ".tgz")

	if isArchiveURL {
		var raw []byte
		fromCache := false
		if wantArchiveSHA256 != "" {
			if cachedRaw, found, cErr := cache.Get(wantArchiveSHA256); cErr != nil {
				return out, cErr
			} else if found {
				raw = cachedRaw
				fromCache = true
			}
		}
		if !fromCache {
			raw, err = FetchRemoteBytes(ctx, client, sourceURL, MaxArchiveBytes)
			if err != nil {
				return out, err
			}
		}
		pkg, snap, dig, archSHA, err := ReadArchiveBytes(raw, wantArchiveSHA256)
		if err != nil {
			return out, err
		}
		if err := verifyIDAndVersion(pkg, kitID, wantVersion); err != nil {
			return out, err
		}
		existingMeta, metaFound, mErr := cache.GetMetadata(archSHA)
		if mErr != nil {
			return out, mErr
		}
		effectiveMin, err := maxMinCLIVersion(existingMeta.MinCLIVersion, pkg.MinCLIVersion)
		if err != nil {
			return out, err
		}
		if err := CheckMinCLIVersion(cliVersion, effectiveMin, pkg.ID+" "+pkg.Version); err != nil {
			return out, err
		}
		var pendingMeta *CacheMetadata
		metaNeedWrite := false
		if effectiveMin != "" {
			pendingMeta = &CacheMetadata{
				SchemaVersion: 1,
				ArchiveSHA256: archSHA,
				PackageDigest: dig,
				MinCLIVersion: effectiveMin,
			}
			metaNeedWrite = !metaFound || existingMeta.MinCLIVersion != effectiveMin
		}
		res := ResolvedPackage{
			Package:         pkg,
			Source:          snap,
			PackageDigest:   dig,
			ArchiveSHA256:   archSHA,
			SourceKind:      "remote",
			fromCache:       fromCache,
			pendingMeta:     pendingMeta,
			metaWriteNeeded: metaNeedWrite,
		}
		if !fromCache {
			res.archiveBytes = raw
		}
		if mode == CachePersist {
			if _, err := res.PersistCache(cache); err != nil {
				return out, err
			}
			res.archiveBytes = nil
			res.fromCache = true
			res.metaWriteNeeded = false
		}
		return res, nil
	}

	// Registry index JSON resolution.
	indexBytes, err := FetchRemoteBytes(ctx, client, sourceURL, MaxIndexBytes)
	if err != nil {
		return out, err
	}
	idx, err := ParseRegistryIndex(indexBytes, sourceURL)
	if err != nil {
		return out, err
	}
	entry, err := selectIndexEntry(idx.Packages, kitID, wantVersion)
	if err != nil {
		return out, err
	}
	if err := CheckMinCLIVersion(cliVersion, entry.MinCLIVersion, entry.ID+" "+entry.Version); err != nil {
		return out, err
	}
	if wantArchiveSHA256 != "" && !strings.EqualFold(wantArchiveSHA256, entry.ArchiveSHA256) {
		return out, apperr.Conflict("archive_digest_mismatch", fmt.Sprintf("pinned SHA-256 %s conflicts with registry entry %s", wantArchiveSHA256, entry.ArchiveSHA256))
	}

	expectedArchiveSHA := entry.ArchiveSHA256
	var raw []byte
	fromCache := false
	if cachedRaw, found, cErr := cache.Get(expectedArchiveSHA); cErr != nil {
		return out, cErr
	} else if found {
		raw = cachedRaw
		fromCache = true
	} else {
		raw, err = FetchRemoteBytes(ctx, client, entry.URL, MaxArchiveBytes)
		if err != nil {
			return out, err
		}
	}

	existingMeta, metaFound, mErr := cache.GetMetadata(expectedArchiveSHA)
	if mErr != nil {
		return out, mErr
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

	effectiveMin, err := maxMinCLIVersion(entry.MinCLIVersion, pkg.MinCLIVersion)
	if err != nil {
		return out, err
	}
	effectiveMin, err = maxMinCLIVersion(existingMeta.MinCLIVersion, effectiveMin)
	if err != nil {
		return out, err
	}
	if err := CheckMinCLIVersion(cliVersion, effectiveMin, pkg.ID+" "+pkg.Version); err != nil {
		return out, err
	}

	var pendingMeta *CacheMetadata
	metaNeedWrite := false
	if effectiveMin != "" {
		pendingMeta = &CacheMetadata{
			SchemaVersion: 1,
			ArchiveSHA256: archSHA,
			PackageDigest: pkgDigest,
			MinCLIVersion: effectiveMin,
		}
		metaNeedWrite = !metaFound || existingMeta.MinCLIVersion != effectiveMin
	}

	res := ResolvedPackage{
		Package:         pkg,
		Source:          snap,
		PackageDigest:   pkgDigest,
		ArchiveSHA256:   archSHA,
		SourceKind:      "remote",
		fromCache:       fromCache,
		pendingMeta:     pendingMeta,
		metaWriteNeeded: metaNeedWrite,
	}
	if !fromCache {
		res.archiveBytes = raw
	}
	if mode == CachePersist {
		if _, err := res.PersistCache(cache); err != nil {
			return out, err
		}
		res.archiveBytes = nil
		res.fromCache = true
		res.metaWriteNeeded = false
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

// CheckMinCLIVersion enforces minVersion against cliVersion with uniform semantics across index, archive, cached, offline, and local sources.
func CheckMinCLIVersion(cliVersion, minVersion, targetLabel string) error {
	if minVersion == "" {
		return nil
	}
	ok, err := SatisfiesMinCLI(cliVersion, minVersion)
	if err != nil {
		return apperr.Wrap(apperr.ExitUsage, "invalid_cli_version", fmt.Sprintf("cannot verify minCliVersion %s against CLI version %q: %v", minVersion, cliVersion, err), err)
	}
	if !ok {
		return apperr.Usage("cli_upgrade_required", fmt.Sprintf("%s requires dtkit >= %s (current %s)", targetLabel, minVersion, cliVersion))
	}
	return nil
}

// SatisfiesMinCLI checks whether current meets the strict X.Y.Z minVersion compatibility floor.
// Prerelease/dev builds formatted as X.Y.Z-<prerelease> (for example "0.4.0-dev" or "0.4.0-rc.1")
// evaluate against their base X.Y.Z floor so release candidates can install packages targeting X.Y.Z,
// while malformed versions without valid X.Y.Z base fail with an error.
func SatisfiesMinCLI(current, min string) (bool, error) {
	cur, err := parseCLIVersionTuple(current)
	if err != nil {
		return false, err
	}
	req, err := parseSemverTuple(min)
	if err != nil {
		return false, err
	}
	return compareTuples(cur, req) >= 0, nil
}

func maxMinCLIVersion(a, b string) (string, error) {
	if a == "" {
		return b, nil
	}
	if b == "" {
		return a, nil
	}
	ta, err := parseSemverTuple(a)
	if err != nil {
		return "", err
	}
	tb, err := parseSemverTuple(b)
	if err != nil {
		return "", err
	}
	if compareTuples(tb, ta) > 0 {
		return b, nil
	}
	return a, nil
}

func parseCLIVersionTuple(v string) ([3]int, error) {
	base, pre, hasPre := strings.Cut(v, "-")
	if hasPre && !prereleasePattern.MatchString(pre) {
		return [3]int{}, fmt.Errorf("invalid CLI prerelease version %q", v)
	}
	return parseSemverTuple(base)
}

func parseSemverTuple(v string) ([3]int, error) {
	var out [3]int
	if !catalog.IsValidVersion(v) {
		return out, fmt.Errorf("invalid semver %q", v)
	}
	parts := strings.Split(v, ".")
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
