package project

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

// InitOptions configures dtkit project init behavior.
type InitOptions struct {
	Path   string
	DryRun bool
}

// InitResult reports the outcome of dtkit project init.
type InitResult struct {
	Root        string  `json:"root"`
	ConfigPath  string  `json:"configPath"`
	Created     bool    `json:"created"`
	DryRun      bool    `json:"dryRun"`
	TrustDigest string  `json:"trustDigest"`
	PreviewJSON string  `json:"previewJson,omitempty"`
	Profile     Profile `json:"profile"`
}

// LoadProfile reads and strictly validates .dtkit/project.json under repoRoot.
func LoadProfile(root string) (Profile, string, bool, error) {
	cfgPath, err := paths.ResolveTarget(root, ProfileRelPath)
	if err != nil {
		return Profile{}, "", false, err
	}
	data, exists, err := readOptionalRegular(root, ProfileRelPath)
	if err != nil {
		return Profile{}, cfgPath, false, err
	}
	if !exists {
		return Profile{}, cfgPath, false, nil
	}
	p, err := ParseAndValidate(data)
	if err != nil {
		return Profile{}, cfgPath, true, err
	}
	return p, cfgPath, true, nil
}

// Init generates and optionally writes .dtkit/project.json without ever overwriting an existing profile.
func Init(ctx context.Context, env paths.Environment, opts InitOptions) (InitResult, error) {
	insp, err := Inspect(ctx, env, opts.Path)
	if err != nil {
		return InitResult{}, err
	}
	if insp.ProfileExists {
		return InitResult{}, apperr.WithHint(
			apperr.WithPath(
				apperr.Conflict("project_profile_exists", fmt.Sprintf("%s already exists; refusing to overwrite existing user profile", ProfileRelPath)),
				ProfileRelPath,
			),
			"edit .dtkit/project.json manually or remove it before re-running dtkit project init",
		)
	}

	formatted, err := json.MarshalIndent(insp.Profile, "", "  ")
	if err != nil {
		return InitResult{}, err
	}
	formatted = append(formatted, '\n')

	res := InitResult{
		Root:        insp.Root,
		ConfigPath:  insp.ConfigPath,
		DryRun:      opts.DryRun,
		TrustDigest: insp.TrustDigest,
		PreviewJSON: string(formatted),
		Profile:     insp.Profile,
	}
	if opts.DryRun {
		return res, nil
	}

	if err := os.MkdirAll(filepath.Dir(insp.ConfigPath), 0755); err != nil {
		return InitResult{}, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(insp.ConfigPath), ".project.json.tmp.*")
	if err != nil {
		return InitResult{}, err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(formatted); err != nil {
		_ = tmp.Close()
		return InitResult{}, err
	}
	if err := tmp.Chmod(0644); err != nil {
		_ = tmp.Close()
		return InitResult{}, err
	}
	if err := tmp.Close(); err != nil {
		return InitResult{}, err
	}
	if err := os.Rename(tmpName, insp.ConfigPath); err != nil {
		return InitResult{}, err
	}
	cleanup = false
	res.Created = true
	return res, nil
}
