package scout

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/guard"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

// ConvergenceReport summarizes plan/checklist/file/receipt alignment for a repository.
type ConvergenceReport struct {
	Root            string   `json:"root"`
	PlanPath        string   `json:"planPath,omitempty"`
	Constitution    string   `json:"constitutionPath,omitempty"`
	Status          string   `json:"status"` // "converged" | "incomplete" | "unverified" | "no_plan"
	TotalItems      int      `json:"totalItems"`
	CompletedItems  int      `json:"completedItems"`
	UnmetItems      []string `json:"unmetItems,omitempty"`
	ReferencedFiles []string `json:"referencedFiles,omitempty"`
	ExistingFiles   []string `json:"existingFiles,omitempty"`
	MissingFiles    []string `json:"missingFiles,omitempty"`
	ReceiptVerified bool     `json:"receiptVerified"`
	ReceiptReason   string   `json:"receiptReason,omitempty"`
}

var (
	checklistOpenRe = regexp.MustCompile(`^\s*(?:[-*]|\d+\.)\s+\[\s\]\s+(.+?)\s*$`)
	checklistDoneRe = regexp.MustCompile(`^\s*(?:[-*]|\d+\.)\s+\[[xX]\]\s+(.+?)\s*$`)
	backtickSpanRe  = regexp.MustCompile("`([^`\n]{2,180})`")
	knownPathExts   = map[string]bool{
		".go": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".mjs": true,
		".py": true, ".java": true, ".kt": true, ".kts": true,
		".c": true, ".cc": true, ".cpp": true, ".h": true, ".hpp": true,
		".rs": true, ".cs": true, ".md": true, ".json": true, ".yml": true, ".yaml": true,
		".toml": true, ".sql": true, ".proto": true, ".graphql": true, ".sh": true, ".ps1": true,
	}
	candidatePlanPaths = []string{
		"implementation-plan.md",
		"docs/implementation-plan.md",
		"plan.md",
		"specs/plan.md",
	}
	candidateConstitutionPaths = []string{
		".dtkit/constitution.md",
		"docs/code-standards.md",
		".specify/memory/constitution.md",
	}
)

// AnalyzeConvergence compares a plan/spec markdown artifact against actual repository files
// and the cryptographic check receipt strictly read-only.
func AnalyzeConvergence(ctx context.Context, env paths.Environment, rawRoot, planRel string) (ConvergenceReport, error) {
	if err := ctx.Err(); err != nil {
		return ConvergenceReport{}, apperr.Cancelled("convergence analysis cancelled")
	}
	projRes, err := paths.ResolveProject(ctx, env, rawRoot)
	if err != nil {
		return ConvergenceReport{}, err
	}
	root := projRes.Root

	rep := ConvergenceReport{
		Root:   root,
		Status: "no_plan",
	}
	for _, cRel := range candidateConstitutionPaths {
		if ok := isRegularWithin(root, cRel); ok {
			rep.Constitution = cRel
			break
		}
	}

	resolvedPlanRel, planAbs, err := resolvePlanFile(root, planRel)
	if err != nil {
		return ConvergenceReport{}, err
	}
	vStat, vErr := guard.VerifyWorkspace(root)
	if vErr == nil {
		rep.ReceiptVerified = vStat.Verified
		rep.ReceiptReason = vStat.Reason
	}

	if resolvedPlanRel == "" {
		return rep, nil
	}
	rep.PlanPath = resolvedPlanRel

	rawPlan, err := os.ReadFile(planAbs)
	if err != nil {
		return ConvergenceReport{}, err
	}
	openItems, doneCount := parseChecklistLines(string(rawPlan))
	rep.UnmetItems = openItems
	rep.CompletedItems = doneCount
	rep.TotalItems = len(openItems) + doneCount

	refs := extractReferencedFiles(string(rawPlan))
	rep.ReferencedFiles = refs
	for _, rel := range refs {
		if isExistingWorkspaceEntry(root, rel) {
			rep.ExistingFiles = append(rep.ExistingFiles, rel)
		} else {
			rep.MissingFiles = append(rep.MissingFiles, rel)
		}
	}

	switch {
	case len(rep.UnmetItems) > 0 || len(rep.MissingFiles) > 0:
		rep.Status = "incomplete"
	case !rep.ReceiptVerified:
		rep.Status = "unverified"
	default:
		rep.Status = "converged"
	}
	return rep, nil
}

