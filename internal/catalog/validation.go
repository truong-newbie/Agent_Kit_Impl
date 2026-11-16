package catalog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

const (
	MaxManifestBytes = 256 * 1024
	MaxComponents    = 128
	MaxFiles         = 512
	MaxFileBytes     = 1024 * 1024
	MaxPackageBytes  = 10 * 1024 * 1024
)

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)
var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

var disallowedDirs = map[string]bool{
	".git":         true,
	".dtkit":       true,
	"node_modules": true,
	"dist":         true,
}

var disallowedExtensions = map[string]bool{
	".exe": true, ".dll": true, ".so": true, ".dylib": true,
	".com": true, ".msi": true, ".bat": true, ".cmd": true,
	".ps1": true, ".psm1": true, ".psd1": true, ".sh": true,
	".bash": true, ".zsh": true, ".ksh": true, ".py": true,
	".pyc": true, ".rb": true, ".pl": true, ".js": true,
	".mjs": true, ".cjs": true, ".ts": true, ".vbs": true,
	".wsf": true, ".jar": true, ".scr": true, ".pif": true,
}

func validID(id string, max int) bool { return len(id) <= max && idPattern.MatchString(id) }

func subsetUnique(actual []string, allowed []string) bool {
	if len(actual) == 0 {
		return false
	}
	allowMap := make(map[string]bool, len(allowed))
	for _, a := range allowed {
		allowMap[a] = true
	}
	seen := make(map[string]bool, len(actual))
	for _, x := range actual {
		if !allowMap[x] || seen[x] {
			return false
		}
		seen[x] = true
	}
	return true
}

func checkManifestKeys(data []byte) error {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return err
	}
	if err := exactKeys(top, []string{"schemaVersion", "id", "name", "version", "description", "runtimeTargets", "scopes", "requiresCapabilities", "packageDependencies", "components"}); err != nil {
		return err
	}
	var components []map[string]json.RawMessage
	if err := json.Unmarshal(top["components"], &components); err != nil {
		return fmt.Errorf("components: %w", err)
	}
	for i, component := range components {
		if err := exactKeys(component, []string{"id", "kind", "source", "requires"}); err != nil {
			return fmt.Errorf("component %d: %w", i, err)
		}
	}
	return nil
}

func exactKeys(object map[string]json.RawMessage, expected []string) error {
	if object == nil {
		return fmt.Errorf("expected an object")
	}
	allowed := map[string]bool{}
	for _, key := range expected {
		allowed[key] = true
	}
	for key, value := range object {
		if !allowed[key] {
			return fmt.Errorf("unknown field %q", key)
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("field %q cannot be null", key)
		}
	}
	for _, key := range expected {
		if _, ok := object[key]; !ok {
			return fmt.Errorf("missing field %q", key)
		}
	}
	return nil
}

// checkTree rejects links, disallowed directories/executables, case collisions,
// and oversized package content before reading it.
func checkTree(fsys fs.FS) error {
	count, total := 0, int64(0)
	pathsSeen := map[string]string{}
	return fs.WalkDir(fsys, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		if !paths.ValidateRelativePath(name) {
			return apperr.WithPath(apperr.Usage("invalid_package_path", fmt.Sprintf("invalid package path %q", name)), name)
		}
		key := strings.ToLower(name)
		if prev, ok := pathsSeen[key]; ok {
			return apperr.WithPath(apperr.Usage("path_collision", fmt.Sprintf("case-insensitive path collision: %q and %q", prev, name)), name)
		}
		pathsSeen[key] = name
		if entry.Type()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
			return apperr.WithPath(apperr.Usage("symlink_not_allowed", fmt.Sprintf("symbolic link or reparse point not allowed: %q", name)), name)
		}
		if entry.IsDir() {
			if disallowedDirs[strings.ToLower(entry.Name())] {
				return apperr.WithPath(apperr.Usage("disallowed_directory", fmt.Sprintf("%s directory is not allowed in package", entry.Name())), name)
			}
			return nil
		}
		ext := strings.ToLower(path.Ext(name))
		if disallowedExtensions[ext] {
			return apperr.WithPath(apperr.Usage("disallowed_file_type", fmt.Sprintf("executable or script file %q is not allowed in v0.1 packages", name)), name)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&(fs.ModeSymlink|fs.ModeIrregular) != 0 || !info.Mode().IsRegular() {
			return apperr.WithPath(apperr.Usage("special_file_not_allowed", fmt.Sprintf("special file not allowed: %q", name)), name)
		}
		count++
		total += info.Size()
		if count > MaxFiles || info.Size() > MaxFileBytes || total > MaxPackageBytes {
			return apperr.WithPath(apperr.Usage("package_size_limit", fmt.Sprintf("package size or file count limit exceeded at %q", name)), name)
		}
		return nil
	})
}

func checkContentSafety(name string, data []byte) error {
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return apperr.WithPath(apperr.Usage("binary_file_not_allowed", fmt.Sprintf("non-UTF-8 or binary file not allowed: %q", name)), name)
	}
	trimmed := bytes.TrimLeft(data, " \t\r\n")
	if bytes.HasPrefix(trimmed, []byte("#!")) {
		return apperr.WithPath(apperr.Usage("executable_not_allowed", fmt.Sprintf("shebang script not allowed: %q", name)), name)
	}
	if bytes.HasPrefix(data, []byte("\x7fELF")) || bytes.HasPrefix(data, []byte("MZ")) {
		return apperr.WithPath(apperr.Usage("executable_not_allowed", fmt.Sprintf("executable binary not allowed: %q", name)), name)
	}
	return nil
}

// checkJSON rejects duplicate object keys at every depth and trailing values.
func checkJSON(data []byte) error {
	if !utf8.Valid(data) {
		return fmt.Errorf("invalid UTF-8")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	var value func(int) error
	value = func(depth int) error {
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
				key, err := dec.Token()
				if err != nil {
					return err
				}
				s, ok := key.(string)
				if !ok {
					return fmt.Errorf("expected object key")
				}
				if seen[s] {
					return fmt.Errorf("duplicate key %q", s)
				}
				seen[s] = true
				if err := value(depth + 1); err != nil {
					return err
				}
			}
		case '[':
			for dec.More() {
				if err := value(depth + 1); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("unexpected JSON delimiter")
		}
		_, err = dec.Token()
		return err
	}
	if err := value(0); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return fmt.Errorf("trailing JSON value")
	}
	return nil
}
