package installer

import (
	"fmt"
	"path/filepath"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

func runtimeProfile(runtime string) (adapter.RuntimeProfile, error) {
	p, err := adapter.MustProfile(runtime)
	if err != nil {
		return adapter.RuntimeProfile{}, apperr.Usage("unsupported_runtime", err.Error())
	}
	return p, nil
}

func runtimeStateTarget(runtime, scope string, roots map[string]string) (string, error) {
	p, err := runtimeProfile(runtime)
	if err != nil {
		return "", err
	}
	return paths.ResolveTarget(roots[scopeRoot(scope)], p.StateRelPath)
}

func runtimeOperationalTarget(runtime, scope string, roots map[string]string) (string, error) {
	p, err := runtimeProfile(runtime)
	if err != nil {
		return "", err
	}
	return paths.ResolveTarget(roots[scopeRoot(scope)], p.OperationalDir)
}

func checkGlobalHomeBinding(p adapter.RuntimeProfile, scope, recorded string, roots map[string]string) error {
	if scope != "global" || p.GlobalHomeKey == "" {
		return nil
	}
	current := roots[p.GlobalHomeKey]
	if !samePath(recorded, current) {
		return apperr.Conflict("profile_mismatch", fmt.Sprintf("installed %s home %q does not match current %q", p.DisplayName, recorded, current))
	}
	return nil
}

func resolveGlobalHomeFromRoots(p adapter.RuntimeProfile, scope string, roots map[string]string) string {
	if scope != "global" || p.GlobalHomeKey == "" {
		return ""
	}
	return filepath.Clean(roots[p.GlobalHomeKey])
}
