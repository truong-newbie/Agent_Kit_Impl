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
	EventSessionStart = "SessionStart"
	DefaultTimeoutSec = 3
	MaxHooksJSONBytes = 4 * 1024 * 1024
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

// MergeHooksJSON structurally inserts entry into Codex hooks.json while preserving
// all unrelated top-level keys, events, and user-authored hooks.
func MergeHooksJSON(existing []byte, exists bool, entry map[string]any) (out []byte, digest string, err error) {
	digest, err = CanonicalEntryDigest(entry)
	if err != nil {
		return nil, "", err
	}
	if exists && len(existing) > MaxHooksJSONBytes {
		return nil, "", apperr.Conflict("hooks_json_too_large", "hooks.json exceeds 4 MiB limit")
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
			return nil, "", apperr.Conflict("unsupported_hooks_schema", "existing hooks.json 'hooks' field is not an object")
		}
		hooksMap = hm
	} else {
		hooksMap = map[string]any{}
		root["hooks"] = hooksMap
	}

	var sessionList []any
	if rawSession, ok := hooksMap[EventSessionStart]; ok {
		sl, ok := rawSession.([]any)
		if !ok {
			return nil, "", apperr.Conflict("unsupported_hooks_schema", "existing hooks.json 'SessionStart' field is not an array")
		}
		for _, item := range sl {
			itemDig, dErr := CanonicalEntryDigest(item)
			if dErr == nil && itemDig == digest {
				return nil, "", apperr.Conflict("hook_already_present", "matching DTKit hook entry already exists in hooks.json without matching state")
			}
		}
		sessionList = append([]any{}, sl...)
	}
	sessionList = append(sessionList, entry)
	hooksMap[EventSessionStart] = sessionList

	encoded, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, "", err
	}
	return append(encoded, '\n'), digest, nil
}

// UnmergeHooksJSON removes only the DTKit entry matching expectedDigest, preserving all user config.
func UnmergeHooksJSON(existing []byte, expectedDigest string, createdFile bool) (out []byte, deleteFile bool, err error) {
	root, err := parseHooksObject(existing)
	if err != nil {
		return nil, false, err
	}
	rawHooks, ok := root["hooks"]
	if !ok {
		return nil, false, apperr.Conflict("hook_entry_missing", "hooks.json is missing 'hooks' object")
	}
	hooksMap, ok := rawHooks.(map[string]any)
	if !ok {
		return nil, false, apperr.Conflict("unsupported_hooks_schema", "hooks.json 'hooks' field is not an object")
	}
	rawSession, ok := hooksMap[EventSessionStart]
	if !ok {
		return nil, false, apperr.Conflict("hook_entry_missing", "hooks.json is missing 'SessionStart' array")
	}
	sessionList, ok := rawSession.([]any)
	if !ok {
		return nil, false, apperr.Conflict("unsupported_hooks_schema", "hooks.json 'SessionStart' field is not an array")
	}

	kept := make([]any, 0, len(sessionList))
	removedCount := 0
	for _, item := range sessionList {
		d, dErr := CanonicalEntryDigest(item)
		if dErr == nil && d == expectedDigest {
			removedCount++
			continue
		}
		kept = append(kept, item)
	}
	if removedCount == 0 {
		return nil, false, apperr.Conflict("hook_entry_modified", "DTKit hook entry in hooks.json was modified or removed externally")
	}
	if removedCount > 1 {
		return nil, false, apperr.WithHint(
			apperr.Conflict("hook_entry_ambiguous", fmt.Sprintf("multiple (%d) hooks.json entries share digest %s; cannot safely disambiguate ownership", removedCount, expectedDigest)),
			"remove duplicate entries in hooks.json manually before disabling or uninstalling",
		)
	}

	if len(kept) == 0 {
		delete(hooksMap, EventSessionStart)
	} else {
		hooksMap[EventSessionStart] = kept
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

// InspectHooksJSON checks whether expectedDigest is intact in hooks.json.
func InspectHooksJSON(existing []byte, expectedDigest string) (present bool, err error) {
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
		return false, apperr.Conflict("unsupported_hooks_schema", "hooks.json 'hooks' field is not an object")
	}
	rawSession, ok := hooksMap[EventSessionStart]
	if !ok {
		return false, nil
	}
	sessionList, ok := rawSession.([]any)
	if !ok {
		return false, apperr.Conflict("unsupported_hooks_schema", "hooks.json 'SessionStart' field is not an array")
	}
	matchCount := 0
	for _, item := range sessionList {
		if d, dErr := CanonicalEntryDigest(item); dErr == nil && d == expectedDigest {
			matchCount++
		}
	}
	if matchCount > 1 {
		return false, apperr.Conflict("hook_entry_ambiguous", fmt.Sprintf("multiple (%d) hooks.json entries share digest %s", matchCount, expectedDigest))
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
