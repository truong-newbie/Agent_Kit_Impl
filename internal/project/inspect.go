package project

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

const maxInspectManifestBytes = 1 << 20

var (
	goWorkUsePattern  = regexp.MustCompile(`(?m)^\s*([^\s#/]+)\s*$`)
	makeTargetPattern = regexp.MustCompile(`(?m)^([A-Za-z0-9][A-Za-z0-9._-]{0,63})\s*:`)
	nodeScriptAllowed = map[string]string{"test": "test", "lint": "lint", "typecheck": "typecheck", "check": "lint", "build": "build"}
	makeTargetAllowed = map[string]string{"test": "test", "lint": "lint", "typecheck": "typecheck", "check": "lint", "build": "build"}
	defaultGenerated  = []string{"node_modules", "dist", "build", ".venv", "__pycache__", "vendor", ".dtkit"}
	skipWalkDirs      = map[string]bool{".git": true, ".dtkit": true, "node_modules": true, "dist": true, "build": true, ".venv": true, "venv": true, "__pycache__": true, "vendor": true, ".next": true, "target": true}
)

// InspectResult holds read-only repository discovery results and candidate Profile.
type InspectResult struct {
	Root            string   `json:"root"`
	ConfigPath      string   `json:"configPath"`
	ProfileExists   bool     `json:"profileExists"`
	TrustDigest     string   `json:"trustDigest"`
	ExistingProfile *Profile `json:"existingProfile,omitempty"`
	Profile         Profile  `json:"profile"`
	Warnings        []string `json:"warnings,omitempty"`
}

