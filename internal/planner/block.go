// Package planner contains read-only operations used to prepare filesystem changes.
package planner

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/state"
)

// BlockState records only the bytes DTKit owns, not the whole shared file.
type BlockState = state.BlockState

var blockTagPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*(?: [a-z][a-z0-9-]*)? (?:project|global)$`)

func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func blockTag(runtimeName, kitID, scope string) (string, error) {
	if kitID == "" || strings.ContainsAny(kitID, " \t\r\n") {
		return "", fmt.Errorf("invalid kit id %q", kitID)
	}
	if scope != "project" && scope != "global" {
		return "", fmt.Errorf("invalid scope %q", scope)
	}
	if runtimeName != "" && runtimeName != "codex" && runtimeName != "gemini" {
		if strings.ContainsAny(runtimeName, " \t\r\n") {
			return "", fmt.Errorf("invalid runtime name %q", runtimeName)
		}
		return kitID + " " + runtimeName + " " + scope, nil
	}
	return kitID + " " + scope, nil
}

func markers(runtimeName, kitID, scope string) (string, string, string, error) {
	tag, err := blockTag(runtimeName, kitID, scope)
	if err != nil {
		return "", "", "", err
	}
	return tag, "<!-- dtkit:begin " + tag + " -->", "<!-- dtkit:end " + tag + " -->", nil
}

func parseMarkerLine(line string) (kind, tag string, isMarker, valid bool) {
	if strings.HasPrefix(line, "<!-- dtkit:begin ") {
		if !strings.HasSuffix(line, " -->") {
			return "", "", true, false
		}
		t := strings.TrimSuffix(strings.TrimPrefix(line, "<!-- dtkit:begin "), " -->")
		return "begin", t, true, blockTagPattern.MatchString(t)
	}
	if strings.HasPrefix(line, "<!-- dtkit:end ") {
		if !strings.HasSuffix(line, " -->") {
			return "", "", true, false
		}
		t := strings.TrimSuffix(strings.TrimPrefix(line, "<!-- dtkit:end "), " -->")
		return "end", t, true, blockTagPattern.MatchString(t)
	}
	return "", "", false, false
}

// locate returns the exact byte range including the end marker's newline.
// All malformed, nested, unclosed, or repeated DTKit markers fail closed.
func locate(data []byte, runtimeName, kitID, scope string) (int, int, error) {
	targetTag, _, _, err := markers(runtimeName, kitID, scope)
	if err != nil {
		return 0, 0, err
	}
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return 0, 0, fmt.Errorf("instruction file must be UTF-8 text")
	}
	start, stop := -1, -1
	activeTag := ""
	for pos := 0; pos < len(data); {
		lineEnd := bytes.IndexByte(data[pos:], '\n')
		next := len(data)
		if lineEnd >= 0 {
			next = pos + lineEnd + 1
		}
		line := strings.TrimSuffix(string(data[pos:next]), "\n")
		line = strings.TrimSuffix(line, "\r")
		if pos == 0 {
			line = strings.TrimPrefix(line, "\uFEFF")
		}
		if kind, tag, isMarker, valid := parseMarkerLine(line); isMarker {
			if !valid {
				return 0, 0, fmt.Errorf("unexpected DTKit marker")
			}
			if kind == "begin" {
				if activeTag != "" {
					return 0, 0, fmt.Errorf("unexpected nested DTKit marker")
				}
				if tag == targetTag {
					if start >= 0 {
						return 0, 0, fmt.Errorf("duplicate begin marker")
					}
					start = pos
				}
				activeTag = tag
			} else {
				if activeTag == "" || activeTag != tag {
					return 0, 0, fmt.Errorf("unmatched end marker")
				}
				if tag == targetTag {
					if stop >= 0 {
						return 0, 0, fmt.Errorf("unmatched end marker")
					}
					stop = next
				}
				activeTag = ""
			}
		}
		pos = next
	}
	if activeTag != "" || (start >= 0 && stop < 0) {
		return 0, 0, fmt.Errorf("unclosed DTKit block")
	}
	return start, stop, nil
}

func validateRendered(block []byte, runtimeName, kitID, scope string) error {
	if bytes.Contains(block, []byte("\r")) {
		return fmt.Errorf("rendered block must use LF newlines")
	}
	start, end, err := locate(block, runtimeName, kitID, scope)
	if err != nil {
		return err
	}
	if start != 0 || end != len(block) {
		return fmt.Errorf("rendered instruction is not exactly one block")
	}
	return nil
}

// AppendBlock plans adding an engineer managed block while retaining existing bytes.
func AppendBlock(existing []byte, exists bool, block []byte, scope string) ([]byte, BlockState, error) {
	return AppendBlockForRuntime("codex", "engineer", existing, exists, block, scope)
}

// AppendBlockForKit plans adding a Codex managed block for kitID while retaining existing bytes.
func AppendBlockForKit(kitID string, existing []byte, exists bool, block []byte, scope string) ([]byte, BlockState, error) {
	return AppendBlockForRuntime("codex", kitID, existing, exists, block, scope)
}

// AppendBlockForRuntime plans adding a managed block for runtimeName + kitID while preserving existing content and valid sibling blocks.
func AppendBlockForRuntime(runtimeName, kitID string, existing []byte, exists bool, block []byte, scope string) ([]byte, BlockState, error) {
	var state BlockState
	if !exists && len(existing) != 0 {
		return nil, state, fmt.Errorf("missing file cannot have existing content")
	}
	if err := validateRendered(block, runtimeName, kitID, scope); err != nil {
		return nil, state, err
	}
	start, _, err := locate(existing, runtimeName, kitID, scope)
	if err != nil {
		return nil, state, err
	}
	if start >= 0 {
		return nil, state, fmt.Errorf("block exists without ownership")
	}
	newline := "\n"
	if bytes.Contains(existing, []byte("\r\n")) {
		newline = "\r\n"
	}
	if newline == "\r\n" {
		block = bytes.ReplaceAll(block, []byte("\n"), []byte("\r\n"))
	}
	separator := ""
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		separator = newline
	}
	out := make([]byte, 0, len(existing)+len(separator)+len(block))
	out = append(out, existing...)
	out = append(out, separator...)
	out = append(out, block...)
	return out, BlockState{Digest: digest(block), Separator: separator, CreatedFile: !exists}, nil
}

// ReplaceBlock changes only a block that still has the recorded digest.
func ReplaceBlock(existing, block []byte, scope string, state BlockState) ([]byte, BlockState, error) {
	return ReplaceBlockForRuntime("codex", "engineer", existing, block, scope, state)
}

// ReplaceBlockForKit changes only a Codex block for kitID that still has the recorded digest.
func ReplaceBlockForKit(kitID string, existing, block []byte, scope string, state BlockState) ([]byte, BlockState, error) {
	return ReplaceBlockForRuntime("codex", kitID, existing, block, scope, state)
}

// ReplaceBlockForRuntime changes only the owned runtime block matching state.Digest.
func ReplaceBlockForRuntime(runtimeName, kitID string, existing, block []byte, scope string, state BlockState) ([]byte, BlockState, error) {
	if err := validateState(state); err != nil {
		return nil, state, err
	}
	if err := validateRendered(block, runtimeName, kitID, scope); err != nil {
		return nil, state, err
	}
	start, end, err := locate(existing, runtimeName, kitID, scope)
	if err != nil {
		return nil, state, err
	}
	if start < 0 {
		return nil, state, fmt.Errorf("managed block is missing")
	}
	old := existing[start:end]
	if digest(old) != state.Digest {
		return nil, state, fmt.Errorf("managed block has changed")
	}
	if bytes.Contains(old, []byte("\r\n")) {
		block = bytes.ReplaceAll(block, []byte("\n"), []byte("\r\n"))
	}
	out := append([]byte{}, existing[:start]...)
	out = append(out, block...)
	out = append(out, existing[end:]...)
	state.Digest = digest(block)
	return out, state, nil
}

// BlockDigest returns the digest of DTKit's exact managed block.
func BlockDigest(existing []byte, scope string) (string, error) {
	return BlockDigestForRuntime("codex", "engineer", existing, scope)
}

// BlockDigestForKit returns the digest of kitID's exact Codex managed block.
func BlockDigestForKit(kitID string, existing []byte, scope string) (string, error) {
	return BlockDigestForRuntime("codex", kitID, existing, scope)
}

// BlockDigestForRuntime returns the digest of the runtime-specific managed block.
func BlockDigestForRuntime(runtimeName, kitID string, existing []byte, scope string) (string, error) {
	start, end, err := locate(existing, runtimeName, kitID, scope)
	if err != nil {
		return "", err
	}
	if start < 0 {
		return "", fmt.Errorf("managed block is missing")
	}
	return digest(existing[start:end]), nil
}

// RemoveBlock returns the planned bytes and whether the entire file may be removed.
func RemoveBlock(existing []byte, scope string, state BlockState) ([]byte, bool, error) {
	return RemoveBlockForRuntime("codex", "engineer", existing, scope, state)
}

// RemoveBlockForKit returns the planned bytes and whether the entire file may be removed.
func RemoveBlockForKit(kitID string, existing []byte, scope string, state BlockState) ([]byte, bool, error) {
	return RemoveBlockForRuntime("codex", kitID, existing, scope, state)
}

// RemoveBlockForRuntime removes only the runtime-specific owned block and keeps sibling blocks/user bytes.
func RemoveBlockForRuntime(runtimeName, kitID string, existing []byte, scope string, state BlockState) ([]byte, bool, error) {
	if err := validateState(state); err != nil {
		return nil, false, err
	}
	start, end, err := locate(existing, runtimeName, kitID, scope)
	if err != nil {
		return nil, false, err
	}
	if start < 0 {
		return nil, false, fmt.Errorf("managed block is missing")
	}
	if digest(existing[start:end]) != state.Digest {
		return nil, false, fmt.Errorf("managed block has changed")
	}
	prefix := existing[:start]
	if state.Separator != "" && bytes.HasSuffix(prefix, []byte(state.Separator)) {
		prefix = prefix[:len(prefix)-len(state.Separator)]
	}
	out := append([]byte{}, prefix...)
	out = append(out, existing[end:]...)
	return out, state.CreatedFile && len(out) == 0, nil
}

func validateState(state BlockState) error {
	decoded, err := hex.DecodeString(state.Digest)
	if err != nil || len(decoded) != sha256.Size || (state.Separator != "" && state.Separator != "\n" && state.Separator != "\r\n") {
		return fmt.Errorf("invalid managed block state")
	}
	return nil
}
