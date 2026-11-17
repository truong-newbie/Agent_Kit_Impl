package hooks

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
)

const (
	EventSessionStart     = "SessionStart"
	EventPostToolUse      = "PostToolUse"
	EventPreToolUse       = "PreToolUse"
	EventBeforeTool       = "BeforeTool"
	EventAfterTool        = "AfterTool"
	EventAfterFileEdit    = "afterFileEdit"
	EventToolExecuteAfter = "tool.execute.after"
	DefaultTimeoutSec     = 3
	MaxHooksJSONBytes     = 4 * 1024 * 1024
)

func BuildSessionContextEntry(posixCmd, winCmd string, timeoutSec int) map[string]any {
	if timeoutSec <= 0 {
		timeoutSec = DefaultTimeoutSec
	}
	return map[string]any{
		"matcher": "startup|resume|clear|compact",
		"hooks": []any{
			map[string]any{
				"type":           "command",
				"command":        posixCmd,
				"commandWindows": winCmd,
				"timeout":        timeoutSec,
				"statusMessage":  "DTKit session context",
			},
		},
	}
}

// BuildClaudeHookEntry constructs a Claude Code settings.json hook matcher group.
func BuildClaudeHookEntry(command, matcher string, timeoutSec int, statusMessage string) map[string]any {
	if timeoutSec <= 0 {
		timeoutSec = 10
	}
	return map[string]any{
		"matcher": matcher,
		"hooks": []any{
			map[string]any{
				"type":          "command",
				"command":       command,
				"timeout":       timeoutSec,
				"statusMessage": statusMessage,
			},
		},
	}
}

// BuildGeminiHookEntry constructs a Gemini CLI settings.json hook matcher group (timeout in ms).
func BuildGeminiHookEntry(name, command, matcher string, timeoutSec int) map[string]any {
	if timeoutSec <= 0 {
		timeoutSec = 10
	}
	return map[string]any{
		"matcher": matcher,
		"hooks": []any{
			map[string]any{
				"name":    name,
				"type":    "command",
				"command": command,
				"timeout": timeoutSec * 1000,
			},
		},
	}
}

// BuildCursorHookEntry constructs a Cursor .cursor/hooks.json command entry.
func BuildCursorHookEntry(command string) map[string]any {
	return map[string]any{
		"command": command,
	}
}

// MergeCursorHooksJSON inserts entry into Cursor hooks.json (requiring root version: 1) while preserving existing hooks.
func MergeCursorHooksJSON(existing []byte, exists bool, eventName string, entry map[string]any) ([]byte, string, error) {
	if !exists || len(bytes.TrimSpace(existing)) == 0 {
		seed := []byte("{\n  \"version\": 1,\n  \"hooks\": {}\n}\n")
		return MergeEventHookJSON(seed, true, eventName, entry)
	}
	root, err := parseHooksObject(existing)
	if err != nil {
		return nil, "", err
	}
	if ver, hasVer := root["version"]; hasVer {
		if fmt.Sprint(ver) != "1" {
			return nil, "", apperr.Conflict("unsupported_hooks_schema", fmt.Sprintf("unsupported Cursor hooks.json version %v (expected 1)", ver))
		}
	} else {
		root["version"] = 1
		updated, err := json.MarshalIndent(root, "", "  ")
		if err != nil {
			return nil, "", err
		}
		existing = updated
	}
	return MergeEventHookJSON(existing, true, eventName, entry)
}

// UnmergeCursorHooksJSON removes a DTKit entry from Cursor hooks.json while keeping root version: 1 if user hooks remain.
func UnmergeCursorHooksJSON(existing []byte, eventName, expectedDigest string, createdFile bool) ([]byte, bool, error) {
	out, _, err := UnmergeEventHookJSON(existing, eventName, expectedDigest, false)
	if err != nil {
		return nil, false, err
	}
	if createdFile {
		root, pErr := parseHooksObject(out)
		if pErr == nil {
			if _, hasHooks := root["hooks"]; !hasHooks && len(root) == 1 && fmt.Sprint(root["version"]) == "1" {
				return nil, true, nil
			}
		}
	}
	return out, false, nil
}

