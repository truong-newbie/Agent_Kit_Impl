package catalog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"regexp"
	"strings"
	"unicode/utf8"
)

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)
var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

func validID(id string, max int) bool { return len(id) <= max && idPattern.MatchString(id) }

func equalSet(actual, expected []string) bool {
	if len(actual) != len(expected) { return false }
	counts := map[string]int{}
	for _, x := range actual { counts[x]++ }
	for _, x := range expected { if counts[x] != 1 { return false }; delete(counts, x) }
	return len(counts) == 0
}

func validPath(p string) bool {
	if !fs.ValidPath(p) || p == "." || strings.ContainsAny(p, `\:`) || strings.Count(p, "/") >= 12 { return false }
	for _, segment := range strings.Split(p, "/") {
		if strings.HasSuffix(segment, ".") || strings.HasSuffix(segment, " ") { return false }
		stem := strings.ToUpper(strings.SplitN(segment, ".", 2)[0])
		if stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" || regexp.MustCompile(`^(COM|LPT)[1-9]$`).MatchString(stem) { return false }
	}
	return true
}

// checkTree rejects links and oversized package content before reading it.
// It is not a security boundary against a concurrent process modifying a directory.
func checkTree(fsys fs.FS) error {
	count, total := 0, int64(0)
	paths := map[string]string{}
	return fs.WalkDir(fsys, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil { return err }
		if name == "." { return nil }
		if !validPath(name) { return fmt.Errorf("invalid package path %q", name) }
		key := strings.ToLower(name); if prev, ok := paths[key]; ok { return fmt.Errorf("case-insensitive path collision: %q and %q", prev, name) }; paths[key] = name
		if entry.Type()&fs.ModeSymlink != 0 { return fmt.Errorf("symbolic link not allowed: %q", name) }
		if entry.IsDir() { if entry.Name() == ".git" { return fmt.Errorf(".git directory is not allowed") }; return nil }
		info, err := entry.Info(); if err != nil { return err }
		if !info.Mode().IsRegular() { return fmt.Errorf("special file not allowed: %q", name) }
		count++; total += info.Size()
		if count > 512 || info.Size() > 1024*1024 || total > 10*1024*1024 { return fmt.Errorf("package size limit exceeded at %q", name) }
		return nil
	})
}

// checkJSON rejects duplicate object keys at every depth and trailing values.
func checkJSON(data []byte) error {
	if !utf8.Valid(data) { return fmt.Errorf("invalid UTF-8") }
	dec := json.NewDecoder(bytes.NewReader(data))
	var value func(int) error
	value = func(depth int) error {
		if depth > 64 { return fmt.Errorf("JSON nesting limit exceeded") }
		tok, err := dec.Token(); if err != nil { return err }
		delim, ok := tok.(json.Delim); if !ok { return nil }
		switch delim {
		case '{':
			seen := map[string]bool{}
			for dec.More() { key, err := dec.Token(); if err != nil { return err }; s, ok := key.(string); if !ok { return fmt.Errorf("expected object key") }; if seen[s] { return fmt.Errorf("duplicate key %q", s) }; seen[s] = true; if err := value(depth+1); err != nil { return err } }
		case '[':
			for dec.More() { if err := value(depth+1); err != nil { return err } }
		default: return fmt.Errorf("unexpected JSON delimiter")
		}
		_, err = dec.Token(); return err
	}
	if err := value(0); err != nil { return err }
	if _, err := dec.Token(); err != io.EOF { return fmt.Errorf("trailing JSON value") }
	return nil
}
