package project

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

// InitOptions configures dtkit project init behavior.
type InitOptions struct {
	Path   string
	DryRun bool
}

// SyncOptions configures non-destructive synchronization of .dtkit/project.json.
type SyncOptions struct {
	Path   string
	DryRun bool
}

// SyncResult reports the outcome of SyncProfile.
type SyncResult struct {
	Root               string   `json:"root"`
	ConfigPath         string   `json:"configPath"`
	Created            bool     `json:"created"`
	Updated            bool     `json:"updated"`
	DryRun             bool     `json:"dryRun"`
	AddedRoots         []string `json:"addedRoots,omitempty"`
	NewSuggestedChecks int      `json:"newSuggestedChecks,omitempty"`
	Profile            Profile  `json:"profile"`
}

// InitResult reports the outcome of dtkit project init.
type InitResult struct {
	Root        string  `json:"root"`
	ConfigPath  string  `json:"configPath"`
	Created     bool    `json:"created"`
	DryRun      bool    `json:"dryRun"`
	TrustDigest string  `json:"trustDigest"`
	PreviewJSON string  `json:"previewJson,omitempty"`
	Profile     Profile `json:"profile"`
}

// LoadProfile reads and strictly validates .dtkit/project.json under repoRoot.
func LoadProfile(root string) (Profile, string, bool, error) {
	cfgPath, err := paths.ResolveTarget(root, ProfileRelPath)
	if err != nil {
		return Profile{}, "", false, err
	}
	data, exists, err := readOptionalRegular(root, ProfileRelPath)
	if err != nil {
		return Profile{}, cfgPath, false, err
	}
	if !exists {
		return Profile{}, cfgPath, false, nil
	}
	p, err := ParseAndValidate(data)
	if err != nil {
		return Profile{}, cfgPath, true, err
	}
	return p, cfgPath, true, nil
}

// Init generates and optionally writes .dtkit/project.json without ever overwriting an existing profile.
func Init(ctx context.Context, env paths.Environment, opts InitOptions) (InitResult, error) {
	insp, err := Inspect(ctx, env, opts.Path)
	if err != nil {
		return InitResult{}, err
	}
	if insp.ProfileExists {
		return InitResult{}, apperr.WithHint(
			apperr.WithPath(
				apperr.Conflict("project_profile_exists", fmt.Sprintf("%s already exists; refusing to overwrite existing user profile", ProfileRelPath)),
				ProfileRelPath,
			),
			"edit .dtkit/project.json manually or remove it before re-running dtkit project init",
		)
	}

	formatted, err := marshalProfile(insp.Profile)
	if err != nil {
		return InitResult{}, err
	}

	res := InitResult{
		Root:        insp.Root,
		ConfigPath:  insp.ConfigPath,
		DryRun:      opts.DryRun,
		TrustDigest: insp.TrustDigest,
		PreviewJSON: string(formatted),
		Profile:     insp.Profile,
	}
	if opts.DryRun {
		return res, nil
	}

	if err := writeProfileAtomic(insp.ConfigPath, formatted); err != nil {
		return InitResult{}, err
	}
	res.Created = true
	return res, nil
}