// CanonicalEntryDigest hashes the canonical JSON serialization of a Codex hook matcher group.
// DTKit tracks ownership in state.json rather than injecting non-standard keys into hooks.json.
func CanonicalEntryDigest(entry any) (string, error) {
	b, err := json.Marshal(entry)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// MergeHooksJSON structurally inserts entry into Codex hooks.json SessionStart while preserving all user config.
func MergeHooksJSON(existing []byte, exists bool, entry map[string]any) (out []byte, digest string, err error) {
	return MergeEventHookJSON(existing, exists, EventSessionStart, entry)
}

// MergeEventHookJSON structurally inserts entry under hooks.<eventName> while preserving
// all unrelated top-level settings, events, and user-authored hooks.
func MergeEventHookJSON(existing []byte, exists bool, eventName string, entry map[string]any) (out []byte, digest string, err error) {
	digest, err = CanonicalEntryDigest(entry)
	if err != nil {
		return nil, "", err
	}
	if exists && len(existing) > MaxHooksJSONBytes {
		return nil, "", apperr.Conflict("hooks_json_too_large", "hooks config exceeds 4 MiB limit")
	}
	var root map[string]any
	if !exists || len(bytes.TrimSpace(existing)) == 0 {
		root = map[string]any{}
	} else {
		root, err = parseHooksObject(existing)
		if err != nil {
			return nil, "", err
		}
	}

	var hooksMap map[string]any
	if rawHooks, ok := root["hooks"]; ok {
		hm, ok := rawHooks.(map[string]any)
		if !ok {
			return nil, "", apperr.Conflict("unsupported_hooks_schema", "existing config 'hooks' field is not an object")
		}
		hooksMap = hm
	} else {
		hooksMap = map[string]any{}
		root["hooks"] = hooksMap
	}

	var evList []any
	if rawEv, ok := hooksMap[eventName]; ok {
		sl, ok := rawEv.([]any)
		if !ok {
			return nil, "", apperr.Conflict("unsupported_hooks_schema", fmt.Sprintf("existing config '%s' field is not an array", eventName))
		}
		for _, item := range sl {
			itemDig, dErr := CanonicalEntryDigest(item)
			if dErr == nil && itemDig == digest {
				return nil, "", apperr.Conflict("hook_already_present", fmt.Sprintf("matching DTKit hook entry already exists under %s without matching state", eventName))
			}
		}
		evList = append([]any{}, sl...)
	}
	evList = append(evList, entry)
	hooksMap[eventName] = evList

	encoded, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, "", err
	}
	return append(encoded, '\n'), digest, nil
}

// UnmergeHooksJSON removes only the DTKit SessionStart entry matching expectedDigest.
func UnmergeHooksJSON(existing []byte, expectedDigest string, createdFile bool) (out []byte, deleteFile bool, err error) {
	return UnmergeEventHookJSON(existing, EventSessionStart, expectedDigest, createdFile)
}

// UnmergeEventHookJSON removes only the DTKit entry under hooks.<eventName> matching expectedDigest.
func UnmergeEventHookJSON(existing []byte, eventName, expectedDigest string, createdFile bool) (out []byte, deleteFile bool, err error) {
	root, err := parseHooksObject(existing)
	if err != nil {
		return nil, false, err
	}
	rawHooks, ok := root["hooks"]
	if !ok {
		return nil, false, apperr.Conflict("hook_entry_missing", "config is missing 'hooks' object")
	}
	hooksMap, ok := rawHooks.(map[string]any)
	if !ok {
		return nil, false, apperr.Conflict("unsupported_hooks_schema", "config 'hooks' field is not an object")
	}
	rawEv, ok := hooksMap[eventName]
	if !ok {
		return nil, false, apperr.Conflict("hook_entry_missing", fmt.Sprintf("config is missing '%s' array", eventName))
	}
	evList, ok := rawEv.([]any)
	if !ok {
		return nil, false, apperr.Conflict("unsupported_hooks_schema", fmt.Sprintf("config '%s' field is not an array", eventName))
	}

	kept := make([]any, 0, len(evList))
	removedCount := 0
	for _, item := range evList {
		d, dErr := CanonicalEntryDigest(item)
		if dErr == nil && d == expectedDigest {
			removedCount++
			continue
		}
		kept = append(kept, item)
	}
	if removedCount == 0 {
		return nil, false, apperr.Conflict("hook_entry_modified", "DTKit hook entry in hooks config was modified or removed externally")
	}
	if removedCount > 1 {
		return nil, false, apperr.WithHint(
			apperr.Conflict("hook_entry_ambiguous", fmt.Sprintf("multiple (%d) entries share digest %s; cannot safely disambiguate ownership", removedCount, expectedDigest)),
			"remove duplicate entries manually before disabling or uninstalling",
		)
	}

	if len(kept) == 0 {
		delete(hooksMap, eventName)
	} else {
		hooksMap[eventName] = kept
	}
	if len(hooksMap) == 0 {
		delete(root, "hooks")
	}
	if len(root) == 0 && createdFile {
		return nil, true, nil
	}

	encoded, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, false, err
	}
	return append(encoded, '\n'), false, nil
}

