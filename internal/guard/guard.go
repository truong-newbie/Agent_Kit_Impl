// Package guard provides phase-based edit firewalls, polyglot test-tampering detection,
// and cryptographic check-verification receipts under .dtkit/local/.
package guard

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

const (
	SchemaVersion1 = 1
	GuardRelPath   = ".dtkit/local/guard.json"
	ReceiptRelPath = ".dtkit/local/verify-receipt.json"
	StreakRelPath  = ".dtkit/local/check-streak.json"
)

type Mode string

const (
	ModeUnlocked Mode = "unlocked"
	ModeReadOnly Mode = "read-only"
	ModeTDDRed   Mode = "tdd-red"
	ModeTDDGreen Mode = "tdd-green"
	ModeScoped   Mode = "scoped"
)

// Policy defines the active edit firewall stored at .dtkit/local/guard.json.
type Policy struct {
	SchemaVersion int      `json:"schemaVersion"`
	Mode          Mode     `json:"mode"`
	AllowGlobs    []string `json:"allowGlobs,omitempty"`
	DenyGlobs     []string `json:"denyGlobs,omitempty"`
	ProtectTests  bool     `json:"protectTests,omitempty"`
	UpdatedAt     string   `json:"updatedAt,omitempty"`
}

// StatusReport summarizes active guard policy and current workspace verification state.
type StatusReport struct {
	Root         string             `json:"root"`
	Policy       Policy             `json:"policy"`
	Verification VerificationStatus `json:"verification"`
	Streak       Streak             `json:"streak"`
}

// ValidMode reports whether m is a supported firewall mode.
func ValidMode(m Mode) bool {
	switch m {
	case ModeUnlocked, ModeReadOnly, ModeTDDRed, ModeTDDGreen, ModeScoped:
		return true
	default:
		return false
	}
}