// AppendConvergenceTasks appends unmet checklist items and missing referenced files to tasksRel
// in append-only fashion, leaving the file untouched byte-for-byte when already converged.
func AppendConvergenceTasks(root, tasksRel string, rep ConvergenceReport) (bool, error) {
	if len(rep.UnmetItems) == 0 && len(rep.MissingFiles) == 0 {
		return false, nil
	}
	cleanRel := filepath.ToSlash(strings.TrimSpace(tasksRel))
	if cleanRel == "" {
		return false, apperr.Usage("missing_tasks_path", "--append-tasks requires a workspace-relative markdown path")
	}
	if !paths.ValidateRelativePath(cleanRel) {
		return false, apperr.WithPath(apperr.Usage("invalid_tasks_path", "tasks path must stay within repository root"), cleanRel)
	}
	target, err := paths.ResolveTarget(root, cleanRel)
	if err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return false, err
	}
	var existingBytes []byte
	if info, statErr := os.Lstat(target); statErr == nil {
		if paths.IsReparseOrSymlink(info) {
			return false, apperr.WithPath(apperr.Conflict("symlink_target", fmt.Sprintf("refusing symlinked %s", cleanRel)), cleanRel)
		}
		if !info.Mode().IsRegular() {
			return false, apperr.WithPath(apperr.Conflict("invalid_target", fmt.Sprintf("%s is not a regular file", cleanRel)), cleanRel)
		}
		existingBytes, err = os.ReadFile(target)
		if err != nil {
			return false, err
		}
	} else if !os.IsNotExist(statErr) {
		return false, statErr
	}

	existingText := string(existingBytes)
	var additions []string
	for _, rel := range rep.MissingFiles {
		line := fmt.Sprintf("- [ ] Create or implement missing file `%s` referenced in `%s`", rel, rep.PlanPath)
		if !strings.Contains(existingText, "`"+rel+"`") {
			additions = append(additions, line)
		}
	}
	for _, item := range rep.UnmetItems {
		line := fmt.Sprintf("- [ ] Complete unmet plan item: %s", item)
		if !strings.Contains(existingText, item) {
			additions = append(additions, line)
		}
	}
	if len(additions) == 0 {
		return false, nil
	}

	var b strings.Builder
	b.WriteString(existingText)
	if len(existingBytes) > 0 && !strings.HasSuffix(existingText, "\n") {
		b.WriteByte('\n')
	}
	if len(existingBytes) > 0 {
		b.WriteByte('\n')
	}
	b.WriteString("## Phase Convergence\n\n")
	for _, l := range additions {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(target, []byte(b.String()), 0644); err != nil {
		return false, err
	}
	return true, nil
}

