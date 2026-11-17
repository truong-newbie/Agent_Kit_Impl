package dist

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/project"
)

const (
	// TeamBundleRelPath is the default committed portable team onboarding config path.
	TeamBundleRelPath = ".dtkit/team.json"
	// OnboardingDocRelPath is the default repository onboarding guide path.
	OnboardingDocRelPath = ".dtkit/ONBOARDING.md"
	// MaxTeamBundleBytes bounds portable team onboarding bundles.
	MaxTeamBundleBytes = 1024 * 1024
)

var allowedHookRecommendations = map[string]bool{
	"session-context": true,
	"check-changed":   true,
	"guard-edit":      true,
}

// TeamBundle captures portable team-shared DTKit configuration (zero machine-local state, bindings, backups, or auth).
type TeamBundle struct {
	SchemaVersion       int              `json:"schemaVersion"`
	Name                string           `json:"name,omitempty"`
	DefaultRuntime      string           `json:"defaultRuntime,omitempty"`
	Runtimes            []string         `json:"runtimes"`
	Lockfile            Lockfile         `json:"lockfile"`
	TrustPolicy         *TrustPolicy     `json:"trustPolicy,omitempty"`
	ProjectProfile      *project.Profile `json:"projectProfile,omitempty"`
	HookRecommendations []string         `json:"hookRecommendations,omitempty"`
}

// ParseAndValidateTeamBundle strictly validates a portable TeamBundle JSON payload.
func ParseAndValidateTeamBundle(data []byte) (TeamBundle, error) {
	var bundle TeamBundle
	if len(data) > MaxTeamBundleBytes {
		return bundle, apperr.Usage("team_bundle_too_large", fmt.Sprintf("team bundle exceeds %d byte limit", MaxTeamBundleBytes))
	}
	if err := catalog.CheckStrictJSON(data); err != nil {
		return bundle, apperr.Wrap(apperr.ExitUsage, "invalid_team_bundle", fmt.Sprintf("team bundle JSON error: %v", err), err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return bundle, apperr.Wrap(apperr.ExitUsage, "invalid_team_bundle", "team bundle must be a JSON object", err)
	}
	if err := catalog.CheckObjectKeys(top, []string{"schemaVersion", "runtimes", "lockfile"}, []string{"name", "defaultRuntime", "trustPolicy", "projectProfile", "hookRecommendations"}); err != nil {
		return bundle, apperr.Usage("invalid_team_bundle", fmt.Sprintf("team bundle schema: %v", err))
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&bundle); err != nil || bundle.SchemaVersion != 1 {
		return TeamBundle{}, apperr.Usage("invalid_team_bundle", "team bundle must be valid JSON with schemaVersion 1")
	}

	if len(bundle.Name) > 128 {
		return TeamBundle{}, apperr.Usage("invalid_team_bundle", "team bundle name exceeds 128 characters")
	}
	if bundle.DefaultRuntime != "" {
		if _, ok := adapter.Profile(bundle.DefaultRuntime); !ok {
			return TeamBundle{}, apperr.Usage("invalid_team_bundle", fmt.Sprintf("unsupported defaultRuntime %q", bundle.DefaultRuntime))
		}
	}
	if len(bundle.Runtimes) == 0 {
		return TeamBundle{}, apperr.Usage("invalid_team_bundle", "team bundle requires at least one runtime")
	}
	seenRT := map[string]bool{}
	for _, rt := range bundle.Runtimes {
		if _, ok := adapter.Profile(rt); !ok || seenRT[rt] {
			return TeamBundle{}, apperr.Usage("invalid_team_bundle", fmt.Sprintf("invalid or duplicate runtime %q", rt))
		}
		seenRT[rt] = true
	}

	// Re-validate nested sub-objects strictly through their canonical parsers.
	lfParsed, err := ParseAndValidateLockfile(top["lockfile"])
	if err != nil {
		return TeamBundle{}, err
	}
	bundle.Lockfile = lfParsed

	if rawTrust, ok := top["trustPolicy"]; ok && string(rawTrust) != "null" {
		pol, err := ParseAndValidateTrustPolicy(rawTrust)
		if err != nil {
			return TeamBundle{}, err
		}
		bundle.TrustPolicy = &pol
	}
	if rawProj, ok := top["projectProfile"]; ok && string(rawProj) != "null" {
		prof, err := project.ParseAndValidate(rawProj)
		if err != nil {
			return TeamBundle{}, err
		}
		bundle.ProjectProfile = &prof
	}

	seenHook := map[string]bool{}
	for _, h := range bundle.HookRecommendations {
		if !allowedHookRecommendations[h] || seenHook[h] {
			return TeamBundle{}, apperr.Usage("invalid_team_bundle", fmt.Sprintf("invalid or duplicate hookRecommendation %q", h))
		}
		seenHook[h] = true
	}
	return bundle, nil
}

// MarshalTeamBundle serializes bundle deterministically and validates the result.
func MarshalTeamBundle(bundle TeamBundle) ([]byte, error) {
	out := bundle
	out.SchemaVersion = 1
	out.Runtimes = append([]string(nil), bundle.Runtimes...)
	sort.Strings(out.Runtimes)
	if len(bundle.HookRecommendations) > 0 {
		out.HookRecommendations = append([]string(nil), bundle.HookRecommendations...)
		sort.Strings(out.HookRecommendations)
	}
	lfBytes, err := MarshalLockfile(bundle.Lockfile)
	if err != nil {
		return nil, err
	}
	lfSorted, err := ParseAndValidateLockfile(lfBytes)
	if err != nil {
		return nil, err
	}
	out.Lockfile = lfSorted

	if bundle.TrustPolicy != nil {
		tpBytes, err := MarshalTrustPolicy(*bundle.TrustPolicy)
		if err != nil {
			return nil, err
		}
		tpSorted, err := ParseAndValidateTrustPolicy(tpBytes)
		if err != nil {
			return nil, err
		}
		out.TrustPolicy = &tpSorted
	}

	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, err
	}
	b = append(b, '\n')
	if _, err := ParseAndValidateTeamBundle(b); err != nil {
		return nil, err
	}
	return b, nil
}

