package dist

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing/fstest"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

// ImportOptions configures conversion of a local directory or HTTPS/GitHub repository into a DTKit package.
type ImportOptions struct {
	Context         context.Context
	SourceDir       string
	TargetParentDir string
	KitID           string
	Name            string
	Version         string
	DryRun          bool
	Update          bool
	FetchGitRepo    func(ctx context.Context, cloneURL, destDir string) error
}

// ImportedItem describes one discovered and converted component.
type ImportedItem struct {
	Kind        string `json:"kind"`
	ComponentID string `json:"componentId"`
	SourceRel   string `json:"sourceRel"`
	TargetRel   string `json:"targetRel"`
}

// ImportResult summarizes a community kit import operation.
type ImportResult struct {
	KitID          string          `json:"kitId"`
	Name           string          `json:"name"`
	Version        string          `json:"version"`
	SourceURL      string          `json:"sourceUrl,omitempty"`
	OutputDir      string          `json:"outputDir"`
	PackageDigest  string          `json:"packageDigest"`
	DetectedFormat string          `json:"detectedFormat"` // "skills-repo" | "cursor-rules" | "claude-config" | "mixed"
	Updated        bool            `json:"updated,omitempty"`
	Items          []ImportedItem  `json:"items"`
	Package        catalog.Package `json:"package"`
	Warnings       []string        `json:"warnings,omitempty"`
}

var githubShorthandRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*/[A-Za-z0-9][A-Za-z0-9._-]*$`)

var skipImportDirs = map[string]bool{
	".git": true, ".dtkit": true, "node_modules": true, "dist": true, "build": true,
	"target": true, "vendor": true, ".venv": true, "venv": true, "__pycache__": true,
	".next": true, "bin": true, "obj": true,
}

// ImportKit scans an external community directory (Superpowers, Claude skills/commands, Cursor .mdc rules, etc.)
// and converts it into a validated DTKit package directory.
func ImportKit(opts ImportOptions) (ImportResult, error) {
	var res ImportResult
	kitID := strings.TrimSpace(opts.KitID)
	if !catalog.IsValidID(kitID, 32) {
		return res, apperr.Usage("invalid_kit_id", fmt.Sprintf("kit ID %q must be <=32 lowercase ASCII letters/digits/hyphens", kitID))
	}
	version := strings.TrimSpace(opts.Version)
	if version == "" {
		version = "0.1.0"
	}
	if !catalog.IsValidVersion(version) {
		return res, apperr.Usage("invalid_version", fmt.Sprintf("version %q must be strict X.Y.Z semver", version))
	}
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = "Imported " + kitID + " Kit"
	}

	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}
	srcRoot, resolvedURL, cleanup, err := resolveImportSource(ctx, strings.TrimSpace(opts.SourceDir), opts.FetchGitRepo)
	if err != nil {
		return res, err
	}
	defer cleanup()

	srcInfo, err := os.Lstat(srcRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return res, apperr.WithPath(apperr.Usage("source_not_found", "import source directory does not exist"), srcRoot)
		}
		return res, err
	}
	if paths.IsReparseOrSymlink(srcInfo) {
		return res, apperr.WithPath(apperr.Usage("symlink_not_allowed", "import source directory cannot be a symlink or reparse point"), srcRoot)
	}
	if !srcInfo.IsDir() {
		return res, apperr.WithPath(apperr.Usage("invalid_source_dir", "import source must be a directory"), srcRoot)
	}

	absParent, err := filepath.Abs(strings.TrimSpace(opts.TargetParentDir))
	if err != nil {
		return res, err
	}
	outDir := filepath.Join(absParent, kitID)

	type rawCandidate struct {
		kind      string // "rule" | "skill" | "workflow" | "template"
		slug      string
		sourceRel string
		content   []byte
		formatTag string
	}
	var candidates []rawCandidate
	formatTags := map[string]bool{}

	err = filepath.WalkDir(srcRoot, func(fullPath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relRaw, rErr := filepath.Rel(srcRoot, fullPath)
		if rErr != nil {
			return rErr
		}
		rel := filepath.ToSlash(relRaw)
		if rel == "." {
			return nil
		}
		info, lErr := os.Lstat(fullPath)
		if lErr != nil {
			return lErr
		}
		if paths.IsReparseOrSymlink(info) {
			return apperr.WithPath(apperr.Usage("symlink_not_allowed", fmt.Sprintf("symlink not allowed in import source: %s", rel)), rel)
		}
		if d.IsDir() {
			if skipImportDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.Mode().IsRegular() || isSensitiveImportFile(rel) {
			return nil
		}

		kind, rawSlug, ftag := classifyImportPath(rel)
		if kind == "" {
			return nil
		}
		if info.Size() > catalog.MaxFileBytes {
			return apperr.WithPath(apperr.Usage("file_too_large", fmt.Sprintf("%s exceeds max file size", rel)), rel)
		}
		data, readErr := os.ReadFile(fullPath)
		if readErr != nil {
			return readErr
		}
		if bytes.Contains(data, []byte{0}) {
			return nil
		}
		slug := sanitizeSlug(rawSlug)
		if slug == "" {
			return nil
		}
		formatTags[ftag] = true
		candidates = append(candidates, rawCandidate{
			kind:      kind,
			slug:      slug,
			sourceRel: rel,
			content:   data,
			formatTag: ftag,
		})
		return nil
	})
	if err != nil {
		return res, err
	}

	if len(candidates) == 0 {
		return res, apperr.Usage("empty_import_source", "no recognizable skills, rules, commands, or workflows found in source directory")
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].kind != candidates[j].kind {
			return candidates[i].kind < candidates[j].kind
		}
		if candidates[i].slug != candidates[j].slug {
			return candidates[i].slug < candidates[j].slug
		}
		return candidates[i].sourceRel < candidates[j].sourceRel
	})

	memFS := fstest.MapFS{}
	seenComp := map[string]bool{}
	var rules []string
	var skills []rawCandidate
	var workflows []rawCandidate
	var templates []rawCandidate
	var items []ImportedItem

	for _, c := range candidates {
		compID := c.kind + "/" + c.slug
		if seenComp[compID] {
			continue
		}
		seenComp[compID] = true
		switch c.kind {
		case "rule":
			body := normalizeRuleContent(c.sourceRel, c.content)
			targetRel := "rules/" + c.slug + ".md"
			memFS[targetRel] = &fstest.MapFile{Data: body}
			rules = append(rules, compID)
			items = append(items, ImportedItem{Kind: "rule", ComponentID: compID, SourceRel: c.sourceRel, TargetRel: targetRel})
		case "skill":
			skills = append(skills, c)
		case "workflow":
			workflows = append(workflows, c)
		case "template":
			templates = append(templates, c)
		}
	}

	// Ensure at least one rule exists so skills have a clean engineering rule reference.
	if len(rules) == 0 {
		defaultRuleID := "rule/engineering"
		targetRel := "rules/engineering.md"
		memFS[targetRel] = &fstest.MapFile{
			Data: []byte(fmt.Sprintf("# %s Guidelines\n\n- Follow the imported workflows and skills in `%s`.\n- Respect project scope and verify changes with real checks.\n", name, kitID)),
		}
		rules = append(rules, defaultRuleID)
		items = append(items, ImportedItem{Kind: "rule", ComponentID: defaultRuleID, SourceRel: "(generated)", TargetRel: targetRel})
	}
	primaryRuleID := rules[0]

	var components []catalog.Component
	for _, rID := range rules {
		slug := strings.TrimPrefix(rID, "rule/")
		components = append(components, catalog.Component{
			ID:       rID,
			Kind:     "rule",
			Source:   "rules/" + slug + ".md",
			Requires: []string{},
		})
	}

	for _, t := range templates {
		compID := "template/" + t.slug
		targetRel := "templates/" + t.slug + ".md"
		memFS[targetRel] = &fstest.MapFile{Data: normalizeMarkdownBody(t.content)}
		components = append(components, catalog.Component{
			ID:       compID,
			Kind:     "template",
			Source:   targetRel,
			Requires: []string{},
		})
		items = append(items, ImportedItem{Kind: "template", ComponentID: compID, SourceRel: t.sourceRel, TargetRel: targetRel})
	}

	var skillIDs []string
	for _, s := range skills {
		compID := "skill/" + s.slug
		skillDir := "skills/" + s.slug
		targetRel := skillDir + "/SKILL.md.tmpl"
		tmplBytes := normalizeSkillTemplate(s.slug, kitID, primaryRuleID, s.content)
		memFS[targetRel] = &fstest.MapFile{Data: tmplBytes}
		components = append(components, catalog.Component{
			ID:       compID,
			Kind:     "skill",
			Source:   skillDir,
			Requires: append([]string(nil), rules...),
		})
		skillIDs = append(skillIDs, compID)
		items = append(items, ImportedItem{Kind: "skill", ComponentID: compID, SourceRel: s.sourceRel, TargetRel: targetRel})
	}

	// If source only had rules/workflows (e.g. .cursor/rules/*.mdc), synthesize a bridge skill so all 5 runtimes can install it.
	if len(skillIDs) == 0 {
		compID := "skill/apply-rules"
		skillDir := "skills/apply-rules"
		targetRel := skillDir + "/SKILL.md.tmpl"
		synth := fmt.Sprintf("# Apply %s Rules\n\nApply the imported project guidelines for `%s`.\n", name, kitID)
		memFS[targetRel] = &fstest.MapFile{Data: normalizeSkillTemplate("apply-rules", kitID, primaryRuleID, []byte(synth))}
		components = append(components, catalog.Component{
			ID:       compID,
			Kind:     "skill",
			Source:   skillDir,
			Requires: append([]string(nil), rules...),
		})
		skillIDs = append(skillIDs, compID)
		items = append(items, ImportedItem{Kind: "skill", ComponentID: compID, SourceRel: "(generated)", TargetRel: targetRel})
	}

	for _, w := range workflows {
		compID := "workflow/" + w.slug
		targetRel := "workflows/" + w.slug + ".md"
		memFS[targetRel] = &fstest.MapFile{Data: normalizeMarkdownBody(w.content)}
		components = append(components, catalog.Component{
			ID:       compID,
			Kind:     "workflow",
			Source:   targetRel,
			Requires: []string{skillIDs[0]},
		})
		items = append(items, ImportedItem{Kind: "workflow", ComponentID: compID, SourceRel: w.sourceRel, TargetRel: targetRel})
	}

	var existingVer, existingDig string
	hasExistingImported := false
	if opts.Update && strings.TrimSpace(opts.Version) == "" {
		if exVer, exDig, ok := readExistingImportedMeta(outDir); ok {
			existingVer, existingDig, hasExistingImported = exVer, exDig, true
			version = exVer
		}
	}

	buildPkgAndDigest := func(ver string) (catalog.Package, string, error) {
		p := catalog.Package{
			SchemaVersion:        1,
			ID:                   kitID,
			Name:                 name,
			Version:              ver,
			Description:          fmt.Sprintf("Imported community kit %s (%d components).", kitID, len(components)),
			RuntimeTargets:       []string{"codex", "gemini", "opencode", "cursor", "claude"},
			Scopes:               []string{"project", "global"},
			RequiresCapabilities: []string{"skills"},
			OptionalCapabilities: []string{"instructions"},
			PackageDependencies:  []string{},
			Components:           components,
		}
		pb, mErr := json.MarshalIndent(p, "", "  ")
		if mErr != nil {
			return catalog.Package{}, "", mErr
		}
		pb = append(pb, '\n')
		memFS["package.json"] = &fstest.MapFile{Data: pb}
		if vErr := catalog.Validate(p, memFS); vErr != nil {
			return catalog.Package{}, "", vErr
		}
		for _, rt := range p.RuntimeTargets {
			if _, rErr := adapter.RenderForRuntime(rt, p, memFS, "project"); rErr != nil {
				return catalog.Package{}, "", rErr
			}
		}
		dig, dErr := catalog.Digest(memFS)
		if dErr != nil {
			return catalog.Package{}, "", dErr
		}
		return p, dig, nil
	}

	pkg, digest, err := buildPkgAndDigest(version)
	if err != nil {
		return res, err
	}
	if hasExistingImported && digest != existingDig {
		bumped := bumpPatchVersion(existingVer)
		if bumped != "" && bumped != version {
			version = bumped
			pkg, digest, err = buildPkgAndDigest(version)
			if err != nil {
				return res, err
			}
		}
	}

	detectedFormat := "mixed"
	if len(formatTags) == 1 {
		for k := range formatTags {
			detectedFormat = k
		}
	}

	res = ImportResult{
		KitID:          kitID,
		Name:           name,
		Version:        version,
		SourceURL:      resolvedURL,
		OutputDir:      outDir,
		PackageDigest:  digest,
		DetectedFormat: detectedFormat,
		Items:          items,
		Package:        pkg,
	}

	if opts.DryRun {
		return res, nil
	}

	if info, lErr := os.Lstat(absParent); lErr == nil {
		if paths.IsReparseOrSymlink(info) {
			return res, apperr.WithPath(apperr.Usage("symlink_not_allowed", "output parent directory cannot be a symlink"), absParent)
		}
	}
	_, outStatErr := os.Lstat(outDir)
	alreadyExists := outStatErr == nil
	if alreadyExists && !opts.Update {
		return res, apperr.WithPath(apperr.Conflict("target_exists", fmt.Sprintf("import target directory %s already exists", outDir)), outDir)
	}
	if outStatErr != nil && !os.IsNotExist(outStatErr) {
		return res, outStatErr
	}

	var keys []string
	for k := range memFS {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	stageDir := outDir
	if alreadyExists && opts.Update {
		stageDir = filepath.Join(absParent, "."+kitID+".dtkit-stage")
		_ = os.RemoveAll(stageDir)
	}
	for _, rel := range keys {
		full := filepath.Join(stageDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			_ = os.RemoveAll(stageDir)
			return res, err
		}
		if err := writeRegularFile(full, memFS[rel].Data, 0644); err != nil {
			_ = os.RemoveAll(stageDir)
			return res, err
		}
	}
	if alreadyExists && opts.Update {
		oldDir := filepath.Join(absParent, "."+kitID+".dtkit-old")
		_ = os.RemoveAll(oldDir)
		if err := os.Rename(outDir, oldDir); err != nil {
			_ = os.RemoveAll(stageDir)
			return res, err
		}
		if err := os.Rename(stageDir, outDir); err != nil {
			_ = os.Rename(oldDir, outDir)
			_ = os.RemoveAll(stageDir)
			return res, err
		}
		_ = os.RemoveAll(oldDir)
		res.Updated = true
	}

	return res, nil
}

func resolveImportSource(ctx context.Context, raw string, fetchFn func(ctx context.Context, cloneURL, destDir string) error) (srcDir string, remoteURL string, cleanup func(), err error) {
	noop := func() {}
	if raw == "" {
		return "", "", noop, apperr.Usage("missing_source", "import source cannot be empty")
	}

	var cloneURL string
	switch {
	case strings.Contains(raw, "://"):
		if vErr := ValidateRemoteURL(raw); vErr != nil {
			return "", "", noop, vErr
		}
		cloneURL = raw
	case githubShorthandRe.MatchString(raw) && !strings.Contains(raw, ".."):
		if _, statErr := os.Lstat(raw); os.IsNotExist(statErr) {
			cloneURL = "https://github.com/" + strings.TrimSuffix(raw, ".git") + ".git"
			if vErr := ValidateRemoteURL(cloneURL); vErr != nil {
				return "", "", noop, vErr
			}
		}
	}

	if cloneURL != "" {
		tmpParent, mErr := os.MkdirTemp("", "dtkit-import-*")
		if mErr != nil {
			return "", "", noop, mErr
		}
		cleanFn := func() { _ = os.RemoveAll(tmpParent) }
		dest := filepath.Join(tmpParent, "repo")
		if fetchFn == nil {
			fetchFn = defaultGitClone
		}
		if fErr := fetchFn(ctx, cloneURL, dest); fErr != nil {
			cleanFn()
			return "", "", noop, fErr
		}
		return dest, cloneURL, cleanFn, nil
	}

	abs, err := filepath.Abs(raw)
	if err != nil {
		return "", "", noop, err
	}
	return abs, "", noop, nil
}

func defaultGitClone(ctx context.Context, cloneURL, destDir string) error {
	if _, err := exec.LookPath("git"); err != nil {
		return apperr.WithHint(
			apperr.Runtime("missing_git", "git executable not found on PATH for remote repository import"),
			"install git or clone the repository locally and pass the directory path",
		)
	}
	cloneCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cloneCtx, "git", "clone", "--depth", "1", "--single-branch", "--", cloneURL, destDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return apperr.Wrap(apperr.ExitRuntime, "git_clone_failed", fmt.Sprintf("git clone failed: %v (%s)", err, strings.TrimSpace(string(out))), err)
	}
	return nil
}

func isSensitiveImportFile(rel string) bool {
	base := strings.ToLower(path.Base(rel))
	if base == ".env" || strings.HasPrefix(base, ".env.") {
		return true
	}
	for _, suf := range []string{".pem", ".key", ".p12", ".pfx", ".exe", ".dll", ".so", ".dylib", ".sh", ".ps1", ".bat", ".cmd"} {
		if strings.HasSuffix(base, suf) {
			return true
		}
	}
	return false
}

func classifyImportPath(rel string) (kind, slug, formatTag string) {
	norm := strings.TrimPrefix(filepath.ToSlash(rel), "./")
	lower := strings.ToLower(norm)
	base := path.Base(norm)
	ext := strings.ToLower(path.Ext(base))
	stem := strings.TrimSuffix(base, path.Ext(base))

	// 1. Skills: skills/<slug>/SKILL.md(.tmpl) or .<runtime>/skills/<slug>/SKILL.md
	if base == "SKILL.md" || base == "SKILL.md.tmpl" {
		dir := path.Dir(norm)
		parentSlug := path.Base(dir)
		switch {
		case strings.HasPrefix(lower, "skills/"):
			return "skill", parentSlug, "skills-repo"
		case strings.Contains(lower, "/skills/"):
			return "skill", parentSlug, "claude-config"
		}
	}

	// 2. Slash commands: .claude/commands/<slug>.md or commands/<slug>.md -> skill/<slug>
	if ext == ".md" && (strings.HasPrefix(lower, ".claude/commands/") || strings.HasPrefix(lower, "commands/")) {
		return "skill", stem, "claude-config"
	}

	// 3. Cursor rules: .cursor/rules/<slug>.mdc or .cursorrules
	if strings.HasPrefix(lower, ".cursor/rules/") && (ext == ".mdc" || ext == ".md") {
		return "rule", stem, "cursor-rules"
	}
	if lower == ".cursorrules" {
		return "rule", "cursor-rules", "cursor-rules"
	}

	// 4. Explicit rules, workflows, templates
	if ext == ".md" {
		switch {
		case strings.HasPrefix(lower, "rules/") || strings.HasPrefix(lower, ".claude/rules/"):
			return "rule", stem, "skills-repo"
		case strings.HasPrefix(lower, "workflows/") || strings.HasPrefix(lower, ".claude/workflows/"):
			return "workflow", stem, "claude-config"
		case strings.HasPrefix(lower, "templates/"):
			return "template", stem, "skills-repo"
		case lower == "claude.md" || lower == "agents.md" || lower == "gemini.md":
			return "rule", stem, "claude-config"
		}
	}
	return "", "", ""
}

func sanitizeSlug(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	var b strings.Builder
	lastHyphen := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastHyphen = false
		} else if !lastHyphen && b.Len() > 0 {
			b.WriteByte('-')
			lastHyphen = true
		}
	}
	res := strings.Trim(b.String(), "-")
	if res == "" {
		return ""
	}
	if res[0] >= '0' && res[0] <= '9' {
		res = "s-" + res
	}
	if len(res) > 32 {
		res = strings.Trim(res[:32], "-")
	}
	if !catalog.IsValidID(res, 32) {
		return ""
	}
	return res
}

func stripFrontmatter(raw []byte) (meta map[string]string, body string) {
	meta = map[string]string{}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return meta, strings.TrimSpace(text)
	}
	rest := text[4:]
	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		return meta, strings.TrimSpace(text)
	}
	fm := rest[:idx]
	for _, line := range strings.Split(fm, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		meta[strings.ToLower(strings.TrimSpace(k))] = strings.Trim(strings.TrimSpace(v), `"'`)
	}
	return meta, strings.TrimSpace(rest[idx+5:])
}

