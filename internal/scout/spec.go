package scout

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

// SpecReport summarizes read-only structural and clarity validation of a feature specification.
type SpecReport struct {
	Root                     string   `json:"root"`
	SpecPath                 string   `json:"specPath,omitempty"`
	Status                   string   `json:"status"` // "valid" | "needs_clarification" | "incomplete" | "no_spec"
	FunctionalRequirements   []string `json:"functionalRequirements,omitempty"`
	SuccessCriteria          []string `json:"successCriteria,omitempty"`
	UnresolvedClarifications []string `json:"unresolvedClarifications,omitempty"`
	HasUserScenarios         bool     `json:"hasUserScenarios"`
	HasAcceptanceScenarios   bool     `json:"hasAcceptanceScenarios"`
	MissingSections          []string `json:"missingSections,omitempty"`
}

var (
	specSlugRe       = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	frIDRe           = regexp.MustCompile(`\bFR-\d{2,}\b`)
	scIDRe           = regexp.MustCompile(`\bSC-\d{2,}\b`)
	clarificationRe  = regexp.MustCompile(`\[NEEDS CLARIFICATION(?::?\s*([^\]]*))?\]`)
	givenRe          = regexp.MustCompile(`(?i)\*\*given\*\*|\bgiven\b`)
	whenRe           = regexp.MustCompile(`(?i)\*\*when\*\*|\bwhen\b`)
	thenRe           = regexp.MustCompile(`(?i)\*\*then\*\*|\bthen\b`)
	candidateSpecsRe = []string{"docs/specs", "specs"}
)

// InspectSpec analyzes a feature specification markdown file strictly read-only.
func InspectSpec(ctx context.Context, env paths.Environment, rawRoot, specRel string) (SpecReport, error) {
	if err := ctx.Err(); err != nil {
		return SpecReport{}, apperr.Cancelled("spec check cancelled")
	}
	projRes, err := paths.ResolveProject(ctx, env, rawRoot)
	if err != nil {
		return SpecReport{}, err
	}
	root := projRes.Root
	rep := SpecReport{Root: root, Status: "no_spec"}

	resolvedRel, specAbs, err := resolveSpecFile(root, specRel)
	if err != nil {
		return SpecReport{}, err
	}
	if resolvedRel == "" {
		return rep, nil
	}
	raw, err := os.ReadFile(specAbs)
	if err != nil {
		return SpecReport{}, err
	}
	return analyzeSpecMarkdown(root, resolvedRel, string(raw)), nil
}

// InitSpec scaffolds a new structured feature specification under docs/specs/<slug>.md
// using the bundled engineer template without ever overwriting an existing spec file.
func InitSpec(ctx context.Context, env paths.Environment, rawRoot, slug, title string, dryRun bool) (string, string, error) {
	if err := ctx.Err(); err != nil {
		return "", "", apperr.Cancelled("spec init cancelled")
	}
	cleanSlug := strings.TrimSpace(slug)
	if !specSlugRe.MatchString(cleanSlug) {
		return "", "", apperr.Usage("invalid_spec_slug", fmt.Sprintf("invalid spec slug %q (expected lowercase alphanumeric/hyphen, e.g. auth-sso)", slug))
	}
	projRes, err := paths.ResolveProject(ctx, env, rawRoot)
	if err != nil {
		return "", "", err
	}
	root := projRes.Root
	relPath := filepath.ToSlash(filepath.Join("docs", "specs", cleanSlug+".md"))
	if !paths.ValidateRelativePath(relPath) {
		return "", "", apperr.WithPath(apperr.Usage("invalid_spec_path", "spec path escapes workspace root"), relPath)
	}
	absTarget, err := paths.ResolveTarget(root, relPath)
	if err != nil {
		return "", "", err
	}
	if info, statErr := os.Lstat(absTarget); statErr == nil {
		if paths.IsReparseOrSymlink(info) {
			return "", "", apperr.WithPath(apperr.Conflict("symlink_target", fmt.Sprintf("refusing symlinked %s", relPath)), relPath)
		}
		return "", "", apperr.WithPath(apperr.Conflict("spec_already_exists", fmt.Sprintf("spec file %s already exists", relPath)), relPath)
	} else if !os.IsNotExist(statErr) {
		return "", "", statErr
	}

	_, kitFS, err := catalog.Engineer()
	if err != nil {
		return "", "", err
	}
	tmplBytes, err := fs.ReadFile(kitFS, "templates/feature-spec.md")
	if err != nil {
		return "", "", err
	}
	featureTitle := strings.TrimSpace(title)
	if featureTitle == "" {
		featureTitle = titleFromSlug(cleanSlug)
	}
	rendered := strings.ReplaceAll(string(tmplBytes), "<Feature Name>", featureTitle)
	rendered = strings.ReplaceAll(rendered, "<feature-slug>", cleanSlug)

	if !dryRun {
		if err := os.MkdirAll(filepath.Dir(absTarget), 0755); err != nil {
			return "", "", err
		}
		if err := os.WriteFile(absTarget, []byte(rendered), 0644); err != nil {
			return "", "", err
		}
	}
	return relPath, rendered, nil
}

