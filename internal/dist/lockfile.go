package dist

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

const (
	// LockfileRelPath is the default portable lockfile path at project root.
	LockfileRelPath = "dtkit.lock"
	// MaxLockfileBytes bounds dtkit.lock files.
	MaxLockfileBytes = 512 * 1024
)

// LockSource describes a portable source reference for a locked kit.
type LockSource struct {
	Kind string `json:"kind"` // "bundled" | "local" | "archive" | "remote"
	Ref  string `json:"ref,omitempty"`
}

// LockedKit pins exact kit version, content digests, selection, runtime compatibility, and optional provenance.
type LockedKit struct {
	ID               string                 `json:"id"`
	Version          string                 `json:"version"`
	PackageDigest    string                 `json:"packageDigest"`
	ArchiveSHA256    string                 `json:"archiveSha256,omitempty"`
	Source           LockSource             `json:"source"`
	RuntimeTargets   []string               `json:"runtimeTargets"`
	AdapterRevisions map[string]string      `json:"adapterRevisions,omitempty"`
	Selection        *catalog.SelectionSpec `json:"selection,omitempty"`
	Provenance       *ProvenanceRecord      `json:"provenance,omitempty"`
}

// Lockfile is a portable project declaration separate from machine-local state.json.
type Lockfile struct {
	SchemaVersion int         `json:"schemaVersion"`
	Kits          []LockedKit `json:"kits"`
}

// LockDiffChange describes one kit difference between two lockfiles.
type LockDiffChange struct {
	KitID             string `json:"kitId"`
	Status            string `json:"status"` // "added" | "removed" | "updated"
	BeforeVersion     string `json:"beforeVersion,omitempty"`
	AfterVersion      string `json:"afterVersion,omitempty"`
	BeforeSource      string `json:"beforeSource,omitempty"`
	AfterSource       string `json:"afterSource,omitempty"`
	SourceChanged     bool   `json:"sourceChanged,omitempty"`
	VersionChanged    bool   `json:"versionChanged,omitempty"`
	DigestChanged     bool   `json:"digestChanged,omitempty"`
	SelectionChanged  bool   `json:"selectionChanged,omitempty"`
	ProvenanceChanged bool   `json:"provenanceChanged,omitempty"`
}

// LockDiffReport summarizes changes for human review of dtkit.lock updates.
type LockDiffReport struct {
	Changes []LockDiffChange `json:"changes"`
}

