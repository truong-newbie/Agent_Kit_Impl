package planner

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
)

const (
	// MaxFileDiffBytes bounds any single artifact unified diff patch.
	MaxFileDiffBytes = 16 * 1024
	// MaxTotalDiffBytes bounds total diff patch text emitted across a plan.
	MaxTotalDiffBytes = 64 * 1024
	maxDiffLines      = 2000
	diffContextLines  = 3
)

// ArtifactDiff captures a privacy-preserving, bounded preview diff for one planned artifact.
type ArtifactDiff struct {
	Root         string `json:"root"`
	Path         string `json:"path"`
	Strategy     string `json:"strategy"`
	ComponentID  string `json:"componentId,omitempty"`
	Owner        string `json:"owner,omitempty"`
	Status       string `json:"status"`
	Reason       string `json:"reason"`
	BeforeDigest string `json:"beforeDigest,omitempty"`
	AfterDigest  string `json:"afterDigest,omitempty"`
	Patch        string `json:"patch,omitempty"`
	Truncated    bool   `json:"truncated,omitempty"`
	Binary       bool   `json:"binary,omitempty"`
}

// PlanDiff summarizes bounded content diffs across planned actions.
type PlanDiff struct {
	Changes    []ArtifactDiff `json:"changes"`
	TotalBytes int            `json:"totalBytes"`
	Truncated  bool           `json:"truncated,omitempty"`
}

// ComputePlanDiff computes bounded content diffs for all non-noop (or all if includeNoop) actions
// without exposing unowned files or user-written bytes outside DTKit's managed block.
func ComputePlanDiff(runtimeName, scope, kitID string, roots map[string]string, plan InstallPlan, desired []adapter.Artifact, includeNoop bool) (PlanDiff, error) {
	profile, ok := adapter.Profile(runtimeName)
	if !ok {
		return PlanDiff{}, fmt.Errorf("unsupported runtime %q", runtimeName)
	}
	desiredByKey := make(map[string]adapter.Artifact, len(desired))
	for _, a := range desired {
		desiredByKey[strings.ToLower(a.Root+"\x00"+a.Path)] = a
	}

	out := PlanDiff{Changes: make([]ArtifactDiff, 0, len(plan.Actions))}
	for _, act := range plan.Actions {
		if act.Kind == "noop" && !includeNoop {
			continue
		}
		key := strings.ToLower(act.Root + "\x00" + act.Path)
		want, hasDesired := desiredByKey[key]
		strategy := want.Strategy
		if !hasDesired {
			strategy = inferArtifactStrategy(profile, scope, act.Path)
		}

		entry := ArtifactDiff{
			Root:        act.Root,
			Path:        act.Path,
			Strategy:    strategy,
			ComponentID: want.ComponentID,
			Owner:       kitID,
			Status:      act.Kind,
			Reason:      act.Reason,
		}
		if hasDesired {
			entry.AfterDigest = digest(want.Content)
		}
		if act.Kind == "conflict" && strings.HasPrefix(act.Reason, "artifact_owned_by_") {
			entry.Owner = strings.TrimPrefix(act.Reason, "artifact_owned_by_")
		}

		before, hasBefore, canExposeBefore, err := readOwnedBeforeBytes(runtimeName, scope, kitID, roots, act, strategy)
		if err != nil {
			return PlanDiff{}, err
		}
		if hasBefore {
			entry.BeforeDigest = digest(before)
		}

		var after []byte
		if hasDesired {
			after = want.Content
		}

		if shouldComputePatch(act, canExposeBefore) {
			if out.TotalBytes >= MaxTotalDiffBytes {
				entry.Truncated = true
				out.Truncated = true
			} else {
				remaining := MaxTotalDiffBytes - out.TotalBytes
				limit := MaxFileDiffBytes
				if remaining < limit {
					limit = remaining
				}
				patch, binary, trunc := UnifiedDiff(act.Path, act.Path, before, hasBefore, after, hasDesired, limit)
				entry.Binary = binary
				entry.Truncated = trunc
				entry.Patch = patch
				out.TotalBytes += len(patch)
				if trunc {
					out.Truncated = true
				}
			}
		}

		out.Changes = append(out.Changes, entry)
	}
	return out, nil
}