// OnboardingGuideMarkdown renders a clean, portable repository onboarding markdown template.
func OnboardingGuideMarkdown(bundle TeamBundle) string {
	var b strings.Builder
	title := strings.TrimSpace(bundle.Name)
	if title == "" {
		title = "Repository DTKit Onboarding"
	}
	b.WriteString("# " + title + "\n\n")
	b.WriteString("This repository uses **DTKit** to install reproducible AI agent kits across supported coding runtimes.\n\n")
	b.WriteString("## Supported Runtimes\n\n")
	b.WriteString("- Targets: `" + strings.Join(bundle.Runtimes, "`, `") + "`\n")
	if bundle.DefaultRuntime != "" {
		b.WriteString("- Default runtime: `" + bundle.DefaultRuntime + "`\n")
	}
	b.WriteString("\n## Quick Start (Zero Auto-Execution)\n\n")
	b.WriteString("```bash\n")
	rtFlag := ""
	if bundle.DefaultRuntime != "" && bundle.DefaultRuntime != "codex" {
		rtFlag = " --runtime " + bundle.DefaultRuntime
	}
	b.WriteString("dtkit status --all-runtimes\n")
	b.WriteString("dtkit lock verify\n")
	b.WriteString("dtkit sync" + rtFlag + " --dry-run --diff\n")
	b.WriteString("dtkit sync" + rtFlag + " --yes\n")
	b.WriteString("```\n")

	if bundle.ProjectProfile != nil {
		b.WriteString("\n## Project Verification Checks (`.dtkit/project.json`)\n\n")
		b.WriteString("DTKit never executes repository check commands automatically during `sync` or `import`.\n")
		b.WriteString("Inspect `.dtkit/project.json` first, then run checks interactively once to record local trust:\n\n")
		b.WriteString("```bash\n")
		b.WriteString("dtkit project inspect\n")
		b.WriteString("dtkit check --changed --dry-run\n")
		b.WriteString("dtkit check --changed\n")
		b.WriteString("```\n")
	}

	if len(bundle.HookRecommendations) > 0 {
		b.WriteString("\n## Recommended Opt-In Runtime Hooks\n\n")
		for _, h := range bundle.HookRecommendations {
			b.WriteString("- `" + h + "` (enable explicitly with `dtkit hooks enable " + h + " --project`)\n")
		}
	}

	b.WriteString("\n## Git Hygiene\n\n")
	b.WriteString("Keep `.dtkit/local/` (locks, backups, check trust, transaction journals) ignored in `.gitignore`, while committing `dtkit.lock`, `.dtkit/trust.json`, `.dtkit/project.json`, and `.dtkit/team.json`.\n")
	return b.String()
}