// ParseAndValidateLockfile strictly decodes and validates portable dtkit.lock JSON.
func ParseAndValidateLockfile(data []byte) (Lockfile, error) {
	var lf Lockfile
	if len(data) > MaxLockfileBytes {
		return lf, apperr.Usage("lockfile_too_large", fmt.Sprintf("dtkit.lock exceeds %d byte limit", MaxLockfileBytes))
	}
	if err := catalog.CheckStrictJSON(data); err != nil {
		return lf, apperr.Wrap(apperr.ExitUsage, "invalid_lockfile", fmt.Sprintf("dtkit.lock JSON error: %v", err), err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return lf, apperr.Wrap(apperr.ExitUsage, "invalid_lockfile", "dtkit.lock must be a JSON object", err)
	}
	if err := catalog.CheckObjectKeys(top, []string{"schemaVersion", "kits"}, nil); err != nil {
		return lf, apperr.Usage("invalid_lockfile", fmt.Sprintf("dtkit.lock schema: %v", err))
	}
	var rawKits []map[string]json.RawMessage
	if err := json.Unmarshal(top["kits"], &rawKits); err != nil {
		return lf, apperr.Usage("invalid_lockfile", "dtkit.lock kits must be an array of objects")
	}
	for i, rk := range rawKits {
		if err := catalog.CheckObjectKeys(rk, []string{"id", "version", "packageDigest", "source", "runtimeTargets"}, []string{"archiveSha256", "adapterRevisions", "selection", "provenance"}); err != nil {
			return lf, apperr.Usage("invalid_lockfile", fmt.Sprintf("locked kit %d: %v", i, err))
		}
		var srcMap map[string]json.RawMessage
		if err := json.Unmarshal(rk["source"], &srcMap); err != nil {
			return lf, apperr.Usage("invalid_lockfile", fmt.Sprintf("locked kit %d source must be an object", i))
		}
		if err := catalog.CheckObjectKeys(srcMap, []string{"kind"}, []string{"ref"}); err != nil {
			return lf, apperr.Usage("invalid_lockfile", fmt.Sprintf("locked kit %d source: %v", i, err))
		}
		if rawProv, ok := rk["provenance"]; ok {
			var provMap map[string]json.RawMessage
			if err := json.Unmarshal(rawProv, &provMap); err != nil {
				return lf, apperr.Usage("invalid_lockfile", fmt.Sprintf("locked kit %d provenance must be an object", i))
			}
			if err := catalog.CheckObjectKeys(provMap, []string{"keyId", "algorithm", "signedDigest", "signature"}, nil); err != nil {
				return lf, apperr.Usage("invalid_lockfile", fmt.Sprintf("locked kit %d provenance: %v", i, err))
			}
		}
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&lf); err != nil || lf.SchemaVersion != 1 || lf.Kits == nil {
		return Lockfile{}, apperr.Usage("invalid_lockfile", "dtkit.lock must be valid JSON with schemaVersion 1")
	}

	seen := make(map[string]struct{}, len(lf.Kits))
	for i := range lf.Kits {
		if err := validateLockedKit(&lf.Kits[i]); err != nil {
			return Lockfile{}, err
		}
		if _, dup := seen[lf.Kits[i].ID]; dup {
			return Lockfile{}, apperr.Usage("duplicate_locked_kit", fmt.Sprintf("dtkit.lock contains duplicate kit %q", lf.Kits[i].ID))
		}
		seen[lf.Kits[i].ID] = struct{}{}
	}
	return lf, nil
}

func validateLockedKit(k *LockedKit) error {
	if !catalog.IsValidID(k.ID, 32) {
		return apperr.Usage("invalid_lockfile", fmt.Sprintf("invalid locked kit id %q", k.ID))
	}
	if !catalog.IsValidVersion(k.Version) {
		return apperr.Usage("invalid_lockfile", fmt.Sprintf("locked kit %s has invalid semver %q", k.ID, k.Version))
	}
	normPkg, err := normalizeSHA256(k.PackageDigest)
	if err != nil {
		return apperr.Wrap(apperr.ExitUsage, "invalid_lockfile", fmt.Sprintf("locked kit %s packageDigest is invalid", k.ID), err)
	}
	k.PackageDigest = normPkg
	if k.ArchiveSHA256 != "" {
		normArch, err := normalizeSHA256(k.ArchiveSHA256)
		if err != nil {
			return apperr.Wrap(apperr.ExitUsage, "invalid_lockfile", fmt.Sprintf("locked kit %s archiveSha256 is invalid", k.ID), err)
		}
		k.ArchiveSHA256 = normArch
	}

	switch k.Source.Kind {
	case "bundled":
		if k.Source.Ref != "" {
			return apperr.Usage("invalid_lockfile", fmt.Sprintf("locked kit %s bundled source must not carry ref", k.ID))
		}
	case "local", "archive":
		if k.Source.Ref == "" || isNonPortablePath(k.Source.Ref) || !paths.ValidateRelativePath(k.Source.Ref) {
			return apperr.Usage("non_portable_lock_ref", fmt.Sprintf("locked kit %s %s ref %q must be a clean project-relative POSIX path without home/absolute references", k.ID, k.Source.Kind, k.Source.Ref))
		}
		if k.Source.Kind == "archive" && k.ArchiveSHA256 == "" {
			return apperr.Usage("invalid_lockfile", fmt.Sprintf("locked kit %s archive source requires archiveSha256", k.ID))
		}
	case "remote":
		if k.Source.Ref == "" {
			return apperr.Usage("invalid_lockfile", fmt.Sprintf("locked kit %s remote source requires url ref", k.ID))
		}
		if err := ValidateRemoteURL(k.Source.Ref); err != nil {
			return apperr.Wrap(apperr.ExitUsage, "non_portable_lock_ref", fmt.Sprintf("locked kit %s remote ref is invalid or non-portable", k.ID), err)
		}
		if hasURLQuerySecrets(k.Source.Ref) {
			return apperr.Usage("non_portable_lock_ref", fmt.Sprintf("locked kit %s remote ref must not contain query tokens or credentials", k.ID))
		}
		if k.ArchiveSHA256 == "" {
			return apperr.Usage("invalid_lockfile", fmt.Sprintf("locked kit %s remote source requires archiveSha256", k.ID))
		}
	default:
		return apperr.Usage("invalid_lockfile", fmt.Sprintf("locked kit %s has unsupported source kind %q", k.ID, k.Source.Kind))
	}

	if len(k.RuntimeTargets) == 0 {
		return apperr.Usage("invalid_lockfile", fmt.Sprintf("locked kit %s requires runtimeTargets", k.ID))
	}
	seenRT := map[string]bool{}
	for _, rt := range k.RuntimeTargets {
		if _, ok := adapter.Profile(rt); !ok || seenRT[rt] {
			return apperr.Usage("invalid_lockfile", fmt.Sprintf("locked kit %s has invalid or duplicate runtimeTarget %q", k.ID, rt))
		}
		seenRT[rt] = true
	}
	if k.Selection != nil && len(k.Selection.ResolvedComponents) == 0 {
		return apperr.Usage("invalid_lockfile", fmt.Sprintf("locked kit %s selection must include resolvedComponents", k.ID))
	}
	if k.Provenance != nil {
		if _, err := normalizeSHA256(k.Provenance.SignedDigest); err != nil {
			return apperr.Wrap(apperr.ExitUsage, "invalid_lockfile", fmt.Sprintf("locked kit %s provenance signedDigest is invalid", k.ID), err)
		}
	}
	return nil
}

func isNonPortablePath(p string) bool {
	if filepath.IsAbs(p) || strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`) || strings.HasPrefix(p, "~") {
		return true
	}
	if len(p) >= 2 && p[1] == ':' && ((p[0] >= 'a' && p[0] <= 'z') || (p[0] >= 'A' && p[0] <= 'Z')) {
		return true
	}
	return false
}

func hasURLQuerySecrets(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return true
	}
	if u.User != nil {
		return true
	}
	q := strings.ToLower(u.RawQuery)
	for _, bad := range []string{"token=", "sig=", "signature=", "key=", "secret=", "password=", "auth="} {
		if strings.Contains(q, bad) {
			return true
		}
	}
	return false
}

// MarshalLockfile writes canonical sorted dtkit.lock bytes.
func MarshalLockfile(lf Lockfile) ([]byte, error) {
	copyLF := Lockfile{
		SchemaVersion: 1,
		Kits:          make([]LockedKit, len(lf.Kits)),
	}
	copy(copyLF.Kits, lf.Kits)
	for i := range copyLF.Kits {
		rts := append([]string(nil), copyLF.Kits[i].RuntimeTargets...)
		sort.Strings(rts)
		copyLF.Kits[i].RuntimeTargets = rts
	}
	sort.Slice(copyLF.Kits, func(i, j int) bool {
		return copyLF.Kits[i].ID < copyLF.Kits[j].ID
	})
	b, err := json.MarshalIndent(copyLF, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// LockTargetDigest returns the canonical SHA-256 digest protected by provenance signatures.
func (k LockedKit) LockTargetDigest() string {
	if k.ArchiveSHA256 != "" {
		return k.ArchiveSHA256
	}
	return k.PackageDigest
}

// CheckRuntimeCompatibility ensures runtimeName matches locked RuntimeTargets and optional AdapterRevisions.
func (k LockedKit) CheckRuntimeCompatibility(runtimeName string) error {
	profile, ok := adapter.Profile(runtimeName)
	if !ok {
		return apperr.Usage("unsupported_runtime", fmt.Sprintf("unsupported runtime %q", runtimeName))
	}
	supported := false
	for _, rt := range k.RuntimeTargets {
		if rt == profile.ID {
			supported = true
			break
		}
	}
	if !supported {
		return apperr.Conflict("runtime_not_in_lock", fmt.Sprintf("locked kit %s does not declare runtime target %q", k.ID, profile.ID))
	}
	if rev, ok := k.AdapterRevisions[profile.ID]; ok && rev != "" && rev != profile.Revision {
		return apperr.Conflict("adapter_revision_mismatch", fmt.Sprintf("locked kit %s expects %s adapter revision %s, current CLI uses %s", k.ID, profile.ID, rev, profile.Revision))
	}
	return nil
}

// ResolveLockedKit loads and verifies exact pinned bytes for entry (bundled, project-relative local/archive,
// verified cache hit, or HTTPS remote) before any workspace mutation.
func ResolveLockedKit(ctx context.Context, client *http.Client, cache Cache, projectRoot string, entry LockedKit, cliVersion string, offline bool, mode CacheMode) (ResolvedPackage, error) {
	var out ResolvedPackage
	switch entry.Source.Kind {
	case "bundled":
		if entry.ID != "engineer" {
			return out, apperr.Conflict("source_unavailable", fmt.Sprintf("locked kit %s is not in bundled catalog", entry.ID))
		}
		pkg, source, err := catalog.Engineer()
		if err != nil {
			return out, err
		}
		dig, err := catalog.Digest(source)
		if err != nil {
			return out, err
		}
		out = ResolvedPackage{
			Package:       pkg,
			Source:        source,
			PackageDigest: dig,
			SourceKind:    "bundled",
		}
	case "local", "archive":
		if projectRoot == "" {
			return out, apperr.Usage("missing_project_root", "project root is required to resolve relative lock source")
		}
		target, err := paths.ResolveTarget(projectRoot, entry.Source.Ref)
		if err != nil {
			return out, err
		}
		out, err = ResolveFromPath(target, entry.ID, entry.Version, entry.ArchiveSHA256)
		if err != nil {
			if os.IsNotExist(err) {
				return out, apperr.WithPath(apperr.Conflict("source_unavailable", fmt.Sprintf("locked %s source for %s is missing", entry.Source.Kind, entry.ID)), entry.Source.Ref)
			}
			return out, err
		}
	case "remote":
		// Reproducibility & offline policy: try verified SHA-256 cache first before touching network.
		if cachedRaw, found, cErr := cache.Get(entry.ArchiveSHA256); cErr != nil {
			return out, cErr
		} else if found {
			existingMeta, _, mErr := cache.GetMetadata(entry.ArchiveSHA256)
			if mErr != nil {
				return out, mErr
			}
			pkg, snap, dig, archSHA, rErr := ReadArchiveBytes(cachedRaw, entry.ArchiveSHA256)
			if rErr != nil {
				return out, rErr
			}
			if existingMeta.MinCLIVersion != "" {
				if err := CheckMinCLIVersion(cliVersion, existingMeta.MinCLIVersion, pkg.ID+" "+pkg.Version); err != nil {
					return out, err
				}
			}
			out = ResolvedPackage{
				Package:       pkg,
				Source:        snap,
				PackageDigest: dig,
				ArchiveSHA256: archSHA,
				SourceKind:    "remote",
				fromCache:     true,
			}
		} else if offline {
			return out, apperr.WithHint(
				apperr.Conflict("offline_cache_miss", fmt.Sprintf("locked remote kit %s %s (%s) is not in verified cache in --offline mode", entry.ID, entry.Version, entry.ArchiveSHA256[:12])),
				"populate cache or re-run without --offline",
			)
		} else {
			res, err := ResolveRemote(ctx, client, cache, entry.ID, entry.Version, entry.Source.Ref, entry.ArchiveSHA256, cliVersion, false, mode)
			if err != nil {
				return out, apperr.Wrap(apperr.ExitConflict, "source_unavailable", fmt.Sprintf("unable to resolve locked remote kit %s %s", entry.ID, entry.Version), err)
			}
			out = res
		}
	default:
		return out, apperr.Usage("invalid_lockfile", fmt.Sprintf("unsupported lock source kind %q", entry.Source.Kind))
	}

	if err := CheckMinCLIVersion(cliVersion, out.Package.MinCLIVersion, out.Package.ID+" "+out.Package.Version); err != nil {
		return ResolvedPackage{}, err
	}
	if out.Package.ID != entry.ID {
		return ResolvedPackage{}, apperr.Conflict("kit_id_mismatch", fmt.Sprintf("resolved kit %q does not match locked %q", out.Package.ID, entry.ID))
	}
	if out.Package.Version != entry.Version {
		return ResolvedPackage{}, apperr.Conflict("version_mismatch", fmt.Sprintf("resolved %s version %s does not match locked %s", entry.ID, out.Package.Version, entry.Version))
	}
	if !strings.EqualFold(out.PackageDigest, entry.PackageDigest) {
		return ResolvedPackage{}, apperr.Conflict("lock_digest_mismatch", fmt.Sprintf("resolved %s packageDigest %s does not match lockfile %s", entry.ID, out.PackageDigest, entry.PackageDigest))
	}
	if entry.ArchiveSHA256 != "" && out.ArchiveSHA256 != "" && !strings.EqualFold(out.ArchiveSHA256, entry.ArchiveSHA256) {
		return ResolvedPackage{}, apperr.Conflict("archive_digest_mismatch", fmt.Sprintf("resolved %s archiveSha256 %s does not match lockfile %s", entry.ID, out.ArchiveSHA256, entry.ArchiveSHA256))
	}
	return out, nil
}

// DiffLockfiles summarizes semantic changes across versions, sources, digests, selections, and provenance.
func DiffLockfiles(before, after Lockfile) LockDiffReport {
	bMap := make(map[string]LockedKit, len(before.Kits))
	for _, k := range before.Kits {
		bMap[k.ID] = k
	}
	aMap := make(map[string]LockedKit, len(after.Kits))
	for _, k := range after.Kits {
		aMap[k.ID] = k
	}

	idsMap := map[string]bool{}
	for id := range bMap {
		idsMap[id] = true
	}
	for id := range aMap {
		idsMap[id] = true
	}
	ids := make([]string, 0, len(idsMap))
	for id := range idsMap {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	rep := LockDiffReport{Changes: make([]LockDiffChange, 0, len(ids))}
	for _, id := range ids {
		b, hasB := bMap[id]
		a, hasA := aMap[id]
		switch {
		case !hasB && hasA:
			rep.Changes = append(rep.Changes, LockDiffChange{
				KitID:        id,
				Status:       "added",
				AfterVersion: a.Version,
				AfterSource:  formatLockSource(a.Source),
			})
		case hasB && !hasA:
			rep.Changes = append(rep.Changes, LockDiffChange{
				KitID:         id,
				Status:        "removed",
				BeforeVersion: b.Version,
				BeforeSource:  formatLockSource(b.Source),
			})
		default:
			srcCh := formatLockSource(b.Source) != formatLockSource(a.Source)
			verCh := b.Version != a.Version
			digCh := b.PackageDigest != a.PackageDigest || b.ArchiveSHA256 != a.ArchiveSHA256
			selCh := !catalog.SelectionsEqual(b.Selection, a.Selection)
			provCh := !provenanceEqual(b.Provenance, a.Provenance)
			if srcCh || verCh || digCh || selCh || provCh {
				rep.Changes = append(rep.Changes, LockDiffChange{
					KitID:             id,
					Status:            "updated",
					BeforeVersion:     b.Version,
					AfterVersion:      a.Version,
					BeforeSource:      formatLockSource(b.Source),
					AfterSource:       formatLockSource(a.Source),
					SourceChanged:     srcCh,
					VersionChanged:    verCh,
					DigestChanged:     digCh,
					SelectionChanged:  selCh,
					ProvenanceChanged: provCh,
				})
			}
		}
	}
	return rep
}

func formatLockSource(s LockSource) string {
	if s.Ref == "" {
		return s.Kind
	}
	return s.Kind + ":" + s.Ref
}

func provenanceEqual(a, b *ProvenanceRecord) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