// Inspect scans repo manifests strictly read-only (never executing scripts or tools) and builds a candidate Profile.
func Inspect(ctx context.Context, env paths.Environment, rawPath string) (InspectResult, error) {
	if err := ctx.Err(); err != nil {
		return InspectResult{}, apperr.Cancelled("project inspect cancelled")
	}
	res, err := paths.ResolveProject(ctx, env, rawPath)
	if err != nil {
		return InspectResult{}, err
	}
	root := res.Root
	cfgPath, err := paths.ResolveTarget(root, ProfileRelPath)
	if err != nil {
		return InspectResult{}, err
	}

	out := InspectResult{
		Root:       root,
		ConfigPath: cfgPath,
		Warnings:   append([]string{}, res.Warnings...),
	}
	if data, exists, loadErr := readOptionalRegular(root, ProfileRelPath); loadErr != nil {
		return InspectResult{}, loadErr
	} else if exists {
		out.ProfileExists = true
		existing, parseErr := ParseAndValidate(data)
		if parseErr != nil {
			out.Warnings = append(out.Warnings, fmt.Sprintf("existing %s failed validation: %v", ProfileRelPath, parseErr))
		} else {
			out.ExistingProfile = &existing
		}
	}

	profile := Profile{
		SchemaVersion: SchemaVersion1,
		Name:          filepath.Base(root),
		Roots:         []WorkspaceRoot{},
		Checks:        []CheckSpec{},
		GeneratedDirs: append([]string{}, defaultGenerated...),
		Evidence:      []EvidenceItem{},
	}

	candidates, err := discoverCandidateDirs(root)
	if err != nil {
		return InspectResult{}, err
	}

	rootMap := map[string]WorkspaceRoot{}
	checkMap := map[string]CheckSpec{}
	addRoot := func(relDir, stack, pm, ev string) {
		key := relDir + ":" + stack
		existing, ok := rootMap[key]
		if !ok {
			rootMap[key] = WorkspaceRoot{Path: relDir, Stack: stack, PackageManager: pm, Evidence: []string{ev}}
			return
		}
		if existing.PackageManager == "" && pm != "" {
			existing.PackageManager = pm
		}
		for _, seen := range existing.Evidence {
			if seen == ev {
				rootMap[key] = existing
				return
			}
		}
		existing.Evidence = append(existing.Evidence, ev)
		rootMap[key] = existing
	}
	addCheck := func(c CheckSpec) {
		if _, exists := checkMap[c.ID]; !exists {
			checkMap[c.ID] = c
		}
	}

	for _, relDir := range candidates {
		if err := ctx.Err(); err != nil {
			return InspectResult{}, apperr.Cancelled("project inspect cancelled")
		}
		prefix := ""
		if relDir != "." {
			prefix = relDir + "/"
		}

		// Go discovery
		if _, exists, err := readOptionalRegular(root, prefix+"go.mod"); err != nil {
			return InspectResult{}, err
		} else if exists {
			evPath := prefix + "go.mod"
			addRoot(relDir, "go", "go", evPath)
			profile.Evidence = append(profile.Evidence, EvidenceItem{Source: evPath, Summary: "Detected Go module"})
			addCheck(CheckSpec{
				ID:         makeCheckID(relDir, "go-vet"),
				Kind:       "lint",
				Executable: "go",
				Argv:       []string{"vet", "./..."},
				Cwd:        relDir,
				TimeoutSec: DefaultTimeoutSec,
			})
			addCheck(CheckSpec{
				ID:         makeCheckID(relDir, "go-test"),
				Kind:       "test",
				Executable: "go",
				Argv:       []string{"test", "./..."},
				Cwd:        relDir,
				TimeoutSec: DefaultTimeoutSec,
			})
		}

		// Node / TypeScript discovery
		if pkgBytes, exists, err := readOptionalRegular(root, prefix+"package.json"); err != nil {
			return InspectResult{}, err
		} else if exists {
			evPath := prefix + "package.json"
			var pkgJSON struct {
				Scripts map[string]string `json:"scripts"`
			}
			_ = json.Unmarshal(pkgBytes, &pkgJSON)
			stack := "node"
			if _, tsExists, tsErr := readOptionalRegular(root, prefix+"tsconfig.json"); tsErr == nil && tsExists {
				stack = "ts"
			}
			pm := detectNodePM(root, prefix)
			addRoot(relDir, stack, pm, evPath)
			profile.Evidence = append(profile.Evidence, EvidenceItem{Source: evPath, Summary: fmt.Sprintf("Detected %s workspace (%s)", stack, pm)})

			scriptNames := make([]string, 0, len(pkgJSON.Scripts))
			for k := range pkgJSON.Scripts {
				scriptNames = append(scriptNames, k)
			}
			sort.Strings(scriptNames)
			for _, script := range scriptNames {
				kind, ok := nodeScriptAllowed[script]
				if !ok {
					continue
				}
				argv := []string{"run", script}
				if pm == "yarn" {
					argv = []string{script}
				}
				addCheck(CheckSpec{
					ID:         makeCheckID(relDir, pm+"-"+script),
					Kind:       kind,
					Executable: pm,
					Argv:       argv,
					Cwd:        relDir,
					TimeoutSec: DefaultTimeoutSec,
				})
			}
		}

		// Python discovery
		pyEvidence := ""
		var pyText string
		for _, fname := range []string{"pyproject.toml", "setup.cfg", "requirements.txt"} {
			if raw, exists, err := readOptionalRegular(root, prefix+fname); err != nil {
				return InspectResult{}, err
			} else if exists {
				pyEvidence = prefix + fname
				pyText = string(raw)
				break
			}
		}
		if pyEvidence != "" {
			addRoot(relDir, "python", "python", pyEvidence)
			profile.Evidence = append(profile.Evidence, EvidenceItem{Source: pyEvidence, Summary: "Detected Python workspace"})
			lowerPy := strings.ToLower(pyText)
			if strings.Contains(lowerPy, "pytest") {
				addCheck(CheckSpec{
					ID:         makeCheckID(relDir, "pytest"),
					Kind:       "test",
					Executable: "pytest",
					Argv:       []string{},
					Cwd:        relDir,
					TimeoutSec: DefaultTimeoutSec,
				})
			} else if _, hasTests, _ := dirExistsRegular(root, prefix+"tests"); hasTests {
				addCheck(CheckSpec{
					ID:         makeCheckID(relDir, "python-unittest"),
					Kind:       "test",
					Executable: "python3",
					Argv:       []string{"-m", "unittest", "discover", "-s", "tests"},
					Cwd:        relDir,
					TimeoutSec: DefaultTimeoutSec,
				})
			}
			if strings.Contains(lowerPy, "ruff") {
				addCheck(CheckSpec{
					ID:         makeCheckID(relDir, "ruff"),
					Kind:       "lint",
					Executable: "ruff",
					Argv:       []string{"check", "."},
					Cwd:        relDir,
					TimeoutSec: 60,
				})
			}
			if strings.Contains(lowerPy, "mypy") {
				addCheck(CheckSpec{
					ID:         makeCheckID(relDir, "mypy"),
					Kind:       "typecheck",
					Executable: "mypy",
					Argv:       []string{"."},
					Cwd:        relDir,
					TimeoutSec: DefaultTimeoutSec,
				})
			}
		}
	}

	// Read-only Makefile & GitHub workflow hints at repo root
	if mkRaw, exists, err := readOptionalRegular(root, "Makefile"); err != nil {
		return InspectResult{}, err
	} else if exists {
		var targets []string
		for _, m := range makeTargetPattern.FindAllStringSubmatch(string(mkRaw), -1) {
			tName := m[1]
			if _, ok := makeTargetAllowed[tName]; ok {
				targets = append(targets, tName)
			}
		}
		if len(targets) > 0 {
			sort.Strings(targets)
			profile.Evidence = append(profile.Evidence, EvidenceItem{
				Source:  "Makefile",
				Summary: fmt.Sprintf("Read-only scan found targets: %s", strings.Join(targets, ", ")),
			})
		}
	}

	wfTarget, wfErr := paths.ResolveTarget(root, ".github/workflows")
	if wfErr == nil {
		if ents, err := os.ReadDir(wfTarget); err == nil {
			for _, ent := range ents {
				if !ent.IsDir() && (strings.HasSuffix(ent.Name(), ".yml") || strings.HasSuffix(ent.Name(), ".yaml")) {
					profile.Evidence = append(profile.Evidence, EvidenceItem{
						Source:  ".github/workflows/" + ent.Name(),
						Summary: "CI workflow present (read-only metadata)",
					})
				}
			}
		}
	}

	for _, r := range rootMap {
		sort.Strings(r.Evidence)
		profile.Roots = append(profile.Roots, r)
	}
	sort.Slice(profile.Roots, func(i, j int) bool {
		if profile.Roots[i].Path != profile.Roots[j].Path {
			return profile.Roots[i].Path < profile.Roots[j].Path
		}
		return profile.Roots[i].Stack < profile.Roots[j].Stack
	})
	for _, c := range checkMap {
		profile.Checks = append(profile.Checks, c)
	}
	sort.Slice(profile.Checks, func(i, j int) bool {
		return profile.Checks[i].ID < profile.Checks[j].ID
	})
	sort.Slice(profile.Evidence, func(i, j int) bool {
		if profile.Evidence[i].Source != profile.Evidence[j].Source {
			return profile.Evidence[i].Source < profile.Evidence[j].Source
		}
		return profile.Evidence[i].Summary < profile.Evidence[j].Summary
	})

	if err := ValidateProfile(profile); err != nil {
		return InspectResult{}, err
	}
	out.Profile = profile
	out.TrustDigest = CommandsTrustDigest(profile)
	return out, nil
}

