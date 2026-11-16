package installer

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/state"
)

const (
	maxJournalBytes  = 4 * 1024 * 1024
	maxRetainedTxs   = 10
	journalSchemaVer = 1
	phasePreparing   = "preparing"
	phasePrepared    = "prepared"
	phaseApplying    = "applying"
	phaseCommitted   = "committed"
	phaseRolledBack  = "rolled_back"
	phaseConflict    = "conflict"
)

var txIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{8,64}$`)
var backupNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,64}$`)

type JournalStateRef struct {
	Existed bool   `json:"existed"`
	Backup  string `json:"backup,omitempty"`
	Digest  string `json:"digest,omitempty"`
}

type JournalDir struct {
	Root string `json:"root"`
	Path string `json:"path"`
}

type JournalAction struct {
	Root         string `json:"root"`
	Path         string `json:"path"`
	Kind         string `json:"kind"`     // create | update | remove
	Strategy     string `json:"strategy"` // whole-file | managed-block
	Existed      bool   `json:"existed"`
	BeforeDigest string `json:"beforeDigest,omitempty"`
	AfterDigest  string `json:"afterDigest,omitempty"`
	AfterMissing bool   `json:"afterMissing,omitempty"`
	Backup       string `json:"backup,omitempty"`
	AfterBackup  string `json:"afterBackup,omitempty"`
	Mode         uint32 `json:"mode"`
	Status       string `json:"status"` // pending | applied | rolled_back | conflict
}

type Journal struct {
	SchemaVersion int             `json:"schemaVersion"`
	ID            string          `json:"id"`
	Operation     string          `json:"operation"` // install | update | remove
	Scope         string          `json:"scope"`
	Runtime       string          `json:"runtime,omitempty"`
	KitID         string          `json:"kitId"`
	CodexHome     string          `json:"codexHome,omitempty"`
	RuntimeHome   string          `json:"runtimeHome,omitempty"`
	Phase         string          `json:"phase"`
	CreatedAt     string          `json:"createdAt"`
	UpdatedAt     string          `json:"updatedAt"`
	PreviousState JournalStateRef `json:"previousState"`
	NextState     JournalStateRef `json:"nextState"`
	Actions       []JournalAction `json:"actions"`
	CreatedDirs   []JournalDir    `json:"createdDirs,omitempty"`
}

func (j Journal) EffectiveRuntime() string {
	return adapter.NormalizeRuntime(j.Runtime)
}

func (j Journal) GlobalHome() string {
	if j.CodexHome != "" {
		return j.CodexHome
	}
	return j.RuntimeHome
}

func (j Journal) IsTerminal() bool {
	return j.Phase == phaseCommitted || j.Phase == phaseRolledBack
}

func validateJournal(j Journal, expectedScope string) error {
	return validateJournalForRuntime(j, expectedScope, "")
}