func inferArtifactStrategy(profile adapter.RuntimeProfile, scope, relPath string) string {
	if loc, hasInstr := profile.Instructions[scope]; hasInstr && loc.EffectiveStrategy() == "managed-block" && loc.Path == relPath {
		return "managed-block"
	}
	return "whole-file"
}

func shouldComputePatch(act Action, canExposeBefore bool) bool {
	switch act.Kind {
	case "create":
		return true
	case "update", "remove":
		return canExposeBefore
	case "conflict":
		return act.Reason == "managed_file_modified" || act.Reason == "managed_block_modified"
	default:
		return false
	}
}

func readOwnedBeforeBytes(runtimeName, scope, kitID string, roots map[string]string, act Action, strategy string) ([]byte, bool, bool, error) {
	switch act.Kind {
	case "create":
		return nil, false, true, nil
	case "noop":
		return nil, false, false, nil
	case "remove":
		if act.Reason == "already_missing_metadata_only" {
			return nil, false, false, nil
		}
	case "conflict":
		if act.Reason != "managed_file_modified" && act.Reason != "managed_block_modified" {
			// Privacy & safety boundary: never inspect or dump unowned files or oversized targets.
			return nil, false, false, nil
		}
	}

	target, err := ResolveTarget(roots[act.Root], act.Path)
	if err != nil {
		return nil, false, false, err
	}
	info, statErr := os.Lstat(target)
	if os.IsNotExist(statErr) {
		return nil, false, true, nil
	}
	if statErr != nil {
		return nil, false, false, statErr
	}
	if info.Size() > MaxInstructionBytes || !info.Mode().IsRegular() {
		return nil, false, false, nil
	}
	raw, err := readLimited(target, MaxInstructionBytes)
	if err != nil {
		return nil, false, false, err
	}
	if strategy == "managed-block" {
		block, found, bErr := ExtractBlockForRuntime(runtimeName, kitID, raw, scope)
		if bErr != nil {
			return nil, false, false, nil
		}
		if !found {
			return nil, false, true, nil
		}
		return block, true, true, nil
	}
	return raw, true, true, nil
}

// UnifiedDiff formats a bounded, binary-safe line-based unified diff between before and after.
func UnifiedDiff(oldPath, newPath string, before []byte, hasBefore bool, after []byte, hasAfter bool, maxBytes int) (patch string, binary bool, truncated bool) {
	if (!utf8.Valid(before) || bytes.IndexByte(before, 0) >= 0) || (!utf8.Valid(after) || bytes.IndexByte(after, 0) >= 0) {
		return "", true, false
	}
	if hasBefore && hasAfter && bytes.Equal(before, after) {
		return "", false, false
	}

	oldLabel := "/dev/null"
	if hasBefore {
		oldLabel = "a/" + oldPath
	}
	newLabel := "/dev/null"
	if hasAfter {
		newLabel = "b/" + newPath
	}

	aLines := splitLinesNormalized(before, hasBefore)
	bLines := splitLinesNormalized(after, hasAfter)

	var buf strings.Builder
	fmt.Fprintf(&buf, "--- %s\n+++ %s\n", oldLabel, newLabel)

	if len(aLines) > maxDiffLines || len(bLines) > maxDiffLines {
		fmt.Fprintf(&buf, "@@ -1,%d +1,%d @@\n... [diff omitted: file exceeds %d-line preview limit]\n", len(aLines), len(bLines), maxDiffLines)
		patch, _, _ := boundPatch(buf.String(), maxBytes)
		return patch, false, true
	}

	ops := diffLineOps(aLines, bLines)
	hunks := buildHunks(ops, diffContextLines)
	for _, h := range hunks {
		fmt.Fprintf(&buf, "@@ -%d,%d +%d,%d @@\n", h.oldStart, h.oldCount, h.newStart, h.newCount)
		for _, op := range h.ops {
			buf.WriteByte(op.kind)
			buf.WriteString(op.text)
			buf.WriteByte('\n')
		}
	}
	return boundPatch(buf.String(), maxBytes)
}

func boundPatch(raw string, maxBytes int) (string, bool, bool) {
	if maxBytes <= 0 || len(raw) <= maxBytes {
		return raw, false, false
	}
	const marker = "\n... [truncated]\n"
	if maxBytes <= len(marker) {
		return marker[:maxBytes], false, true
	}
	cut := maxBytes - len(marker)
	for cut > 0 && !utf8.RuneStart(raw[cut]) {
		cut--
	}
	return raw[:cut] + marker, false, true
}

