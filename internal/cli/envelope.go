package cli

import (
	"encoding/json"
	"io"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/buildinfo"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/installer"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/planner"
)

type KitSummary struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Digest  string `json:"digest,omitempty"`
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

type Envelope struct {
	SchemaVersion int                        `json:"schemaVersion"`
	Command       string                     `json:"command"`
	Ok            bool                       `json:"ok"`
	DryRun        bool                       `json:"dryRun,omitempty"`
	Runtime       string                     `json:"runtime,omitempty"`
	Scope         string                     `json:"scope,omitempty"`
	Kit           *KitSummary                `json:"kit,omitempty"`
	Roots         map[string]string          `json:"roots,omitempty"`
	Installed     *bool                      `json:"installed,omitempty"`
	Generation    uint64                     `json:"generation,omitempty"`
	Actions       []ActionView               `json:"actions,omitempty"`
	Summary       *ActionSummary             `json:"summary,omitempty"`
	BuildInfo     *buildinfo.Info            `json:"buildInfo,omitempty"`
	Packages      []PackageSummary           `json:"packages,omitempty"`
	Package       *catalog.Package           `json:"package,omitempty"`
	Checks        []DoctorCheck              `json:"checks,omitempty"`
	Installations []installer.StatusReport   `json:"installations,omitempty"`
	Hooks         []installer.HookStatusInfo `json:"hooks,omitempty"`
	Recovery      *installer.RecoverReport   `json:"recovery,omitempty"`
	Warnings      []string                   `json:"warnings"`
	Errors        []apperr.Error             `json:"errors"`
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
