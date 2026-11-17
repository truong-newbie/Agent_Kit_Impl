package adapter

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
)

// CompatibilityEntry snapshots one (runtime, scope) contract and rendered engineer artifact digest.
type CompatibilityEntry struct {
	Runtime             string              `json:"runtime"`
	Scope               string              `json:"scope"`
	AdapterRevision     string              `json:"adapterRevision"`
	Binary              string              `json:"binary"`
	InstructionStrategy string              `json:"instructionStrategy,omitempty"`
	InstructionPath     string              `json:"instructionPath,omitempty"`
	SkillPrefix         string              `json:"skillPrefix,omitempty"`
	HooksSupported      bool                `json:"hooksSupported"`
	HookConfigPath      string              `json:"hookConfigPath,omitempty"`
	Capabilities        Capabilities        `json:"capabilities"`
	Integration         IntegrationEvidence `json:"integration"`
	RenderedDigest      string              `json:"renderedDigest"`
	ArtifactCount       int                 `json:"artifactCount"`
}

// CompatibilityReport captures deterministic layout, capability, and version compatibility across all runtimes.
type CompatibilityReport struct {
	SchemaVersion int                  `json:"schemaVersion"`
	KitID         string               `json:"kitId"`
	KitVersion    string               `json:"kitVersion"`
	KitDigest     string               `json:"kitDigest"`
	Entries       []CompatibilityEntry `json:"entries"`
}

// CompatibilityChange records one difference between a baseline and candidate CompatibilityReport.
type CompatibilityChange struct {
	Runtime  string `json:"runtime"`
	Scope    string `json:"scope"`
	Status   string `json:"status"` // added | removed | changed
	Details  string `json:"details"`
	Breaking bool   `json:"breaking,omitempty"`
}

// CompatibilityDiff summarizes changes for maintainer review without auto-upgrading adapters.
type CompatibilityDiff struct {
	KitVersionChanged  bool                  `json:"kitVersionChanged,omitempty"`
	HasBreakingChanges bool                  `json:"hasBreakingChanges,omitempty"`
	ChangedCount       int                   `json:"changedCount"`
	Changes            []CompatibilityChange `json:"changes"`
}

// BuildCompatibilityReport produces a deterministic offline compatibility snapshot across all supported runtimes & scopes.
func BuildCompatibilityReport() (CompatibilityReport, error) {
	pkg, srcFS, err := catalog.Engineer()
	if err != nil {
		return CompatibilityReport{}, err
	}
	kitDig, err := catalog.Digest(srcFS)
	if err != nil {
		return CompatibilityReport{}, err
	}

	rep := CompatibilityReport{
		SchemaVersion: 1,
		KitID:         pkg.ID,
		KitVersion:    pkg.Version,
		KitDigest:     kitDig,
	}
	for _, rt := range SupportedRuntimes() {
		profile, ok := Profile(rt)
		if !ok {
			continue
		}
		for _, sc := range profile.Scopes {
			capPlan, err := EvaluateCapabilities(profile, sc, &pkg)
			if err != nil {
				return CompatibilityReport{}, err
			}
			arts, err := RenderForRuntime(rt, pkg, srcFS, sc)
			if err != nil {
				return CompatibilityReport{}, err
			}
			entry := CompatibilityEntry{
				Runtime:         profile.ID,
				Scope:           sc,
				AdapterRevision: profile.Revision,
				Binary:          profile.Binary,
				HooksSupported:  profile.HooksSupported,
				Capabilities:    capPlan.Capabilities,
				Integration:     capPlan.Integration,
				RenderedDigest:  digestRenderedArtifacts(arts),
				ArtifactCount:   len(arts),
			}
			if loc, hasInstr := profile.Instructions[sc]; hasInstr {
				entry.InstructionStrategy = loc.EffectiveStrategy()
				entry.InstructionPath = loc.RelPath(pkg.ID)
			}
			if sk, hasSkill := profile.Skills[sc]; hasSkill {
				entry.SkillPrefix = profile.SkillPathPrefix(sc)
				_ = sk
			}
			if profile.HooksSupported {
				if sc == "project" {
					entry.HookConfigPath = profile.HookProjectPath
				} else if profile.HookGlobalPath != "" {
					entry.HookConfigPath = profile.HookGlobalRoot + ":" + profile.HookGlobalPath
				}
			}
			rep.Entries = append(rep.Entries, entry)
		}
	}
	return rep, nil
}