func discoverCandidateDirs(root string) ([]string, error) {
	seen := map[string]bool{".": true}

	// Expand go.work directories
	if goWorkBytes, ok, err := readOptionalRegular(root, "go.work"); err != nil {
		return nil, err
	} else if ok {
		inUseBlock := false
		for _, rawLine := range strings.Split(string(goWorkBytes), "\n") {
			line := strings.TrimSpace(rawLine)
			if strings.HasPrefix(line, "//") || line == "" {
				continue
			}
			if strings.HasPrefix(line, "use (") {
				inUseBlock = true
				continue
			}
			if inUseBlock && line == ")" {
				inUseBlock = false
				continue
			}
			var token string
			if strings.HasPrefix(line, "use ") && !strings.Contains(line, "(") {
				token = strings.TrimSpace(strings.TrimPrefix(line, "use "))
			} else if inUseBlock {
				if m := goWorkUsePattern.FindStringSubmatch(line); len(m) == 2 {
					token = m[1]
				}
			}
			token = strings.Trim(token, `"`)
			if token != "" && token != "." {
				norm, vErr := validateWorkspaceRel(token, true)
				if vErr == nil {
					seen[norm] = true
				}
			}
		}
	}

	// Expand package.json workspaces patterns
	if pkgBytes, ok, err := readOptionalRegular(root, "package.json"); err != nil {
		return nil, err
	} else if ok {
		var ws struct {
			Workspaces any `json:"workspaces"`
		}
		if json.Unmarshal(pkgBytes, &ws) == nil && ws.Workspaces != nil {
			var patterns []string
			switch v := ws.Workspaces.(type) {
			case []any:
				for _, item := range v {
					if s, ok := item.(string); ok {
						patterns = append(patterns, s)
					}
				}
			case map[string]any:
				if pkgs, ok := v["packages"].([]any); ok {
					for _, item := range pkgs {
						if s, ok := item.(string); ok {
							patterns = append(patterns, s)
						}
					}
				}
			}
			for _, pat := range patterns {
				for _, match := range expandSafeWorkspaceGlob(root, pat) {
					seen[match] = true
				}
			}
		}
	}

	// Shallow bounded scan (depth <= 2) for multi-package repos without root workspace config.
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return nil
		}
		if rel == "." {
			return nil
		}
		slash := filepath.ToSlash(rel)
		if d.IsDir() {
			if skipWalkDirs[d.Name()] || strings.Count(slash, "/") >= 2 {
				return filepath.SkipDir
			}
			return nil
		}
		switch d.Name() {
		case "go.mod", "package.json", "pyproject.toml":
			dir := path.Dir(slash)
			if dir != "." {
				seen[dir] = true
			}
		}
		return nil
	})

	out := make([]string, 0, len(seen))
	for d := range seen {
		out = append(out, d)
	}
	sort.Strings(out)
	return out, nil
}