// InspectHooksJSON checks whether expectedDigest is intact in Codex hooks.json SessionStart.
func InspectHooksJSON(existing []byte, expectedDigest string) (present bool, err error) {
	return InspectEventHooksJSON(existing, EventSessionStart, expectedDigest)
}

// InspectEventHooksJSON checks whether expectedDigest is intact under hooks.<eventName>.
func InspectEventHooksJSON(existing []byte, eventName, expectedDigest string) (present bool, err error) {
	root, err := parseHooksObject(existing)
	if err != nil {
		return false, err
	}
	rawHooks, ok := root["hooks"]
	if !ok {
		return false, nil
	}
	hooksMap, ok := rawHooks.(map[string]any)
	if !ok {
		return false, apperr.Conflict("unsupported_hooks_schema", "config 'hooks' field is not an object")
	}
	rawEv, ok := hooksMap[eventName]
	if !ok {
		return false, nil
	}
	evList, ok := rawEv.([]any)
	if !ok {
		return false, apperr.Conflict("unsupported_hooks_schema", fmt.Sprintf("config '%s' field is not an array", eventName))
	}
	matchCount := 0
	for _, item := range evList {
		if d, dErr := CanonicalEntryDigest(item); dErr == nil && d == expectedDigest {
			matchCount++
		}
	}
	if matchCount > 1 {
		return false, apperr.Conflict("hook_entry_ambiguous", fmt.Sprintf("multiple (%d) entries share digest %s", matchCount, expectedDigest))
	}
	return matchCount == 1, nil
}

func parseHooksObject(data []byte) (map[string]any, error) {
	if !utf8.Valid(data) {
		return nil, apperr.Conflict("invalid_hooks_json", "hooks.json is not valid UTF-8")
	}
	if len(data) > MaxHooksJSONBytes {
		return nil, apperr.Conflict("hooks_json_too_large", "hooks.json exceeds 4 MiB limit")
	}
	if err := catalog.CheckStrictJSON(data); err != nil {
		return nil, apperr.Wrap(apperr.ExitConflict, "invalid_hooks_json", fmt.Sprintf("invalid hooks.json: %v", err), err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var root map[string]any
	if err := dec.Decode(&root); err != nil {
		return nil, apperr.Wrap(apperr.ExitConflict, "invalid_hooks_json", fmt.Sprintf("invalid hooks.json: %v", err), err)
	}
	if root == nil {
		return nil, apperr.Conflict("invalid_hooks_json", "hooks.json root must be a JSON object")
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return nil, apperr.Conflict("invalid_hooks_json", "hooks.json contains trailing JSON data")
	}
	if rawHooks, ok := root["hooks"]; ok {
		hm, ok := rawHooks.(map[string]any)
		if !ok {
			return nil, apperr.Conflict("unsupported_hooks_schema", "existing hooks.json 'hooks' field must be an object")
		}
		for evName, rawList := range hm {
			items, ok := rawList.([]any)
			if !ok {
				return nil, apperr.Conflict("unsupported_hooks_schema", fmt.Sprintf("existing hooks.json event %q must be an array", evName))
			}
			for _, item := range items {
				if _, isObj := item.(map[string]any); !isObj {
					return nil, apperr.Conflict("unsupported_hooks_schema", fmt.Sprintf("existing hooks.json event %q entries must be JSON objects", evName))
				}
			}
		}
	}
	return root, nil
}