func normalizeRuleContent(sourceRel string, raw []byte) []byte {
	_, body := stripFrontmatter(raw)
	if body == "" {
		body = fmt.Sprintf("# Imported Rule (%s)\n\n- Follow repository conventions.\n", path.Base(sourceRel))
	}
	if !strings.HasPrefix(body, "#") {
		body = fmt.Sprintf("# Imported Rule (%s)\n\n%s", path.Base(sourceRel), body)
	}
	return []byte(body + "\n")
}

func normalizeMarkdownBody(raw []byte) []byte {
	_, body := stripFrontmatter(raw)
	if body == "" {
		body = "# Imported Resource"
	}
	return []byte(body + "\n")
}

func normalizeSkillTemplate(slug, kitID, primaryRuleID string, raw []byte) []byte {
	meta, body := stripFrontmatter(raw)
	desc := strings.TrimSpace(meta["description"])
	desc = strings.ReplaceAll(desc, "\n", " ")
	if desc == "" {
		desc = fmt.Sprintf("Imported %s skill from %s kit.", slug, kitID)
	}
	// Escape any existing Go template delimiters inside external raw markdown so text/template never fails.
	safeBody := strings.ReplaceAll(body, "{{", "{ {")
	safeBody = strings.ReplaceAll(safeBody, "}}", "} }")

	var out strings.Builder
	out.WriteString("---\n")
	out.WriteString("name: {{.SkillName}}\n")
	fmt.Fprintf(&out, "description: %s\n", desc)
	out.WriteString("---\n\n")
	if !strings.HasPrefix(safeBody, "#") {
		fmt.Fprintf(&out, "# %s\n\n", slug)
	}
	fmt.Fprintf(&out, "Follow the kit guidance in `{{index .ResourcePaths %q}}`.\n\n", primaryRuleID)
	if safeBody != "" {
		out.WriteString(safeBody)
		out.WriteString("\n\n")
	}

	requiredSections := []struct {
		header      string
		defaultBody string
	}{
		{"## When to use", fmt.Sprintf("- Invoke when applying the `%s` workflow or skill.", slug)},
		{"## When not to use", "- Do not use outside the user's requested task scope."},
		{"## Steps", "1. Inspect the relevant repository files and instructions above.\n2. Execute the workflow carefully within approved scope.\n3. Verify results with real repository checks."},
		{"## Constraints", "- Do not fabricate outputs or bypass failing checks."},
		{"## Examples", fmt.Sprintf("1. **Standard usage:** Apply `%s` to the target module and verify results.", slug)},
	}
	for _, sec := range requiredSections {
		if !strings.Contains(safeBody, sec.header) {
			out.WriteString(sec.header + "\n" + sec.defaultBody + "\n\n")
		}
	}
	return []byte(strings.TrimSpace(out.String()) + "\n")
}

func readExistingImportedMeta(outDir string) (string, string, bool) {
	pkg, _, dig, err := catalog.LoadDir(outDir)
	if err != nil || !catalog.IsValidVersion(pkg.Version) {
		return "", "", false
	}
	return pkg.Version, dig, true
}

func bumpPatchVersion(ver string) string {
	parts := strings.Split(strings.TrimSpace(ver), ".")
	if len(parts) != 3 {
		return ""
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%s.%s.%d", parts[0], parts[1], patch+1)
}
