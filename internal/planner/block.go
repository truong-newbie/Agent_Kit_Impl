// Package planner contains read-only operations used to prepare filesystem changes.
package planner

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	"dtkit.local/dtkit/internal/state"
)

// BlockState records only the bytes DTKit owns, not the whole shared file.
type BlockState = state.BlockState

func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func markers(kitID, scope string) (string, string, error) {
	if kitID == "" || strings.ContainsAny(kitID, " \t\r\n") {
		return "", "", fmt.Errorf("invalid kit id %q", kitID)
	}
	if scope != "project" && scope != "global" {
		return "", "", fmt.Errorf("invalid scope %q", scope)
	}
	return "<!-- dtkit:begin " + kitID + " " + scope + " -->", "<!-- dtkit:end " + kitID + " " + scope + " -->", nil
}

// locate returns the exact byte range including the end marker's newline.
// All malformed or repeated DTKit markers fail closed.
func locate(data []byte, kitID, scope string) (int, int, error) {
	begin, end, err := markers(kitID, scope)
	if err != nil {
		return 0, 0, err
	}
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return 0, 0, fmt.Errorf("instruction file must be UTF-8 text")
	}
	start, stop, active := -1, -1, false
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
		if line == begin {
			if start >= 0 || active {
				return 0, 0, fmt.Errorf("duplicate begin marker")
			}
			start, active = pos, true
		} else if line == end {
			if !active || stop >= 0 {
				return 0, 0, fmt.Errorf("unmatched end marker")
			}
			stop, active = next, false
		} else if strings.HasPrefix(line, "<!-- dtkit:begin ") || strings.HasPrefix(line, "<!-- dtkit:end ") {
			return 0, 0, fmt.Errorf("unexpected DTKit marker")
		}
		pos = next
	}
	if active || (start >= 0 && stop < 0) {
		return 0, 0, fmt.Errorf("unclosed DTKit block")
	}
	return start, stop, nil
}

func validateRendered(block []byte, kitID, scope string) error {
	if bytes.Contains(block, []byte("\r")) {
		return fmt.Errorf("rendered block must use LF newlines")
	}
	start, end, err := locate(block, kitID, scope)
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
	return AppendBlockForKit("engineer", existing, exists, block, scope)
}

// AppendBlockForKit plans adding a managed block for kitID while retaining existing bytes.
func AppendBlockForKit(kitID string, existing []byte, exists bool, block []byte, scope string) ([]byte, BlockState, error) {
	var state BlockState
	if !exists && len(existing) != 0 {
		return nil, state, fmt.Errorf("missing file cannot have existing content")
	}
	if err := validateRendered(block, kitID, scope); err != nil {
		return nil, state, err
	}
	start, _, err := locate(existing, kitID, scope)
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
	return ReplaceBlockForKit("engineer", existing, block, scope, state)
}

// ReplaceBlockForKit changes only a block for kitID that still has the recorded digest.
func ReplaceBlockForKit(kitID string, existing, block []byte, scope string, state BlockState) ([]byte, BlockState, error) {
	if err := validateState(state); err != nil {
		return nil, state, err
	}
	if err := validateRendered(block, kitID, scope); err != nil {
		return nil, state, err
	}
	start, end, err := locate(existing, kitID, scope)
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
	return BlockDigestForKit("engineer", existing, scope)
}

// BlockDigestForKit returns the digest of kitID's exact managed block.
func BlockDigestForKit(kitID string, existing []byte, scope string) (string, error) {
	start, end, err := locate(existing, kitID, scope)
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
	return RemoveBlockForKit("engineer", existing, scope, state)
}

// RemoveBlockForKit returns the planned bytes and whether the entire file may be removed.
func RemoveBlockForKit(kitID string, existing []byte, scope string, state BlockState) ([]byte, bool, error) {
	if err := validateState(state); err != nil {
		return nil, false, err
	}
	start, end, err := locate(existing, kitID, scope)
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