// LoadPolicy loads .dtkit/local/guard.json or returns unlocked when absent.
func LoadPolicy(repoRoot string) (Policy, error) {
	target, err := paths.ResolveTarget(repoRoot, GuardRelPath)
	if err != nil {
		return Policy{}, err
	}
	raw, err := os.ReadFile(target)
	if errors.Is(err, os.ErrNotExist) {
		return Policy{SchemaVersion: SchemaVersion1, Mode: ModeUnlocked}, nil
	}
	if err != nil {
		return Policy{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var p Policy
	if err := dec.Decode(&p); err != nil {
		return Policy{}, apperr.Usage("invalid_guard_policy", fmt.Sprintf("invalid %s: %v", GuardRelPath, err))
	}
	if !ValidMode(p.Mode) {
		return Policy{}, apperr.Usage("invalid_guard_policy", fmt.Sprintf("unsupported guard mode %q", p.Mode))
	}
	return p, nil
}

// SavePolicy validates and atomically writes .dtkit/local/guard.json.
func SavePolicy(repoRoot string, p Policy, now time.Time) (Policy, error) {
	if !ValidMode(p.Mode) {
		return Policy{}, apperr.Usage("invalid_guard_policy", fmt.Sprintf("unsupported guard mode %q", p.Mode))
	}
	p.SchemaVersion = SchemaVersion1
	if p.Mode == ModeScoped && len(p.AllowGlobs) == 0 && len(p.DenyGlobs) == 0 {
		return Policy{}, apperr.Usage("invalid_guard_policy", "scoped mode requires at least one --allow or --deny glob")
	}
	if p.Mode == ModeTDDGreen {
		p.ProtectTests = true
	}
	p.UpdatedAt = now.UTC().Format(time.RFC3339)

	target, err := paths.ResolveTarget(repoRoot, GuardRelPath)
	if err != nil {
		return Policy{}, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return Policy{}, err
	}
	if info, lErr := os.Lstat(target); lErr == nil && paths.IsReparseOrSymlink(info) {
		return Policy{}, apperr.WithPath(apperr.Conflict("symlink_target", "refusing symlinked guard policy"), GuardRelPath)
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return Policy{}, err
	}
	data = append(data, '\n')
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return Policy{}, err
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return Policy{}, err
	}
	return p, nil
}

// ClearPolicy resets the firewall back to ModeUnlocked (removing guard.json if present).
func ClearPolicy(repoRoot string) error {
	target, err := paths.ResolveTarget(repoRoot, GuardRelPath)
	if err != nil {
		return err
	}
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// EvaluateEdit returns an explicit deny reason if relPath violates the active firewall policy.
func EvaluateEdit(repoRoot, relPath string) (bool, string) {
	p, err := LoadPolicy(repoRoot)
	if err != nil {
		return false, fmt.Sprintf("blocked by DTKit guard: cannot read guard policy (%v)", err)
	}
	norm := path.Clean(strings.ReplaceAll(strings.TrimSpace(relPath), `\`, "/"))
	norm = strings.TrimPrefix(norm, "./")

	switch p.Mode {
	case "", ModeUnlocked:
		return true, ""
	case ModeReadOnly:
		return false, fmt.Sprintf("blocked by DTKit guard: repository is locked in read-only mode (refusing edit to %q)", norm)
	case ModeTDDRed:
		if !IsTestFile(norm) {
			return false, fmt.Sprintf("blocked by DTKit guard (tdd-red): only test files may be modified before implementation (refusing %q)", norm)
		}
		return checkAllowDeny(p, norm)
	case ModeTDDGreen:
		if IsTestFile(norm) {
			return false, fmt.Sprintf("blocked by DTKit guard (tdd-green): test files are protected during implementation (refusing %q)", norm)
		}
		return checkAllowDeny(p, norm)
	case ModeScoped:
		if p.ProtectTests && IsTestFile(norm) {
			return false, fmt.Sprintf("blocked by DTKit guard (scoped): test files are protected (refusing %q)", norm)
		}
		return checkAllowDeny(p, norm)
	default:
		return false, fmt.Sprintf("blocked by DTKit guard: unknown mode %q", p.Mode)
	}
}

func checkAllowDeny(p Policy, norm string) (bool, string) {
	if p.ProtectTests && IsTestFile(norm) {
		return false, fmt.Sprintf("blocked by DTKit guard: test files are protected (refusing %q)", norm)
	}
	for _, d := range p.DenyGlobs {
		if matchGlob(d, norm) {
			return false, fmt.Sprintf("blocked by DTKit guard: %q matches deny rule %q", norm, d)
		}
	}
	if len(p.AllowGlobs) > 0 {
		for _, a := range p.AllowGlobs {
			if matchGlob(a, norm) {
				return true, ""
			}
		}
		return false, fmt.Sprintf("blocked by DTKit guard: %q is outside allowed globs %v", norm, p.AllowGlobs)
	}
	return true, ""
}

// IsTestFile identifies polyglot test files across Go, Python, TS/JS, Java/Kotlin, C/C++, Rust, and C#.
func IsTestFile(relPath string) bool {
	lower := strings.ToLower(path.Clean(strings.ReplaceAll(relPath, `\`, "/")))
	base := path.Base(lower)
	switch {
	case strings.HasSuffix(base, "_test.go"),
		strings.Contains(base, ".test."),
		strings.Contains(base, ".spec."),
		strings.HasPrefix(base, "test_"),
		strings.HasSuffix(base, "_test.py"),
		strings.HasSuffix(base, "test.java"),
		strings.HasSuffix(base, "tests.java"),
		strings.HasSuffix(base, "test.kt"),
		strings.HasSuffix(base, "_test.cpp"),
		strings.HasSuffix(base, "_test.cc"),
		strings.HasSuffix(base, "_test.c"),
		strings.HasSuffix(base, "tests.cs"),
		strings.HasSuffix(base, "test.cs"),
		strings.HasSuffix(base, "_test.rs"):
		return true
	}
	return lower == "tests" || strings.HasPrefix(lower, "tests/") ||
		strings.Contains(lower, "/tests/") ||
		strings.Contains(lower, "/__tests__/") ||
		strings.Contains(lower, "/src/test/")
}

func matchGlob(pattern, relPath string) bool {
	pat := path.Clean(strings.ReplaceAll(strings.TrimSpace(pattern), `\`, "/"))
	pat = strings.TrimPrefix(pat, "./")
	if pat == "" {
		return false
	}
	if pat == "**" {
		return true
	}
	if strings.HasSuffix(pat, "/**") {
		prefix := strings.TrimSuffix(pat, "/**")
		return relPath == prefix || strings.HasPrefix(relPath, prefix+"/")
	}
	if strings.HasPrefix(pat, "**/") {
		sub := strings.TrimPrefix(pat, "**/")
		if ok, _ := path.Match(sub, relPath); ok {
			return true
		}
		parts := strings.Split(relPath, "/")
		for i := range parts {
			if ok, _ := path.Match(sub, strings.Join(parts[i:], "/")); ok {
				return true
			}
		}
		return false
	}
	ok, err := path.Match(pat, relPath)
	return err == nil && ok
}