func digestRenderedArtifacts(arts []Artifact) string {
	sorted := append([]Artifact(nil), arts...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Root != sorted[j].Root {
			return sorted[i].Root < sorted[j].Root
		}
		return sorted[i].Path < sorted[j].Path
	})
	h := sha256.New()
	for _, a := range sorted {
		_, _ = fmt.Fprintf(h, "%s\x00%s\x00%s\x00%d\n", a.Root, a.Path, a.Strategy, len(a.Content))
		h.Write(a.Content)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// DiffCompatibilityReports compares baseline vs current compatibility snapshots cleanly.
func DiffCompatibilityReports(baseline, current CompatibilityReport) CompatibilityDiff {
	diff := CompatibilityDiff{
		KitVersionChanged: baseline.KitVersion != current.KitVersion || baseline.KitDigest != current.KitDigest,
		Changes:           []CompatibilityChange{},
	}
	key := func(e CompatibilityEntry) string { return e.Runtime + "/" + e.Scope }
	bMap := make(map[string]CompatibilityEntry, len(baseline.Entries))
	for _, e := range baseline.Entries {
		bMap[key(e)] = e
	}
	cMap := make(map[string]CompatibilityEntry, len(current.Entries))
	for _, e := range current.Entries {
		cMap[key(e)] = e
	}

	seen := map[string]bool{}
	var allKeys []string
	for k := range bMap {
		seen[k] = true
		allKeys = append(allKeys, k)
	}
	for k := range cMap {
		if !seen[k] {
			allKeys = append(allKeys, k)
		}
	}
	sort.Strings(allKeys)

	for _, k := range allKeys {
		b, hasB := bMap[k]
		c, hasC := cMap[k]
		switch {
		case hasB && !hasC:
			diff.HasBreakingChanges = true
			diff.Changes = append(diff.Changes, CompatibilityChange{
				Runtime: b.Runtime, Scope: b.Scope, Status: "removed", Breaking: true,
				Details: "runtime/scope pair removed from supported matrix",
			})
		case !hasB && hasC:
			diff.Changes = append(diff.Changes, CompatibilityChange{
				Runtime: c.Runtime, Scope: c.Scope, Status: "added",
				Details: fmt.Sprintf("added %s/%s (hooks=%v, artifacts=%d)", c.Runtime, c.Scope, c.HooksSupported, c.ArtifactCount),
			})
		default:
			if b.AdapterRevision != c.AdapterRevision ||
				b.InstructionPath != c.InstructionPath ||
				b.InstructionStrategy != c.InstructionStrategy ||
				b.SkillPrefix != c.SkillPrefix ||
				b.HookConfigPath != c.HookConfigPath ||
				b.HooksSupported != c.HooksSupported ||
				b.RenderedDigest != c.RenderedDigest {
				breaking := b.AdapterRevision != c.AdapterRevision ||
					b.InstructionPath != c.InstructionPath ||
					b.SkillPrefix != c.SkillPrefix
				if breaking {
					diff.HasBreakingChanges = true
				}
				diff.Changes = append(diff.Changes, CompatibilityChange{
					Runtime:  c.Runtime,
					Scope:    c.Scope,
					Status:   "changed",
					Breaking: breaking,
					Details: fmt.Sprintf("rev:%s->%s instr:%s->%s skill:%s->%s hooks:%v(%s)->%v(%s) digest:%s->%s",
						b.AdapterRevision, c.AdapterRevision,
						b.InstructionPath, c.InstructionPath,
						b.SkillPrefix, c.SkillPrefix,
						b.HooksSupported, b.HookConfigPath,
						c.HooksSupported, c.HookConfigPath,
						shortDig(b.RenderedDigest), shortDig(c.RenderedDigest)),
				})
			}
		}
	}
	diff.ChangedCount = len(diff.Changes)
	return diff
}

func shortDig(d string) string {
	if len(d) <= 12 {
		return d
	}
	return d[:12]
}

// ParseCompatibilityReport decodes a committed CompatibilityReport JSON snapshot.
func ParseCompatibilityReport(data []byte) (CompatibilityReport, error) {
	var rep CompatibilityReport
	if err := json.Unmarshal(data, &rep); err != nil {
		return CompatibilityReport{}, err
	}
	if rep.SchemaVersion != 1 {
		return CompatibilityReport{}, fmt.Errorf("unsupported compatibility report schemaVersion %d", rep.SchemaVersion)
	}
	return rep, nil
}
