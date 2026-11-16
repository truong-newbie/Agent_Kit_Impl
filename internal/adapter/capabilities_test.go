package adapter

import (
	"testing"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
)

func TestCapabilityContractAcrossAllRuntimesAndScopes(t *testing.T) {
	pkg, src, err := catalog.Engineer()
	if err != nil {
		t.Fatal(err)
	}

	// 1. Package strictly requiring BOTH instructions and skills passes on 7 targets and fails pre-write on cursor/global.
	strictPkg := pkg
	strictPkg.RequiresCapabilities = []string{"instructions", "skills"}
	strictPkg.OptionalCapabilities = nil

	for _, rt := range SupportedRuntimes() {
		for _, sc := range []string{"project", "global"} {
			_, rErr := RenderForRuntime(rt, strictPkg, src, sc)
			if rt == "cursor" && sc == "global" {
				if appErr := apperr.AsError(rErr); appErr == nil || appErr.Code != "unsupported_capability" {
					t.Fatalf("expected unsupported_capability for strict instructions on cursor/global, got %v", rErr)
				}
				continue
			}
			if rErr != nil {
				t.Fatalf("expected strict instructions+skills to pass on %s/%s, got %v", rt, sc, rErr)
			}
		}
	}

	// 2. Engineer kit (requires skills, optional instructions) succeeds across all 8 targets and emits fallback warning only on cursor/global.
	for _, rt := range SupportedRuntimes() {
		prof, ok := Profile(rt)
		if !ok {
			t.Fatalf("missing profile %s", rt)
		}
		for _, sc := range []string{"project", "global"} {
			capPlan, cErr := EvaluateCapabilities(prof, sc, &pkg)
			if cErr != nil {
				t.Fatalf("EvaluateCapabilities(%s/%s): %v", rt, sc, cErr)
			}
			arts, rErr := RenderForRuntime(rt, pkg, src, sc)
			if rErr != nil || len(arts) == 0 {
				t.Fatalf("RenderForRuntime(%s/%s): len=%d err=%v", rt, sc, len(arts), rErr)
			}
			if rt == "cursor" && sc == "global" {
				if len(capPlan.Warnings) == 0 || capPlan.Capabilities.Instructions.DTKitSupported {
					t.Fatalf("expected explicit optional fallback warning for cursor/global: %+v", capPlan)
				}
			} else if len(capPlan.Warnings) != 0 {
				t.Fatalf("unexpected fallback warning on %s/%s: %+v", rt, sc, capPlan.Warnings)
			}
		}
	}

	// 3. Package requiring "hooks" fails cleanly pre-write on all runtimes/scopes.
	hookPkg := pkg
	hookPkg.RequiresCapabilities = []string{"skills", "hooks"}
	hookPkg.OptionalCapabilities = []string{"instructions"}
	for _, rt := range SupportedRuntimes() {
		prof, _ := Profile(rt)
		if _, err := EvaluateCapabilities(prof, "project", &hookPkg); apperr.ExitCodeOf(err) != apperr.ExitUsage {
			t.Fatalf("expected hooks requirement rejection for %s, got %v", rt, err)
		}
	}

	// 4. Unknown capability is rejected at catalog validation level and EvaluateCapabilities.
	badPkg := pkg
	badPkg.RequiresCapabilities = []string{"plugins"}
	if err := catalog.Validate(badPkg, src); err == nil {
		t.Fatalf("expected unknown capability 'plugins' rejection")
	}
	prof, _ := Profile("opencode")
	if _, err := EvaluateCapabilities(prof, "project", &badPkg); apperr.ExitCodeOf(err) != apperr.ExitUsage {
		t.Fatalf("expected unknown capability error from EvaluateCapabilities, got %v", err)
	}
}