func validateJournalForRuntime(j Journal, expectedScope, expectedRuntime string) error {
	if j.SchemaVersion != journalSchemaVer {
		return fmt.Errorf("unsupported journal schemaVersion %d", j.SchemaVersion)
	}
	if !txIDPattern.MatchString(j.ID) {
		return fmt.Errorf("invalid transaction id %q", j.ID)
	}
	switch j.Operation {
	case "install", "update", "remove":
	default:
		return fmt.Errorf("invalid journal operation %q", j.Operation)
	}
	if j.Scope != "project" && j.Scope != "global" {
		return fmt.Errorf("invalid journal scope %q", j.Scope)
	}
	if expectedScope != "" && j.Scope != expectedScope {
		return fmt.Errorf("journal scope %q does not match requested %q", j.Scope, expectedScope)
	}
	rt := j.EffectiveRuntime()
	if expectedRuntime != "" && rt != adapter.NormalizeRuntime(expectedRuntime) {
		return fmt.Errorf("journal runtime %q does not match requested %q", rt, expectedRuntime)
	}
	profile, ok := adapter.Profile(rt)
	if !ok {
		return fmt.Errorf("unsupported journal runtime %q", rt)
	}
	if profile.UsesCodexHome {
		if j.Scope == "global" && j.CodexHome == "" {
			return fmt.Errorf("global journal missing codexHome binding")
		}
		if j.Scope == "project" && j.CodexHome != "" {
			return fmt.Errorf("project journal cannot specify codexHome")
		}
	} else if profile.GlobalHomeKey != "" {
		if j.Scope == "global" && j.RuntimeHome == "" {
			return fmt.Errorf("global journal missing runtimeHome binding")
		}
		if j.Scope == "project" && j.RuntimeHome != "" {
			return fmt.Errorf("project journal cannot specify runtimeHome")
		}
	}
	if j.KitID == "" || strings.ContainsAny(j.KitID, "/\\ \t\r\n") {
		return fmt.Errorf("invalid journal kit id %q", j.KitID)
	}
	switch j.Phase {
	case phasePreparing, phasePrepared, phaseApplying, phaseCommitted, phaseRolledBack, phaseConflict:
	default:
		return fmt.Errorf("invalid journal phase %q", j.Phase)
	}
	if err := validateStateRef(j.PreviousState); err != nil {
		return fmt.Errorf("previousState: %w", err)
	}
	if err := validateStateRef(j.NextState); err != nil {
		return fmt.Errorf("nextState: %w", err)
	}
	seen := map[string]bool{}
	for _, a := range j.Actions {
		if !state.IsAllowedRuntimeArtifactPath(rt, j.Scope, j.KitID, a.Root, a.Path, a.Strategy) {
			return fmt.Errorf("journal action path %q outside allowlist", a.Path)
		}
		key := strings.ToLower(a.Root + "\x00" + a.Path)
		if seen[key] {
			return fmt.Errorf("duplicate journal action path %q", a.Path)
		}
		seen[key] = true
		switch a.Kind {
		case "create", "update", "remove":
		default:
			return fmt.Errorf("invalid action kind %q", a.Kind)
		}
		if a.Backup != "" && !validBackupName(a.Backup) {
			return fmt.Errorf("invalid backup name %q", a.Backup)
		}
		if a.AfterBackup != "" && !validBackupName(a.AfterBackup) {
			return fmt.Errorf("invalid afterBackup name %q", a.AfterBackup)
		}
		if a.BeforeDigest != "" && !validSHA256Hex(a.BeforeDigest) {
			return fmt.Errorf("invalid beforeDigest for %q", a.Path)
		}
		if a.AfterDigest != "" && !validSHA256Hex(a.AfterDigest) {
			return fmt.Errorf("invalid afterDigest for %q", a.Path)
		}
		switch a.Status {
		case "pending", "applied", "rolled_back", "conflict":
		default:
			return fmt.Errorf("invalid action status %q", a.Status)
		}
	}
	for _, d := range j.CreatedDirs {
		if !isAllowedCreatedDir(rt, j.Scope, j.KitID, d.Root, d.Path) {
			return fmt.Errorf("journal createdDir %q outside allowlist", d.Path)
		}
	}
	return nil
}

func validateStateRef(ref JournalStateRef) error {
	if ref.Backup != "" && !validBackupName(ref.Backup) {
		return fmt.Errorf("invalid state backup %q", ref.Backup)
	}
	if ref.Digest != "" && !validSHA256Hex(ref.Digest) {
		return fmt.Errorf("invalid state digest")
	}
	return nil
}

func validBackupName(name string) bool {
	return name != "." && name != ".." && !strings.ContainsAny(name, "/\\:") && backupNamePattern.MatchString(name)
}

func validSHA256Hex(v string) bool {
	b, err := hex.DecodeString(v)
	return err == nil && len(b) == 32 && v == strings.ToLower(v)
}

func isAllowedCreatedDir(runtimeName, scope, kitID, root, rel string) bool {
	profile, ok := adapter.Profile(runtimeName)
	if !ok {
		return false
	}
	return profile.IsAllowedCreatedDir(scope, kitID, root, rel)
}

func resolveBackupFile(txDir, name string) (string, error) {
	if !validBackupName(name) {
		return "", fmt.Errorf("invalid backup filename %q", name)
	}
	p := filepath.Join(txDir, name)
	info, err := os.Lstat(p)
	if err != nil {
		return "", err
	}
	if paths.IsReparseOrSymlink(info) || !info.Mode().IsRegular() {
		return "", fmt.Errorf("backup file %q is not a regular file", name)
	}
	if info.Size() > maxArtifactBytes {
		return "", fmt.Errorf("backup file %q exceeds size limit", name)
	}
	return p, nil
}