// FormatSpecMarkdown formats SpecReport as concise Markdown.
func FormatSpecMarkdown(rep SpecReport) string {
	var b strings.Builder
	b.WriteString("# Feature Spec Check\n\n")
	fmt.Fprintf(&b, "- **Repository:** `%s`\n", filepath.Base(rep.Root))
	if rep.SpecPath != "" {
		fmt.Fprintf(&b, "- **Spec:** `%s`\n", rep.SpecPath)
	}
	fmt.Fprintf(&b, "- **Status:** `%s`\n", rep.Status)
	fmt.Fprintf(&b, "- **Functional Requirements (`FR-`):** %d\n", len(rep.FunctionalRequirements))
	fmt.Fprintf(&b, "- **Success Criteria (`SC-`):** %d\n", len(rep.SuccessCriteria))
	fmt.Fprintf(&b, "- **Acceptance Scenarios (`Given/When/Then`):** %v\n", rep.HasAcceptanceScenarios)
	if len(rep.UnresolvedClarifications) > 0 {
		b.WriteString("\n## Unresolved Clarifications\n\n")
		for _, c := range rep.UnresolvedClarifications {
			fmt.Fprintf(&b, "- %s\n", c)
		}
	}
	if len(rep.MissingSections) > 0 {
		b.WriteString("\n## Missing Required Elements\n\n")
		for _, s := range rep.MissingSections {
			fmt.Fprintf(&b, "- %s\n", s)
		}
	}
	return b.String()
}

func analyzeSpecMarkdown(root, specRel, md string) SpecReport {
	norm := strings.ReplaceAll(md, "\r\n", "\n")
	lower := strings.ToLower(norm)
	rep := SpecReport{
		Root:                   root,
		SpecPath:               specRel,
		FunctionalRequirements: uniqueMatchesInOrder(frIDRe, norm),
		SuccessCriteria:        uniqueMatchesInOrder(scIDRe, norm),
		HasUserScenarios:       strings.Contains(lower, "user story") || strings.Contains(lower, "user scenarios"),
		HasAcceptanceScenarios: givenRe.MatchString(norm) && whenRe.MatchString(norm) && thenRe.MatchString(norm),
	}
	for _, m := range clarificationRe.FindAllStringSubmatch(norm, -1) {
		msg := strings.TrimSpace(m[1])
		if msg == "" {
			msg = strings.TrimSpace(m[0])
		}
		rep.UnresolvedClarifications = append(rep.UnresolvedClarifications, msg)
	}
	if !rep.HasUserScenarios {
		rep.MissingSections = append(rep.MissingSections, "user scenarios (P1/P2 user stories)")
	}
	if !rep.HasAcceptanceScenarios {
		rep.MissingSections = append(rep.MissingSections, "acceptance scenarios (Given/When/Then)")
	}
	if len(rep.FunctionalRequirements) == 0 {
		rep.MissingSections = append(rep.MissingSections, "functional requirements (FR-xxx)")
	}
	if len(rep.SuccessCriteria) == 0 {
		rep.MissingSections = append(rep.MissingSections, "measurable success criteria (SC-xxx)")
	}

	switch {
	case len(rep.UnresolvedClarifications) > 0:
		rep.Status = "needs_clarification"
	case len(rep.MissingSections) > 0:
		rep.Status = "incomplete"
	default:
		rep.Status = "valid"
	}
	return rep
}

func resolveSpecFile(root, specRel string) (string, string, error) {
	trimmed := strings.TrimSpace(specRel)
	if trimmed != "" {
		cleanRel := filepath.ToSlash(trimmed)
		if !paths.ValidateRelativePath(cleanRel) {
			return "", "", apperr.WithPath(apperr.Usage("invalid_spec_path", "spec path must be relative to workspace root"), cleanRel)
		}
		abs, err := paths.ResolveTarget(root, cleanRel)
		if err != nil {
			return "", "", err
		}
		info, statErr := os.Lstat(abs)
		if os.IsNotExist(statErr) {
			return "", "", apperr.WithPath(apperr.Usage("spec_not_found", fmt.Sprintf("spec file %s does not exist", cleanRel)), cleanRel)
		}
		if statErr != nil {
			return "", "", statErr
		}
		if paths.IsReparseOrSymlink(info) || !info.Mode().IsRegular() {
			return "", "", apperr.WithPath(apperr.Conflict("invalid_spec_file", fmt.Sprintf("spec path %s is not a regular file", cleanRel)), cleanRel)
		}
		return cleanRel, abs, nil
	}

	for _, dirRel := range candidateSpecsRe {
		absDir, err := paths.ResolveTarget(root, dirRel)
		if err != nil {
			continue
		}
		info, err := os.Lstat(absDir)
		if err != nil || paths.IsReparseOrSymlink(info) || !info.IsDir() {
			continue
		}
		entries, err := os.ReadDir(absDir)
		if err != nil {
			continue
		}
		var matches []string
		for _, ent := range entries {
			if ent.IsDir() {
				specNested := filepath.ToSlash(filepath.Join(dirRel, ent.Name(), "spec.md"))
				if isRegularWithin(root, specNested) {
					matches = append(matches, specNested)
				}
				continue
			}
			if strings.HasSuffix(strings.ToLower(ent.Name()), ".md") {
				cand := filepath.ToSlash(filepath.Join(dirRel, ent.Name()))
				if isRegularWithin(root, cand) {
					matches = append(matches, cand)
				}
			}
		}
		sort.Strings(matches)
		if len(matches) > 0 {
			abs, err := paths.ResolveTarget(root, matches[0])
			if err == nil {
				return matches[0], abs, nil
			}
		}
	}
	return "", "", nil
}

func uniqueMatchesInOrder(re *regexp.Regexp, s string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range re.FindAllString(s, -1) {
		if !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	return out
}

func titleFromSlug(slug string) string {
	parts := strings.Split(slug, "-")
	for i, p := range parts {
		if len(p) == 0 {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}
