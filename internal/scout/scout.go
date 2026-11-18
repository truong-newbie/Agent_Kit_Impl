// Package scout performs bounded, read-only polyglot repository reconnaissance,
// secret-redacted file slicing, and docs/codebase-summary.md generation.
package scout

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/project"
)

const (
	DefaultMaxFiles      = 50
	MaxAllowedFiles      = 200
	DefaultMaxBytes      = 32 * 1024
	MaxAllowedBytes      = 256 * 1024
	maxScanFileSizeBytes = 512 * 1024
)

var (
	skipDirNames = map[string]bool{
		".git": true, ".dtkit": true, "node_modules": true, "dist": true, "build": true,
		"target": true, "vendor": true, ".venv": true, "venv": true, "__pycache__": true,
		".next": true, "bin": true, "obj": true, ".idea": true, ".vscode": true,
	}
	secretRedactPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(api[_-]?key|token|secret|password|passwd|authorization|bearer)\s*[:=]\s*("[^"\r\n]*"|'[^'\r\n]*'|[^\s\r\n,;]+)`),
		regexp.MustCompile(`sk-[A-Za-z0-9_\-]{12,}`),
		regexp.MustCompile(`gh[pousr]_[A-Za-z0-9_]{16,}`),
		regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	}
)

// Options configures repository reconnaissance and optional context packing.
type Options struct {
	Root           string
	Query          string
	Paths          []string
	MaxFiles       int
	MaxBytes       int
	IncludeContent bool
}

// ModuleZone summarizes one logical sub-workspace or top-level module in the repository.
type ModuleZone struct {
	RelPath   string   `json:"relPath"`
	Languages []string `json:"languages"`
	Manifests []string `json:"manifests"`
	FileCount int      `json:"fileCount"`
}

// FileEntry captures metadata and an optional secret-redacted snippet for a matched file.
type FileEntry struct {
	RelPath   string `json:"relPath"`
	Language  string `json:"language"`
	SizeBytes int64  `json:"sizeBytes"`
	LineCount int    `json:"lineCount"`
	IsTest    bool   `json:"isTest"`
	Role      string `json:"role,omitempty"` // "entrypoint" | "contract" | "config" | "test" | "source"
	Snippet   string `json:"snippet,omitempty"`
	score     int
}

// Report is the structured result of dtkit scout.
type Report struct {
	Root          string         `json:"root"`
	TotalFiles    int            `json:"totalFiles"`
	TotalLines    int            `json:"totalLines"`
	LanguageStats map[string]int `json:"languageStats"`
	Zones         []ModuleZone   `json:"zones"`
	Contracts     []string       `json:"contracts,omitempty"`
	ExistingDocs  []string       `json:"existingDocs,omitempty"`
	MatchedFiles  []FileEntry    `json:"matchedFiles"`
	Truncated     bool           `json:"truncated,omitempty"`
}

// RedactText masks credential assignments and common key/token prefixes.
func RedactText(in string) string {
	out := in
	for _, re := range secretRedactPatterns {
		out = re.ReplaceAllStringFunc(out, func(m string) string {
			if idx := strings.IndexAny(m, ":="); idx > 0 {
				return m[:idx+1] + " [REDACTED]"
			}
			return "[REDACTED]"
		})
	}
	return out
}

