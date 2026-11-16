package state

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

const MaxStateBytes = 4 * 1024 * 1024

// LoadFile reads and strictly validates an existing DTKit installation state file.
// Returns (nil, false, nil) if the file does not exist.
func LoadFile(statePath, expectedScope string) (*Installation, bool, error) {
	return LoadFileForRuntime(statePath, expectedScope, "")
}

// LoadFileForRuntime reads and strictly validates an existing state file for scope and optional runtime.
func LoadFileForRuntime(statePath, expectedScope, expectedRuntime string) (*Installation, bool, error) {
	info, err := os.Lstat(statePath)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if paths.IsReparseOrSymlink(info) || !info.Mode().IsRegular() {
		return nil, true, apperr.WithPath(apperr.Conflict("invalid_state_file", "installation state is not a regular file"), statePath)
	}
	if info.Size() > MaxStateBytes {
		return nil, true, apperr.WithPath(apperr.Conflict("state_too_large", "installation state exceeds 4 MiB"), statePath)
	}
	f, err := os.Open(statePath)
	if err != nil {
		return nil, true, err
	}
	data, readErr := io.ReadAll(io.LimitReader(f, MaxStateBytes+1))
	closeErr := f.Close()
	if readErr != nil {
		return nil, true, readErr
	}
	if closeErr != nil {
		return nil, true, closeErr
	}
	if len(data) > MaxStateBytes {
		return nil, true, apperr.WithPath(apperr.Conflict("state_too_large", "installation state exceeds 4 MiB"), statePath)
	}
	inst, err := ParseAndValidate(data, expectedScope)
	if err != nil {
		return nil, true, apperr.WithPath(apperr.Wrap(apperr.ExitConflict, "invalid_state", err.Error(), err), statePath)
	}
	if expectedRuntime != "" && inst.Runtime != expectedRuntime {
		return nil, true, apperr.WithPath(apperr.Conflict("runtime_mismatch", fmt.Sprintf("installation state runtime %q does not match requested %q", inst.Runtime, expectedRuntime)), statePath)
	}
	return &inst, true, nil
}

// ParseAndValidate strictly decodes and validates state JSON bytes.
func ParseAndValidate(data []byte, expectedScope string) (Installation, error) {
	var inst Installation
	if !utf8.Valid(data) {
		return inst, fmt.Errorf("installation state is not valid UTF-8")
	}
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return inst, fmt.Errorf("invalid installation state JSON: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&inst); err != nil {
		return inst, fmt.Errorf("invalid installation state: %w", err)
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return inst, fmt.Errorf("installation state must contain one JSON object")
	}
	if expectedScope != "" && inst.Scope != expectedScope {
		return inst, fmt.Errorf("installation state scope %q does not match requested %q", inst.Scope, expectedScope)
	}
	if err := inst.ValidateSelf(); err != nil {
		return inst, err
	}
	return inst, nil
}

func rejectDuplicateJSONKeys(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 64 {
			return fmt.Errorf("JSON nesting limit exceeded")
		}
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		delim, ok := tok.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for dec.More() {
				k, err := dec.Token()
				if err != nil {
					return err
				}
				ks, ok := k.(string)
				if !ok {
					return fmt.Errorf("expected object key")
				}
				if seen[ks] {
					return fmt.Errorf("duplicate key %q", strings.Clone(ks))
				}
				seen[ks] = true
				if err := walk(depth + 1); err != nil {
					return err
				}
			}
		case '[':
			for dec.More() {
				if err := walk(depth + 1); err != nil {
					return err
				}
			}
		}
		_, err = dec.Token()
		return err
	}
	if err := walk(0); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return fmt.Errorf("trailing JSON data")
	}
	return nil
}