func splitLinesNormalized(data []byte, present bool) []string {
	if !present || len(data) == 0 {
		return nil
	}
	s := strings.ReplaceAll(string(data), "\r\n", "\n")
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

type lineOp struct {
	kind byte // ' ', '-', '+'
	text string
	aIdx int
	bIdx int
}

type diffHunk struct {
	oldStart int
	oldCount int
	newStart int
	newCount int
	ops      []lineOp
}

func diffLineOps(a, b []string) []lineOp {
	n, m := len(a), len(b)
	// Strip common prefix/suffix first to keep DP tiny for typical edits.
	prefix := 0
	for prefix < n && prefix < m && a[prefix] == b[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < n-prefix && suffix < m-prefix && a[n-1-suffix] == b[m-1-suffix] {
		suffix++
	}

	ops := make([]lineOp, 0, n+m)
	for i := 0; i < prefix; i++ {
		ops = append(ops, lineOp{kind: ' ', text: a[i], aIdx: i, bIdx: i})
	}

	aMid := a[prefix : n-suffix]
	bMid := b[prefix : m-suffix]
	ops = append(ops, lcsMidOps(aMid, bMid, prefix, prefix)...)

	for i := 0; i < suffix; i++ {
		ai := n - suffix + i
		bi := m - suffix + i
		ops = append(ops, lineOp{kind: ' ', text: a[ai], aIdx: ai, bIdx: bi})
	}
	return ops
}

func lcsMidOps(a, b []string, aOff, bOff int) []lineOp {
	n, m := len(a), len(b)
	if n == 0 && m == 0 {
		return nil
	}
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	var ops []lineOp
	i, j := 0, 0
	for i < n && j < m {
		if a[i] == b[j] {
			ops = append(ops, lineOp{kind: ' ', text: a[i], aIdx: aOff + i, bIdx: bOff + j})
			i++
			j++
		} else if dp[i+1][j] >= dp[i][j+1] {
			ops = append(ops, lineOp{kind: '-', text: a[i], aIdx: aOff + i, bIdx: -1})
			i++
		} else {
			ops = append(ops, lineOp{kind: '+', text: b[j], aIdx: -1, bIdx: bOff + j})
			j++
		}
	}
	for i < n {
		ops = append(ops, lineOp{kind: '-', text: a[i], aIdx: aOff + i, bIdx: -1})
		i++
	}
	for j < m {
		ops = append(ops, lineOp{kind: '+', text: b[j], aIdx: -1, bIdx: bOff + j})
		j++
	}
	return ops
}

func buildHunks(ops []lineOp, context int) []diffHunk {
	var hunks []diffHunk
	i := 0
	for i < len(ops) {
		if ops[i].kind == ' ' {
			i++
			continue
		}
		start := i - context
		if start < 0 {
			start = 0
		}
		end := i
		for end < len(ops) {
			if ops[end].kind != ' ' {
				end++
				continue
			}
			// Bridge nearby changes separated by <= 2*context unchanged lines.
			nextChange := end
			for nextChange < len(ops) && ops[nextChange].kind == ' ' {
				nextChange++
			}
			if nextChange < len(ops) && nextChange-end <= context*2 {
				end = nextChange
				continue
			}
			break
		}
		hunkEnd := end + context
		if hunkEnd > len(ops) {
			hunkEnd = len(ops)
		}
		slice := ops[start:hunkEnd]
		hunks = append(hunks, summarizeHunk(slice))
		i = hunkEnd
	}
	return hunks
}

func summarizeHunk(slice []lineOp) diffHunk {
	oldCount, newCount := 0, 0
	oldStart, newStart := 0, 0
	for _, op := range slice {
		switch op.kind {
		case ' ':
			if oldStart == 0 {
				oldStart = op.aIdx + 1
			}
			if newStart == 0 {
				newStart = op.bIdx + 1
			}
			oldCount++
			newCount++
		case '-':
			if oldStart == 0 {
				oldStart = op.aIdx + 1
			}
			oldCount++
		case '+':
			if newStart == 0 {
				newStart = op.bIdx + 1
			}
			newCount++
		}
	}
	return diffHunk{
		oldStart: oldStart,
		oldCount: oldCount,
		newStart: newStart,
		newCount: newCount,
		ops:      slice,
	}
}