// Run inspects a project root strictly read-only and returns bounded file, language, and zone summaries.
func Run(ctx context.Context, env paths.Environment, opts Options) (Report, error) {
	if err := ctx.Err(); err != nil {
		return Report{}, apperr.Cancelled("scout cancelled")
	}
	projRes, err := paths.ResolveProject(ctx, env, opts.Root)
	if err != nil {
		return Report{}, err
	}
	root := projRes.Root

	maxFiles := opts.MaxFiles
	if maxFiles <= 0 {
		maxFiles = DefaultMaxFiles
	}
	if maxFiles > MaxAllowedFiles {
		maxFiles = MaxAllowedFiles
	}
	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	if maxBytes > MaxAllowedBytes {
		maxBytes = MaxAllowedBytes
	}

	zoneFilters := normalizeZones(opts.Paths)
	queryLower := strings.ToLower(strings.TrimSpace(opts.Query))
	queryTerms := strings.Fields(queryLower)

	insp, _ := project.Inspect(ctx, env, root)
	zoneMap := map[string]*ModuleZone{}
	zoneLangSets := map[string]map[string]bool{}
	zoneManifestSets := map[string]map[string]bool{}
	ensureZone := func(relDir string) {
		if _, ok := zoneMap[relDir]; ok {
			return
		}
		zoneMap[relDir] = &ModuleZone{RelPath: relDir}
		zoneLangSets[relDir] = map[string]bool{}
		zoneManifestSets[relDir] = map[string]bool{}
	}
	for _, wr := range insp.Profile.Roots {
		ensureZone(wr.Path)
		if wr.Stack != "" {
			zoneLangSets[wr.Path][wr.Stack] = true
		}
		for _, ev := range wr.Evidence {
			zoneManifestSets[wr.Path][ev] = true
		}
	}

	rep := Report{
		Root:          root,
		LanguageStats: map[string]int{},
		MatchedFiles:  []FileEntry{},
	}

	var candidates []FileEntry
	err = filepath.WalkDir(root, func(fullPath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if ctx.Err() != nil {
			return apperr.Cancelled("scout cancelled")
		}
		relRaw, relErr := filepath.Rel(root, fullPath)
		if relErr != nil {
			return nil
		}
		rel := filepath.ToSlash(relRaw)
		if rel == "." {
			return nil
		}
		info, lErr := os.Lstat(fullPath)
		if lErr != nil || paths.IsReparseOrSymlink(info) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if skipDirNames[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.Mode().IsRegular() || isSensitiveFile(rel) {
			return nil
		}

		lang := detectLanguage(rel)
		role := classifyRole(rel)
		isTest := isTestRelPath(rel)
		lineCount := 0
		var content []byte
		if info.Size() <= maxScanFileSizeBytes {
			raw, rErr := os.ReadFile(fullPath)
			if rErr == nil && !bytes.Contains(raw, []byte{0}) {
				lineCount = countLines(raw)
				content = raw
			}
		}

		rep.TotalFiles++
		rep.TotalLines += lineCount
		if lang != "" {
			rep.LanguageStats[lang]++
		}
		if role == "contract" {
			rep.Contracts = append(rep.Contracts, rel)
		}
		if isKnownDoc(rel) {
			rep.ExistingDocs = append(rep.ExistingDocs, rel)
		}

		zDir := matchZone(rel, zoneMap)
		ensureZone(zDir)
		zoneMap[zDir].FileCount++
		if lang != "" && lang != "text" && lang != "markdown" {
			zoneLangSets[zDir][lang] = true
		}
		if role == "config" && isManifestFile(path.Base(rel)) {
			zoneManifestSets[zDir][rel] = true
		}

		if len(zoneFilters) > 0 && !matchesAnyZone(rel, zoneFilters) {
			return nil
		}

		score := scoreFile(rel, role, isTest, queryTerms, content)
		if len(queryTerms) > 0 && score == 0 {
			return nil
		}

		fe := FileEntry{
			RelPath:   rel,
			Language:  lang,
			SizeBytes: info.Size(),
			LineCount: lineCount,
			IsTest:    isTest,
			Role:      role,
			score:     score,
		}
		if opts.IncludeContent && len(content) > 0 {
			fe.Snippet = RedactText(string(content))
		}
		candidates = append(candidates, fe)
		return nil
	})
	if err != nil {
		return Report{}, err
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].RelPath < candidates[j].RelPath
	})

	usedBytes := 0
	for i, c := range candidates {
		if i >= maxFiles {
			rep.Truncated = true
			break
		}
		if opts.IncludeContent && c.Snippet != "" {
			rem := maxBytes - usedBytes
			if rem <= 0 {
				c.Snippet = ""
				rep.Truncated = true
			} else if len(c.Snippet) > rem {
				c.Snippet = c.Snippet[:rem] + "\n... [truncated]"
				usedBytes = maxBytes
				rep.Truncated = true
			} else {
				usedBytes += len(c.Snippet)
			}
		} else {
			c.Snippet = ""
		}
		rep.MatchedFiles = append(rep.MatchedFiles, c)
	}

	sort.Strings(rep.Contracts)
	sort.Strings(rep.ExistingDocs)

	zoneKeys := make([]string, 0, len(zoneMap))
	for k := range zoneMap {
		zoneKeys = append(zoneKeys, k)
	}
	sort.Strings(zoneKeys)
	for _, k := range zoneKeys {
		z := zoneMap[k]
		for l := range zoneLangSets[k] {
			z.Languages = append(z.Languages, l)
		}
		sort.Strings(z.Languages)
		for m := range zoneManifestSets[k] {
			z.Manifests = append(z.Manifests, m)
		}
		sort.Strings(z.Manifests)
		rep.Zones = append(rep.Zones, *z)
	}

	return rep, nil
}

