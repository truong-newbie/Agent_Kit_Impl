package check

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/project"
)

// ChangedSelection summarizes Git working-tree path detection and check selection.
type ChangedSelection struct {
	ChangedPaths   []string `json:"changedPaths,omitempty"`
	MatchedRoots   []string `json:"matchedRoots,omitempty"`
	FallbackAll    bool     `json:"fallbackAll,omitempty"`
	FallbackReason string   `json:"fallbackReason,omitempty"`
}

// DetectChangedSelection inspects Git status (staged, unstaged, untracked, renames, deletes)
// and maps changed paths cleanly to workspace root directories.
func DetectChangedSelection(ctx context.Context, repoRoot string, prof project.Profile) ChangedSelection {
	gitDir, err := os.Stat(filepath.Join(repoRoot, ".git"))
	if err != nil || (!gitDir.IsDir() && !gitDir.Mode().IsRegular()) {
		return ChangedSelection{
			FallbackAll:    true,
			FallbackReason: "no .git directory found; running all checks",
		}
	}

	cmd := exec.CommandContext(ctx, "git", "-C", repoRoot, "status", "--porcelain=v1", "-z", "-uall")
	out, err := cmd.Output()
	if err != nil {
		return ChangedSelection{
			FallbackAll:    true,
			FallbackReason: "git status unavailable; running all checks",
		}
	}

	pathsList := parsePorcelainZ(out)
	if len(pathsList) == 0 {
		return ChangedSelection{ChangedPaths: []string{}, MatchedRoots: []string{}}
	}

	rootFallbackTriggers := map[string]bool{
		".dtkit/project.json": true,
		"go.work":             true,
		"pnpm-workspace.yaml": true,
		"Makefile":            true,
	}
	for _, p := range pathsList {
		if rootFallbackTriggers[p] {
			return ChangedSelection{
				ChangedPaths:   pathsList,
				FallbackAll:    true,
				FallbackReason: "shared workspace config modified (" + p + "); running all checks",
			}
		}
	}

	// Collect known workspace roots sorted deepest-first so nested workspaces match before ".".
	var wsRoots []string
	seenWS := map[string]bool{}
	for _, r := range prof.Roots {
		norm := path.Clean(strings.ReplaceAll(r.Path, `\`, "/"))
		if !seenWS[norm] {
			seenWS[norm] = true
			wsRoots = append(wsRoots, norm)
		}
	}
	sort.Slice(wsRoots, func(i, j int) bool {
		if wsRoots[i] == "." {
			return false
		}
		if wsRoots[j] == "." {
			return true
		}
		if len(wsRoots[i]) != len(wsRoots[j]) {
			return len(wsRoots[i]) > len(wsRoots[j])
		}
		return wsRoots[i] < wsRoots[j]
	})

	matchedSet := map[string]bool{}
	for _, cp := range pathsList {
		matched := ""
		for _, ws := range wsRoots {
			if ws == "." {
				matched = "."
				break
			}
			if cp == ws || strings.HasPrefix(cp, ws+"/") {
				matched = ws
				break
			}
		}
		if matched == "" {
			matched = "."
		}
		matchedSet[matched] = true
	}

	matchedRoots := make([]string, 0, len(matchedSet))
	for r := range matchedSet {
		matchedRoots = append(matchedRoots, r)
	}
	sort.Strings(matchedRoots)
	return ChangedSelection{
		ChangedPaths: pathsList,
		MatchedRoots: matchedRoots,
	}
}

func parsePorcelainZ(data []byte) []string {
	entries := bytes.Split(data, []byte{0})
	seen := map[string]bool{}
	var pathsOut []string
	for i := 0; i < len(entries); i++ {
		rec := entries[i]
		if len(rec) < 4 {
			continue
		}
		statusX := rec[0]
		statusY := rec[1]
		p := string(rec[3:])
		if p != "" {
			norm := path.Clean(strings.ReplaceAll(p, `\`, "/"))
			if !seen[norm] {
				seen[norm] = true
				pathsOut = append(pathsOut, norm)
			}
		}
		// In -z porcelain v1, rename ('R') and copy ('C') emit two NUL-terminated paths (dest, then src).
		if statusX == 'R' || statusX == 'C' || statusY == 'R' || statusY == 'C' {
			if i+1 < len(entries) && len(entries[i+1]) > 0 {
				i++
				orig := path.Clean(strings.ReplaceAll(string(entries[i]), `\`, "/"))
				if !seen[orig] {
					seen[orig] = true
					pathsOut = append(pathsOut, orig)
				}
			}
		}
	}
	sort.Strings(pathsOut)
	return pathsOut
}