func expandSafeWorkspaceGlob(root, pat string) []string {
	clean := strings.TrimSpace(strings.TrimPrefix(strings.ReplaceAll(pat, `\`, "/"), "./"))
	if clean == "" || strings.HasPrefix(clean, "!") || strings.Contains(clean, "..") || strings.HasPrefix(clean, "/") {
		return nil
	}
	matches, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(clean)))
	if err != nil {
		return nil
	}
	var dirs []string
	for _, m := range matches {
		info, lErr := os.Lstat(m)
		if lErr != nil || !info.IsDir() || paths.IsReparseOrSymlink(info) {
			continue
		}
		rel, rErr := filepath.Rel(root, m)
		if rErr != nil {
			continue
		}
		norm, vErr := validateWorkspaceRel(filepath.ToSlash(rel), false)
		if vErr == nil {
			dirs = append(dirs, norm)
		}
	}
	return dirs
}

func detectNodePM(root, prefix string) string {
	for _, pair := range []struct {
		lock string
		pm   string
	}{
		{"pnpm-lock.yaml", "pnpm"},
		{"yarn.lock", "yarn"},
		{"bun.lockb", "bun"},
		{"bun.lock", "bun"},
		{"package-lock.json", "npm"},
	} {
		if _, ok, _ := readOptionalRegular(root, prefix+pair.lock); ok {
			return pair.pm
		}
		if prefix != "" {
			if _, ok, _ := readOptionalRegular(root, pair.lock); ok {
				return pair.pm
			}
		}
	}
	return "npm"
}

func makeCheckID(relDir, suffix string) string {
	if relDir == "." {
		return suffix
	}
	prefix := strings.NewReplacer("/", "-", "_", "-", " ", "-").Replace(strings.ToLower(relDir))
	id := prefix + "-" + suffix
	if len(id) > 64 {
		id = id[:64]
	}
	return strings.Trim(id, "-")
}

func readOptionalRegular(root, rel string) ([]byte, bool, error) {
	target, err := paths.ResolveTarget(root, rel)
	if err != nil {
		return nil, false, err
	}
	info, statErr := os.Lstat(target)
	if os.IsNotExist(statErr) {
		return nil, false, nil
	}
	if statErr != nil {
		return nil, false, statErr
	}
	if paths.IsReparseOrSymlink(info) {
		return nil, false, apperr.WithPath(apperr.Conflict("symlink_target", fmt.Sprintf("refusing symlinked manifest %s", rel)), rel)
	}
	if !info.Mode().IsRegular() {
		return nil, false, nil
	}
	if info.Size() > maxInspectManifestBytes {
		return nil, false, apperr.WithPath(apperr.Usage("file_too_large", fmt.Sprintf("%s exceeds max inspect size", rel)), rel)
	}
	b, err := os.ReadFile(target)
	return b, true, err
}

func dirExistsRegular(root, rel string) (string, bool, error) {
	target, err := paths.ResolveTarget(root, rel)
	if err != nil {
		return "", false, err
	}
	info, statErr := os.Lstat(target)
	if os.IsNotExist(statErr) {
		return "", false, nil
	}
	if statErr != nil {
		return "", false, statErr
	}
	if paths.IsReparseOrSymlink(info) || !info.IsDir() {
		return "", false, nil
	}
	return target, true, nil
}
