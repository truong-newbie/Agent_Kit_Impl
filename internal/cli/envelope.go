package cli

import (
	"encoding/json"
	"io"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/buildinfo"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/check"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/dist"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/installer"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/planner"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/project"
)

type KitSummary struct {
	ID            string `json:"id"`
	Version       string `json:"version"`
	Digest        string `json:"digest,omitempty"`
	ArchiveSHA256 string `json:"archiveSha256,omitempty"`
}

type PackageSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

type ActionView struct {
	Root       string `json:"root"`
	Kind       string `json:"kind"`
	Path       string `json:"path"`
	ReasonCode string `json:"reasonCode"`
}

type ActionSummary struct {
	Create   int `json:"create"`
	Update   int `json:"update"`
	Remove   int `json:"remove"`
	Noop     int `json:"noop"`
	Conflict int `json:"conflict"`
}

type DoctorCheck struct {
	Name    string `json:"name"`
	Scope   string `json:"scope,omitempty"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

type ExplainView struct {
	KitID              string                 `json:"kitId"`
	SourceKind         string                 `json:"sourceKind"`
	SourceLocation     string                 `json:"sourceLocation,omitempty"`
	InstalledVersion   string                 `json:"installedVersion,omitempty"`
	TargetVersion      string                 `json:"targetVersion,omitempty"`
	InstalledDigest    string                 `json:"installedDigest,omitempty"`
	TargetDigest       string                 `json:"targetDigest,omitempty"`
	Selection          *catalog.SelectionSpec `json:"selection,omitempty"`
	OwnedWholeFiles    int                    `json:"ownedWholeFiles"`
	OwnedManagedBlocks int                    `json:"ownedManagedBlocks"`
	PeerOwnedBlocked   int                    `json:"peerOwnedBlocked,omitempty"`
	DriftFiles         int                    `json:"driftFiles,omitempty"`
}

type SyncKitResult struct {
	KitID         string `json:"kitId"`
	Version       string `json:"version"`
	PackageDigest string `json:"packageDigest"`
	ArchiveSHA256 string `json:"archiveSha256,omitempty"`
	Action        string `json:"action"` // "install" | "update" | "noop"
	Generation    uint64 `json:"generation,omitempty"`
	ProvenanceKey string `json:"provenanceKey,omitempty"`
}

type TeamImportSummary struct {
	BundleName          string   `json:"bundleName,omitempty"`
	WrittenFiles        []string `json:"writtenFiles,omitempty"`
	HookRecommendations []string `json:"hookRecommendations,omitempty"`
	ProjectCheckTrust   string   `json:"projectCheckTrust,omitempty"`
}

type TeamTemplateResult struct {
	WrittenFiles []string `json:"writtenFiles,omitempty"`
	SkippedFiles []string `json:"skippedFiles,omitempty"`
}

type Envelope struct {
	SchemaVersion  int                          `json:"schemaVersion"`
	Command        string                       `json:"command"`
	Ok             bool                         `json:"ok"`
	DryRun         bool                         `json:"dryRun,omitempty"`
	Runtime        string                       `json:"runtime,omitempty"`
	Scope          string                       `json:"scope,omitempty"`
	Capabilities   *adapter.Capabilities        `json:"capabilities,omitempty"`
	Integration    *adapter.IntegrationEvidence `json:"integration,omitempty"`
	Kit            *KitSummary                  `json:"kit,omitempty"`
	Selection      *catalog.SelectionSpec       `json:"selection,omitempty"`
	Roots          map[string]string            `json:"roots,omitempty"`
	Installed      *bool                        `json:"installed,omitempty"`
	Generation     uint64                       `json:"generation,omitempty"`
	Actions        []ActionView                 `json:"actions,omitempty"`
	Summary        *ActionSummary               `json:"summary,omitempty"`
	Explain        *ExplainView                 `json:"explain,omitempty"`
	Diff           *planner.PlanDiff            `json:"diff,omitempty"`
	BuildInfo      *buildinfo.Info              `json:"buildInfo,omitempty"`
	Packages       []PackageSummary             `json:"packages,omitempty"`
	Package        *catalog.Package             `json:"package,omitempty"`
	Checks         []DoctorCheck                `json:"checks,omitempty"`
	Installations  []installer.StatusReport     `json:"installations,omitempty"`
	Hooks          []installer.HookStatusInfo   `json:"hooks,omitempty"`
	Recovery       *installer.RecoverReport     `json:"recovery,omitempty"`
	Pack           *dist.PackResult             `json:"pack,omitempty"`
	Fork           *dist.ForkResult             `json:"fork,omitempty"`
	Lockfile       *dist.Lockfile               `json:"lockfile,omitempty"`
	LockDiff       *dist.LockDiffReport         `json:"lockDiff,omitempty"`
	SyncResults    []SyncKitResult              `json:"syncResults,omitempty"`
	TeamBundle     *dist.TeamBundle             `json:"teamBundle,omitempty"`
	TeamImport     *TeamImportSummary           `json:"teamImport,omitempty"`
	TeamTemplate   *TeamTemplateResult          `json:"teamTemplate,omitempty"`
	CreateKit      *dist.CreateKitResult        `json:"createKit,omitempty"`
	CreateSkill    *dist.CreateSkillResult      `json:"createSkill,omitempty"`
	ProjectInspect *project.InspectResult       `json:"projectInspect,omitempty"`
	ProjectInit    *project.InitResult          `json:"projectInit,omitempty"`
	CheckReport    *check.Report                `json:"checkReport,omitempty"`
	Warnings       []string                     `json:"warnings"`
	Errors         []apperr.Error               `json:"errors"`
}

// JSONReportedError wraps an error that was already serialized to stdout as JSON.
type JSONReportedError struct {
	Err error
}

func (e *JSONReportedError) Error() string { return e.Err.Error() }
func (e *JSONReportedError) Unwrap() error { return e.Err }

func newEnvelope(command string) Envelope {
	return Envelope{
		SchemaVersion: 1,
		Command:       command,
		Ok:            true,
		Warnings:      []string{},
		Errors:        []apperr.Error{},
	}
}

func writeEnvelope(w io.Writer, env Envelope) error {
	if env.Warnings == nil {
		env.Warnings = []string{}
	}
	if env.Errors == nil {
		env.Errors = []apperr.Error{}
	}
	return json.NewEncoder(w).Encode(env)
}

func failJSON(w io.Writer, env Envelope, err error) error {
	env.Ok = false
	appErr := apperr.AsError(err)
	env.Errors = append(env.Errors, *appErr)
	_ = writeEnvelope(w, env)
	return &JSONReportedError{Err: err}
}

func summarizeActions(actions []planner.Action) ([]ActionView, *ActionSummary) {
	views := make([]ActionView, 0, len(actions))
	summary := &ActionSummary{}
	for _, a := range actions {
		views = append(views, ActionView{
			Root:       a.Root,
			Kind:       a.Kind,
			Path:       a.Path,
			ReasonCode: a.Reason,
		})
		switch a.Kind {
		case "create":
			summary.Create++
		case "update":
			summary.Update++
		case "remove":
			summary.Remove++
		case "noop":
			summary.Noop++
		case "conflict":
			summary.Conflict++
		}
	}
	return views, summary
}