// SyncProfile creates .dtkit/project.json if absent, or non-destructively merges newly discovered
// roots, generatedDirs, and evidence while keeping existing checks untouched for safety.
func SyncProfile(ctx context.Context, env paths.Environment, opts SyncOptions) (SyncResult, error) {
	insp, err := Inspect(ctx, env, opts.Path)
	if err != nil {
		return SyncResult{}, err
	}

	if !insp.ProfileExists {
		formatted, mErr := marshalProfile(insp.Profile)
		if mErr != nil {
			return SyncResult{}, mErr
		}
		res := SyncResult{
			Root:       insp.Root,
			ConfigPath: insp.ConfigPath,
			Created:    true,
			Updated:    true,
			DryRun:     opts.DryRun,
			Profile:    insp.Profile,
		}
		for _, r := range insp.Profile.Roots {
			res.AddedRoots = append(res.AddedRoots, r.Path+":"+r.Stack)
		}
		if opts.DryRun {
			return res, nil
		}
		if err := writeProfileAtomic(insp.ConfigPath, formatted); err != nil {
			return SyncResult{}, err
		}
		return res, nil
	}

	existingData, exists, rErr := readOptionalRegular(insp.Root, ProfileRelPath)
	if rErr != nil {
		return SyncResult{}, rErr
	}
	if !exists {
		return SyncResult{}, apperr.New(apperr.ExitInternal, "project_profile_disappeared", fmt.Sprintf("%s could not be read", ProfileRelPath))
	}
	existing, pErr := ParseAndValidate(existingData)
	if pErr != nil {
		return SyncResult{}, pErr
	}

	merged, addedRoots, newChecks := mergeProfiles(existing, insp.Profile)
	if vErr := ValidateProfile(merged); vErr != nil {
		return SyncResult{}, vErr
	}
	formatted, mErr := marshalProfile(merged)
	if mErr != nil {
		return SyncResult{}, mErr
	}
	changed := !bytes.Equal(bytes.TrimSpace(existingData), bytes.TrimSpace(formatted))

	res := SyncResult{
		Root:               insp.Root,
		ConfigPath:         insp.ConfigPath,
		Updated:            changed,
		DryRun:             opts.DryRun,
		AddedRoots:         addedRoots,
		NewSuggestedChecks: newChecks,
		Profile:            merged,
	}
	if opts.DryRun || !changed {
		return res, nil
	}
	if err := writeProfileAtomic(insp.ConfigPath, formatted); err != nil {
		return SyncResult{}, err
	}
	return res, nil
}

func mergeProfiles(existing, discovered Profile) (Profile, []string, int) {
	merged := existing
	if merged.Name == "" {
		merged.Name = discovered.Name
	}

	rootIndex := make(map[string]int, len(existing.Roots))
	merged.Roots = make([]WorkspaceRoot, len(existing.Roots))
	for i, r := range existing.Roots {
		merged.Roots[i] = r
		rootIndex[r.Path+":"+r.Stack] = i
	}
	var addedRoots []string
	for _, r := range discovered.Roots {
		key := r.Path + ":" + r.Stack
		if idx, ok := rootIndex[key]; ok {
			if merged.Roots[idx].PackageManager == "" && r.PackageManager != "" {
				merged.Roots[idx].PackageManager = r.PackageManager
			}
			continue
		}
		rootIndex[key] = len(merged.Roots)
		merged.Roots = append(merged.Roots, r)
		addedRoots = append(addedRoots, key)
	}

	merged.GeneratedDirs = mergeUniqueStrings(existing.GeneratedDirs, discovered.GeneratedDirs)
	merged.Evidence = mergeEvidence(existing.Evidence, discovered.Evidence)

	existingCheckIDs := make(map[string]bool, len(existing.Checks))
	for _, c := range existing.Checks {
		existingCheckIDs[c.ID] = true
	}
	newSuggested := 0
	for _, c := range discovered.Checks {
		if !existingCheckIDs[c.ID] {
			newSuggested++
		}
	}
	return merged, addedRoots, newSuggested
}

func mergeUniqueStrings(base, extra []string) []string {
	seen := make(map[string]bool, len(base)+len(extra))
	out := make([]string, 0, len(base)+len(extra))
	for _, item := range base {
		if !seen[item] {
			seen[item] = true
			out = append(out, item)
		}
	}
	for _, item := range extra {
		if !seen[item] {
			seen[item] = true
			out = append(out, item)
		}
	}
	return out
}

func mergeEvidence(base, extra []EvidenceItem) []EvidenceItem {
	seen := make(map[string]bool, len(base)+len(extra))
	out := make([]EvidenceItem, 0, len(base)+len(extra))
	for _, ev := range base {
		k := ev.Source + "|" + ev.Summary
		if !seen[k] {
			seen[k] = true
			out = append(out, ev)
		}
	}
	for _, ev := range extra {
		k := ev.Source + "|" + ev.Summary
		if !seen[k] {
			seen[k] = true
			out = append(out, ev)
		}
	}
	return out
}

func marshalProfile(p Profile) ([]byte, error) {
	formatted, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(formatted, '\n'), nil
}

func writeProfileAtomic(configPath string, formatted []byte) error {
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(configPath), ".project.json.tmp.*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(formatted); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0644); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, configPath); err != nil {
		return err
	}
	cleanup = false
	return nil
}