// FormatConvergenceMarkdown formats ConvergenceReport as readable Markdown.
func FormatConvergenceMarkdown(rep ConvergenceReport) string {
	var b strings.Builder
	b.WriteString("# Convergence Report\n\n")
	fmt.Fprintf(&b, "- **Repository:** `%s`\n", filepath.Base(rep.Root))
	if rep.PlanPath != "" {
		fmt.Fprintf(&b, "- **Plan:** `%s`\n", rep.PlanPath)
	}
	if rep.Constitution != "" {
		fmt.Fprintf(&b, "- **Constitution:** `%s`\n", rep.Constitution)
	}
	fmt.Fprintf(&b, "- **Status:** `%s`\n", rep.Status)
	fmt.Fprintf(&b, "- **Checklist:** %d/%d completed\n", rep.CompletedItems, rep.TotalItems)
	fmt.Fprintf(&b, "- **Test Receipt Verified:** %v", rep.ReceiptVerified)
	if rep.ReceiptReason != "" {
		fmt.Fprintf(&b, " (%s)", rep.ReceiptReason)
	}
	b.WriteString("\n\n")

	if len(rep.MissingFiles) > 0 {
		b.WriteString("## Missing Referenced Files\n\n")
		for _, f := range rep.MissingFiles {
			fmt.Fprintf(&b, "- `%s`\n", f)
		}
		b.WriteString("\n")
	}
	if len(rep.UnmetItems) > 0 {
		b.WriteString("## Unmet Checklist Items\n\n")
		for _, item := range rep.UnmetItems {
			fmt.Fprintf(&b, "- [ ] %s\n", item)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func resolvePlanFile(root, planRel string) (string, string, error) {
	trimmed := strings.TrimSpace(planRel)
	if trimmed != "" {
		cleanRel := filepath.ToSlash(trimmed)
		if !paths.ValidateRelativePath(cleanRel) {
			return "", "", apperr.WithPath(apperr.Usage("invalid_plan_path", "plan path must be relative to workspace root"), cleanRel)
		}
		abs, err := paths.ResolveTarget(root, cleanRel)
		if err != nil {
			return "", "", err
		}
		info, statErr := os.Lstat(abs)
		if os.IsNotExist(statErr) {
			return "", "", apperr.WithPath(apperr.Usage("plan_not_found", fmt.Sprintf("plan file %s does not exist", cleanRel)), cleanRel)
		}
		if statErr != nil {
			return "", "", statErr
		}
		if paths.IsReparseOrSymlink(info) || !info.Mode().IsRegular() {
			return "", "", apperr.WithPath(apperr.Conflict("invalid_plan_file", fmt.Sprintf("plan path %s is not a regular file", cleanRel)), cleanRel)
		}
		return cleanRel, abs, nil
	}
	for _, cand := range candidatePlanPaths {
		if isRegularWithin(root, cand) {
			abs, err := paths.ResolveTarget(root, cand)
			if err == nil {
				return cand, abs, nil
			}
		}
	}
	return "", "", nil
}

func isRegularWithin(root, rel string) bool {
	abs, err := paths.ResolveTarget(root, rel)
	if err != nil {
		return false
	}
	info, err := os.Lstat(abs)
	return err == nil && !paths.IsReparseOrSymlink(info) && info.Mode().IsRegular()
}

func isExistingWorkspaceEntry(root, rel string) bool {
	abs, err := paths.ResolveTarget(root, rel)
	if err != nil {
		return false
	}
	info, err := os.Lstat(abs)
	return err == nil && !paths.IsReparseOrSymlink(info)
}

func parseChecklistLines(md string) ([]string, int) {
	var unmet []string
	done := 0
	for _, line := range strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n") {
		if m := checklistOpenRe.FindStringSubmatch(line); len(m) == 2 {
			unmet = append(unmet, strings.TrimSpace(m[1]))
		} else if checklistDoneRe.MatchString(line) {
			done++
		}
	}
	return unmet, done
}

func extractReferencedFiles(md string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range backtickSpanRe.FindAllStringSubmatch(md, -1) {
		cand := strings.TrimSpace(m[1])
		if strings.ContainsAny(cand, " \t*?<>|\"'()[]{}$") || strings.Contains(cand, "://") {
			continue
		}
		cand = strings.TrimPrefix(filepath.ToSlash(cand), "./")
		ext := strings.ToLower(filepath.Ext(cand))
		if !knownPathExts[ext] || !strings.Contains(cand, "/") {
			continue
		}
		if !paths.ValidateRelativePath(cand) {
			continue
		}
		if !seen[cand] {
			seen[cand] = true
			out = append(out, cand)
		}
	}
	sort.Strings(out)
	return out
}
