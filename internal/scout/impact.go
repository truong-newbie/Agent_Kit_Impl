package scout

import (
	"bytes"
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

// HotspotEntry highlights a high fan-in file or module with many repository dependents.
type HotspotEntry struct {
	Path       string `json:"path"`
	Dependents int    `json:"dependents"`
}

// ImpactReport summarizes reverse-dependency blast radius and related tests for changed files.
type ImpactReport struct {
	Root                 string         `json:"root"`
	ChangedFiles         []string       `json:"changedFiles"`
	DirectDependents     []string       `json:"directDependents"`
	TransitiveDependents []string       `json:"transitiveDependents"`
	RelatedTests         []string       `json:"relatedTests"`
	Hotspots             []HotspotEntry `json:"hotspots,omitempty"`
	EdgeCount            int            `json:"edgeCount"`
}

var (
	tsImportRe   = regexp.MustCompile(`(?m)(?:import\s+[^'"\n]*?from\s*|export\s+[^'"\n]*?from\s*|import\s*\(\s*|require\s*\(\s*)['"]([^'"]+)['"]`)
	pyFromRe     = regexp.MustCompile(`(?m)^\s*from\s+(\.{0,3}[A-Za-z0-9_.]*)\s+import\s+`)
	pyImportRe   = regexp.MustCompile(`(?m)^\s*import\s+([A-Za-z0-9_.]+(?:\s*,\s*[A-Za-z0-9_.]+)*)`)
	javaPkgRe    = regexp.MustCompile(`(?m)^\s*package\s+([A-Za-z0-9_.]+)\s*;?`)
	javaImportRe = regexp.MustCompile(`(?m)^\s*import\s+(?:static\s+)?([A-Za-z0-9_.*]+)\s*;?`)
	cppIncludeRe = regexp.MustCompile(`(?m)^\s*#include\s+"([^"]+)"`)
	goModuleRe   = regexp.MustCompile(`(?m)^\s*module\s+([^\s#]+)`)
)

// AnalyzeImpact constructs an intra-repo import/include graph across Go, TS/JS, Python, Java/Kotlin, and C/C++
// and computes direct/transitive dependents and related test files for changedPaths.
func AnalyzeImpact(ctx context.Context, env paths.Environment, rootArg string, changedPaths []string) (ImpactReport, error) {
	if err := ctx.Err(); err != nil {
		return ImpactReport{}, apperr.Cancelled("impact analysis cancelled")
	}
	projRes, err := paths.ResolveProject(ctx, env, rootArg)
	if err != nil {
		return ImpactReport{}, err
	}
	root := projRes.Root

	allFiles := map[string][]byte{}
	goModules := map[string]string{} // dir -> module path
	goPkgFiles := map[string][]string{}
	javaFQCN := map[string]string{} // "com.example.Foo" -> relPath

	err = filepath.WalkDir(root, func(fullPath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if ctx.Err() != nil {
			return apperr.Cancelled("impact analysis cancelled")
		}
		relRaw, rErr := filepath.Rel(root, fullPath)
		if rErr != nil {
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
		if !info.Mode().IsRegular() || isSensitiveFile(rel) || info.Size() > maxScanFileSizeBytes {
			return nil
		}

		base := path.Base(rel)
		if base == "go.mod" {
			if raw, rErr := os.ReadFile(fullPath); rErr == nil {
				if m := goModuleRe.FindSubmatch(raw); len(m) == 2 {
					goModules[path.Dir(rel)] = strings.TrimSpace(string(m[1]))
				}
			}
			return nil
		}

		lang := detectLanguage(rel)
		switch lang {
		case "go", "typescript", "javascript", "python", "java", "cpp":
		default:
			return nil
		}
		raw, rErr := os.ReadFile(fullPath)
		if rErr != nil || bytes.Contains(raw, []byte{0}) {
			return nil
		}
		allFiles[rel] = raw
		if lang == "go" {
			dir := path.Dir(rel)
			goPkgFiles[dir] = append(goPkgFiles[dir], rel)
		} else if lang == "java" {
			if m := javaPkgRe.FindSubmatch(raw); len(m) == 2 {
				cls := strings.TrimSuffix(base, path.Ext(base))
				javaFQCN[string(m[1])+"."+cls] = rel
			}
		}
		return nil
	})
	if err != nil {
		return ImpactReport{}, err
	}

	// Build file-level forward & reverse adjacency lists.
	forward := map[string]map[string]bool{}
	reverse := map[string]map[string]bool{}
	addEdge := func(fromFile, toFile string) {
		if fromFile == toFile || toFile == "" {
			return
		}
		if forward[fromFile] == nil {
			forward[fromFile] = map[string]bool{}
		}
		if !forward[fromFile][toFile] {
			forward[fromFile][toFile] = true
			if reverse[toFile] == nil {
				reverse[toFile] = map[string]bool{}
			}
			reverse[toFile][fromFile] = true
		}
	}

	fset := token.NewFileSet()
	for rel, raw := range allFiles {
		lang := detectLanguage(rel)
		switch lang {
		case "go":
			f, pErr := parser.ParseFile(fset, rel, raw, parser.ImportsOnly)
			if pErr != nil {
				continue
			}
			for _, imp := range f.Imports {
				impPath, uErr := strconv.Unquote(imp.Path.Value)
				if uErr != nil {
					continue
				}
				if targetDir := resolveGoImportDir(rel, impPath, goModules); targetDir != "" {
					for _, targetFile := range goPkgFiles[targetDir] {
						if !isTestRelPath(targetFile) {
							addEdge(rel, targetFile)
						}
					}
				}
			}
		case "typescript", "javascript":
			for _, m := range tsImportRe.FindAllStringSubmatch(string(raw), -1) {
				if resolved := resolveTSImport(rel, m[1], allFiles); resolved != "" {
					addEdge(rel, resolved)
				}
			}
		case "python":
			for _, target := range resolvePythonImports(rel, string(raw), allFiles) {
				addEdge(rel, target)
			}
		case "java":
			for _, m := range javaImportRe.FindAllStringSubmatch(string(raw), -1) {
				imp := strings.TrimSpace(m[1])
				if target, ok := javaFQCN[imp]; ok {
					addEdge(rel, target)
				} else if strings.HasSuffix(imp, ".*") {
					pkgPrefix := strings.TrimSuffix(imp, "*")
					for fqcn, target := range javaFQCN {
						if strings.HasPrefix(fqcn, pkgPrefix) {
							addEdge(rel, target)
						}
					}
				}
			}
		case "cpp":
			for _, m := range cppIncludeRe.FindAllStringSubmatch(string(raw), -1) {
				if resolved := resolveCppInclude(rel, m[1], allFiles); resolved != "" {
					addEdge(rel, resolved)
				}
			}
		}
	}

	// Normalize requested changed paths relative to root.
	var normChanged []string
	changedSet := map[string]bool{}
	for _, rawP := range changedPaths {
		norm := normalizeRelInput(root, rawP)
		if norm != "" && !changedSet[norm] {
			changedSet[norm] = true
			normChanged = append(normChanged, norm)
		}
	}
	sort.Strings(normChanged)

	// Direct & transitive reverse-BFS (up to depth 3)
	directSet := map[string]bool{}
	transitiveSet := map[string]bool{}
	visited := map[string]bool{}
	type qNode struct {
		file  string
		depth int
	}
	var queue []qNode
	for _, c := range normChanged {
		visited[c] = true
		queue = append(queue, qNode{file: c, depth: 0})
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.depth >= 3 {
			continue
		}
		for dep := range reverse[cur.file] {
			if cur.depth == 0 && !changedSet[dep] {
				directSet[dep] = true
			} else if cur.depth > 0 && !changedSet[dep] && !directSet[dep] {
				transitiveSet[dep] = true
			}
			if !visited[dep] {
				visited[dep] = true
				queue = append(queue, qNode{file: dep, depth: cur.depth + 1})
			}
		}
	}

	// Related tests: any test file in touched/dependent dirs OR test files directly importing touched/dependent files.
	touchedDirs := map[string]bool{}
	for _, c := range normChanged {
		touchedDirs[path.Dir(c)] = true
	}
	for d := range directSet {
		if isTestRelPath(d) {
			continue
		}
		touchedDirs[path.Dir(d)] = true
	}

	testSet := map[string]bool{}
	for d := range directSet {
		if isTestRelPath(d) {
			testSet[d] = true
		}
	}
	for tFile := range transitiveSet {
		if isTestRelPath(tFile) {
			testSet[tFile] = true
		}
	}
	for rel := range allFiles {
		if !isTestRelPath(rel) {
			continue
		}
		if touchedDirs[path.Dir(rel)] {
			testSet[rel] = true
			continue
		}
		// Match Java/TS/Python conventional test mirrors e.g. AuthService.java -> AuthServiceTest.java
		for _, c := range normChanged {
			if matchesTestStem(c, rel) {
				testSet[rel] = true
				break
			}
		}
	}

	edgeCount := 0
	for _, tos := range forward {
		edgeCount += len(tos)
	}

	var hotspots []HotspotEntry
	for file, deps := range reverse {
		if len(deps) > 0 {
			hotspots = append(hotspots, HotspotEntry{Path: file, Dependents: len(deps)})
		}
	}
	sort.Slice(hotspots, func(i, j int) bool {
		if hotspots[i].Dependents != hotspots[j].Dependents {
			return hotspots[i].Dependents > hotspots[j].Dependents
		}
		return hotspots[i].Path < hotspots[j].Path
	})
	if len(hotspots) > 10 {
		hotspots = hotspots[:10]
	}

	rep := ImpactReport{
		Root:                 root,
		ChangedFiles:         normChanged,
		DirectDependents:     sortedKeys(directSet),
		TransitiveDependents: sortedKeys(transitiveSet),
		RelatedTests:         sortedKeys(testSet),
		Hotspots:             hotspots,
		EdgeCount:            edgeCount,
	}
	return rep, nil
}

// FormatImpactMarkdown formats ImpactReport as human-readable Markdown.
func FormatImpactMarkdown(rep ImpactReport) string {
	var b strings.Builder
	b.WriteString("# Impact Analysis\n\n")
	fmt.Fprintf(&b, "- **Repository:** `%s`\n", filepath.Base(rep.Root))
	fmt.Fprintf(&b, "- **Graph Edges Scanned:** %d\n", rep.EdgeCount)
	fmt.Fprintf(&b, "- **Changed Files:** %s\n\n", strings.Join(rep.ChangedFiles, ", "))

	writeList := func(title string, items []string) {
		fmt.Fprintf(&b, "## %s (%d)\n\n", title, len(items))
		if len(items) == 0 {
			b.WriteString("- None\n\n")
			return
		}
		for _, item := range items {
			fmt.Fprintf(&b, "- `%s`\n", item)
		}
		b.WriteString("\n")
	}
	writeList("Direct Dependents", rep.DirectDependents)
	writeList("Transitive Dependents", rep.TransitiveDependents)
	writeList("Related Tests", rep.RelatedTests)

	if len(rep.Hotspots) > 0 {
		b.WriteString("## Top Core Hotspots\n\n")
		for _, h := range rep.Hotspots {
			fmt.Fprintf(&b, "- `%s` (%d dependents)\n", h.Path, h.Dependents)
		}
	}
	return b.String()
}

func resolveGoImportDir(fromRel, impPath string, goModules map[string]string) string {
	bestDir := ""
	bestMod := ""
	for modDir, modName := range goModules {
		if impPath == modName || strings.HasPrefix(impPath, modName+"/") {
			if modDir == "." || fromRel == modDir || strings.HasPrefix(fromRel, modDir+"/") {
				if len(modName) >= len(bestMod) {
					bestDir = modDir
					bestMod = modName
				}
			}
		}
	}
	if bestMod == "" {
		return ""
	}
	sub := strings.TrimPrefix(impPath, bestMod)
	sub = strings.TrimPrefix(sub, "/")
	if bestDir == "." {
		if sub == "" {
			return "."
		}
		return sub
	}
	if sub == "" {
		return bestDir
	}
	return path.Join(bestDir, sub)
}

func resolveTSImport(fromRel, spec string, allFiles map[string][]byte) string {
	if !strings.HasPrefix(spec, ".") {
		return ""
	}
	base := path.Clean(path.Join(path.Dir(fromRel), spec))
	candidates := []string{
		base,
		base + ".ts", base + ".tsx", base + ".js", base + ".jsx",
		path.Join(base, "index.ts"), path.Join(base, "index.tsx"), path.Join(base, "index.js"),
	}
	for _, c := range candidates {
		if _, ok := allFiles[c]; ok {
			return c
		}
	}
	return ""
}

func resolvePythonImports(fromRel, text string, allFiles map[string][]byte) []string {
	var out []string
	dir := path.Dir(fromRel)
	zoneRoot := "."
	if strings.Contains(fromRel, "/") {
		zoneRoot = strings.SplitN(fromRel, "/", 2)[0]
	}

	resolveMod := func(mod string) {
		if strings.HasPrefix(mod, ".") {
			trimmed := strings.TrimLeft(mod, ".")
			dots := len(mod) - len(trimmed)
			baseDir := dir
			for i := 1; i < dots && baseDir != "."; i++ {
				baseDir = path.Dir(baseDir)
			}
			relPath := strings.ReplaceAll(trimmed, ".", "/")
			for _, cand := range []string{path.Join(baseDir, relPath+".py"), path.Join(baseDir, relPath, "__init__.py")} {
				cand = path.Clean(cand)
				if _, ok := allFiles[cand]; ok {
					out = append(out, cand)
				}
			}
			return
		}
		relPath := strings.ReplaceAll(mod, ".", "/")
		prefixes := []string{relPath}
		if zoneRoot != "." {
			prefixes = append(prefixes, path.Join(zoneRoot, relPath))
		}
		for _, p := range prefixes {
			for _, cand := range []string{p + ".py", path.Join(p, "__init__.py")} {
				if _, ok := allFiles[cand]; ok {
					out = append(out, cand)
				}
			}
		}
	}

	for _, m := range pyFromRe.FindAllStringSubmatch(text, -1) {
		resolveMod(m[1])
	}
	for _, m := range pyImportRe.FindAllStringSubmatch(text, -1) {
		for _, part := range strings.Split(m[1], ",") {
			name := strings.TrimSpace(strings.Split(part, " as ")[0])
			if name != "" {
				resolveMod(name)
			}
		}
	}
	return out
}

func resolveCppInclude(fromRel, inc string, allFiles map[string][]byte) string {
	clean := path.Clean(strings.ReplaceAll(inc, `\`, "/"))
	candidates := []string{
		path.Clean(path.Join(path.Dir(fromRel), clean)),
		clean,
		path.Clean(path.Join("include", clean)),
	}
	if strings.Contains(fromRel, "/") {
		zone := strings.SplitN(fromRel, "/", 2)[0]
		candidates = append(candidates,
			path.Clean(path.Join(zone, clean)),
			path.Clean(path.Join(zone, "include", clean)),
		)
	}
	for _, c := range candidates {
		if _, ok := allFiles[c]; ok {
			return c
		}
	}
	return ""
}

func normalizeRelInput(root, raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if filepath.IsAbs(s) {
		rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(s))
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return ""
		}
		return filepath.ToSlash(rel)
	}
	clean := path.Clean(strings.ReplaceAll(s, `\`, "/"))
	clean = strings.TrimPrefix(clean, "./")
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return ""
	}
	return clean
}

func matchesTestStem(srcRel, testRel string) bool {
	srcBase := strings.ToLower(strings.TrimSuffix(path.Base(srcRel), path.Ext(srcRel)))
	testBase := strings.ToLower(strings.TrimSuffix(path.Base(testRel), path.Ext(testRel)))
	if srcBase == "" {
		return false
	}
	return strings.Contains(testBase, srcBase)
}

func sortedKeys(m map[string]bool) []string {
	if len(m) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