// FormatMarkdownSummary builds a concise docs/codebase-summary.md representation from a Report.
func FormatMarkdownSummary(rep Report) string {
	var b strings.Builder
	b.WriteString("# Codebase Summary\n\n")
	fmt.Fprintf(&b, "- **Repository:** `%s`\n", filepath.Base(rep.Root))
	fmt.Fprintf(&b, "- **Scanned Files:** %d (%d lines)\n\n", rep.TotalFiles, rep.TotalLines)

	b.WriteString("## Language Breakdown\n\n")
	if len(rep.LanguageStats) == 0 {
		b.WriteString("No source files detected.\n\n")
	} else {
		langs := make([]string, 0, len(rep.LanguageStats))
		for l := range rep.LanguageStats {
			langs = append(langs, l)
		}
		sort.Strings(langs)
		b.WriteString("| Language | Files |\n|---|---|\n")
		for _, l := range langs {
			fmt.Fprintf(&b, "| %s | %d |\n", l, rep.LanguageStats[l])
		}
		b.WriteString("\n")
	}

	b.WriteString("## Workspace Zones\n\n")
	b.WriteString("| Zone | Languages | Manifests | Files |\n|---|---|---|---|\n")
	for _, z := range rep.Zones {
		langs := strings.Join(z.Languages, ", ")
		if langs == "" {
			langs = "-"
		}
		manifests := strings.Join(z.Manifests, ", ")
		if manifests == "" {
			manifests = "-"
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s | %d |\n", z.RelPath, langs, manifests, z.FileCount)
	}
	b.WriteString("\n")

	if len(rep.Contracts) > 0 {
		b.WriteString("## Cross-Service Contracts\n\n")
		for _, c := range rep.Contracts {
			fmt.Fprintf(&b, "- `%s`\n", c)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Key Matched Files\n\n")
	for _, f := range rep.MatchedFiles {
		fmt.Fprintf(&b, "- `%s` (%s, %d lines, role=%s)\n", f.RelPath, f.Language, f.LineCount, f.Role)
	}
	return b.String()
}

// WriteCodebaseSummary safely writes docs/codebase-summary.md under root.
func WriteCodebaseSummary(ctx context.Context, env paths.Environment, root, content string) (string, error) {
	projRes, err := paths.ResolveProject(ctx, env, root)
	if err != nil {
		return "", err
	}
	target, err := paths.ResolveTarget(projRes.Root, "docs/codebase-summary.md")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return "", err
	}
	if info, statErr := os.Lstat(target); statErr == nil && paths.IsReparseOrSymlink(info) {
		return "", apperr.WithPath(apperr.Conflict("symlink_target", "refusing symlinked docs/codebase-summary.md"), "docs/codebase-summary.md")
	}
	if err := os.WriteFile(target, []byte(content), 0644); err != nil {
		return "", err
	}
	return target, nil
}

func normalizeZones(raw []string) []string {
	var out []string
	for _, z := range raw {
		clean := strings.Trim(path.Clean(strings.ReplaceAll(strings.TrimSpace(z), `\`, "/")), "/")
		if clean == "" || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
			continue
		}
		out = append(out, clean)
	}
	return out
}

func matchesAnyZone(rel string, zones []string) bool {
	for _, z := range zones {
		if rel == z || strings.HasPrefix(rel, z+"/") {
			return true
		}
	}
	return false
}

func matchZone(rel string, zones map[string]*ModuleZone) string {
	best := "."
	bestLen := 0
	for k := range zones {
		if k == "." {
			continue
		}
		if (rel == k || strings.HasPrefix(rel, k+"/")) && len(k) > bestLen {
			best = k
			bestLen = len(k)
		}
	}
	if best == "." && strings.Contains(rel, "/") {
		top := strings.SplitN(rel, "/", 2)[0]
		return top
	}
	return best
}

func isSensitiveFile(rel string) bool {
	base := strings.ToLower(path.Base(rel))
	if base == ".env" || (strings.HasPrefix(base, ".env.") && !strings.HasSuffix(base, ".example") && !strings.HasSuffix(base, ".sample")) {
		return true
	}
	for _, suf := range []string{".pem", ".key", ".p12", ".pfx", ".jks", ".keystore"} {
		if strings.HasSuffix(base, suf) {
			return true
		}
	}
	return base == "id_rsa" || base == "id_ed25519"
}

func detectLanguage(rel string) string {
	lower := strings.ToLower(rel)
	switch {
	case strings.HasSuffix(lower, ".go"):
		return "go"
	case strings.HasSuffix(lower, ".ts"), strings.HasSuffix(lower, ".tsx"):
		return "typescript"
	case strings.HasSuffix(lower, ".js"), strings.HasSuffix(lower, ".jsx"), strings.HasSuffix(lower, ".mjs"):
		return "javascript"
	case strings.HasSuffix(lower, ".py"):
		return "python"
	case strings.HasSuffix(lower, ".java"), strings.HasSuffix(lower, ".kt"), strings.HasSuffix(lower, ".kts"):
		return "java"
	case strings.HasSuffix(lower, ".c"), strings.HasSuffix(lower, ".cpp"), strings.HasSuffix(lower, ".cc"), strings.HasSuffix(lower, ".cxx"), strings.HasSuffix(lower, ".h"), strings.HasSuffix(lower, ".hpp"):
		return "cpp"
	case strings.HasSuffix(lower, ".rs"):
		return "rust"
	case strings.HasSuffix(lower, ".cs"):
		return "csharp"
	case strings.HasSuffix(lower, ".sql"):
		return "sql"
	case strings.HasSuffix(lower, ".md"):
		return "markdown"
	case strings.HasSuffix(lower, ".json"), strings.HasSuffix(lower, ".yaml"), strings.HasSuffix(lower, ".yml"), strings.HasSuffix(lower, ".toml"), strings.HasSuffix(lower, ".xml"):
		return "config"
	default:
		return "text"
	}
}

func classifyRole(rel string) string {
	lower := strings.ToLower(rel)
	base := path.Base(lower)
	switch {
	case isContractFile(lower):
		return "contract"
	case isTestRelPath(rel):
		return "test"
	case isEntrypoint(lower, base):
		return "entrypoint"
	case isManifestFile(base) || strings.HasSuffix(base, ".yml") || strings.HasSuffix(base, ".yaml") || strings.HasSuffix(base, ".json"):
		return "config"
	default:
		return "source"
	}
}

func isContractFile(lower string) bool {
	base := path.Base(lower)
	return strings.HasPrefix(base, "openapi.") || strings.HasPrefix(base, "swagger.") ||
		strings.HasSuffix(base, ".proto") || strings.HasSuffix(base, ".graphql") ||
		strings.HasSuffix(base, ".gql") || base == "schema.sql" || base == "schema.prisma"
}

func isEntrypoint(lower, base string) bool {
	return base == "main.go" || base == "app.ts" || base == "index.ts" || base == "main.ts" ||
		base == "app.py" || base == "main.py" || base == "program.cs" || base == "lib.rs" ||
		base == "main.rs" || base == "main.cpp" || strings.HasSuffix(base, "application.java") ||
		strings.HasPrefix(lower, "cmd/")
}

func isManifestFile(base string) bool {
	switch strings.ToLower(base) {
	case "go.mod", "go.work", "package.json", "tsconfig.json", "pyproject.toml", "requirements.txt", "pom.xml", "build.gradle", "build.gradle.kts", "cmakelists.txt", "cargo.toml", "makefile", "docker-compose.yml", "docker-compose.yaml":
		return true
	}
	return strings.HasSuffix(strings.ToLower(base), ".sln") || strings.HasSuffix(strings.ToLower(base), ".csproj")
}

func isKnownDoc(rel string) bool {
	switch filepath.ToSlash(rel) {
	case "docs/codebase-summary.md", "docs/system-architecture.md", "docs/code-standards.md", "README.md":
		return true
	}
	return false
}

func isTestRelPath(rel string) bool {
	lower := strings.ToLower(rel)
	base := path.Base(lower)
	return strings.HasSuffix(base, "_test.go") ||
		strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") ||
		strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py") ||
		strings.HasSuffix(base, "test.java") || strings.HasSuffix(base, "tests.java") ||
		strings.HasSuffix(base, "_test.cpp") || strings.HasSuffix(base, "_test.cc") ||
		strings.HasSuffix(base, "tests.cs") || strings.Contains(lower, "/tests/") ||
		strings.Contains(lower, "/__tests__/") || strings.HasPrefix(lower, "tests/")
}

func scoreFile(rel, role string, isTest bool, queryTerms []string, content []byte) int {
	if len(queryTerms) > 0 {
		relLower := strings.ToLower(rel)
		score := 0
		for _, term := range queryTerms {
			if strings.Contains(relLower, term) {
				score += 10
			} else if len(content) > 0 && bytes.Contains(bytes.ToLower(content), []byte(term)) {
				score += 4
			}
		}
		if score == 0 {
			return 0
		}
		if role == "entrypoint" || role == "contract" {
			score += 2
		}
		return score
	}

	switch role {
	case "contract":
		return 30
	case "entrypoint":
		return 25
	case "config":
		return 20
	case "source":
		return 15
	case "test":
		return 10
	default:
		if isTest {
			return 10
		}
		return 5
	}
}

func countLines(b []byte) int {
	if len(b) == 0 {
		return 0
	}
	n := bytes.Count(b, []byte{'\n'})
	if b[len(b)-1] != '\n' {
		n++
	}
	return n
}
