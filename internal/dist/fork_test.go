package dist

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
)

func TestForkKitLifecycleDuplicateIDsLocalReferencesAndPrivacy(t *testing.T) {
	engPkg, engFS, err := catalog.Engineer()
	if err != nil {
		t.Fatal(err)
	}

	tmpHome := filepath.Join(t.TempDir(), "alice-secret-home")
	forkRoot := filepath.Join(tmpHome, "forks")

	// 1. Default duplicate ID without --same-id fails closed.
	_, dupErr := ForkKit(forkRoot, engPkg, engFS, ForkOptions{TargetID: "engineer"})
	var appErr *apperr.Error
	if !errors.As(dupErr, &appErr) || appErr.Code != "duplicate_kit_id" {
		t.Fatalf("expected duplicate_kit_id conflict, got %v", dupErr)
	}

	// 2. --same-id requires bumped version (> source 0.4.0); equal or lower fails.
	_, unbumpedErr := ForkKit(forkRoot, engPkg, engFS, ForkOptions{
		TargetID:    "engineer",
		Version:     engPkg.Version,
		AllowSameID: true,
	})
	if !errors.As(unbumpedErr, &appErr) || appErr.Code != "version_not_bumped" {
		t.Fatalf("expected version_not_bumped, got %v", unbumpedErr)
	}

	// 3. Independent ID fork succeeds, passes LoadDir + multi-runtime rendering, and keeps local refs clean.
	res, err := ForkKit(forkRoot, engPkg, engFS, ForkOptions{
		TargetID: "acme-engineer",
		Version:  "0.1.0",
	})
	if err != nil {
		t.Fatalf("ForkKit acme-engineer failed: %v", err)
	}
	if res.ID != "acme-engineer" || res.Version != "0.1.0" || res.SourceID != "engineer" {
		t.Fatalf("unexpected fork result: %+v", res)
	}

	loadedPkg, loadedSnap, _, err := catalog.LoadDir(res.Dir)
	if err != nil {
		t.Fatalf("LoadDir on forked kit failed: %v", err)
	}
	arts, err := adapter.RenderForRuntime("codex", loadedPkg, loadedSnap, "project")
	if err != nil {
		t.Fatalf("RenderForRuntime on forked kit failed: %v", err)
	}
	foundLocalRef := false
	for _, a := range arts {
		if strings.Contains(string(a.Content), ".dtkit/content/engineer/") {
			t.Fatalf("forked artifact %s still references upstream kit ID:\n%s", a.Path, string(a.Content))
		}
		if strings.Contains(string(a.Content), ".dtkit/content/acme-engineer/") {
			foundLocalRef = true
		}
	}
	if !foundLocalRef {
		t.Fatalf("expected forked kit templates to resolve local references under .dtkit/content/acme-engineer/")
	}

	// 4. Source privacy: no host home/temp directory path leaks into forked package files.
	if walkErr := filepath.WalkDir(res.Dir, func(p string, d fs.DirEntry, wErr error) error {
		if wErr != nil || d.IsDir() {
			return wErr
		}
		data, rErr := os.ReadFile(p)
		if rErr != nil {
			return rErr
		}
		if strings.Contains(string(data), "alice-secret-home") || strings.Contains(string(data), forkRoot) {
			t.Fatalf("source privacy leak in forked file %s", p)
		}
		return nil
	}); walkErr != nil {
		t.Fatal(walkErr)
	}

	// 5. Forking again into the same directory fails closed with target_exists.
	_, existsErr := ForkKit(forkRoot, engPkg, engFS, ForkOptions{TargetID: "acme-engineer"})
	if !errors.As(existsErr, &appErr) || appErr.Code != "target_exists" {
		t.Fatalf("expected target_exists conflict, got %v", existsErr)
	}

	// 6. --same-id without explicit version auto-bumps patch (0.4.0 -> 0.4.1).
	sameRes, err := ForkKit(forkRoot, engPkg, engFS, ForkOptions{
		TargetID:    "engineer",
		AllowSameID: true,
	})
	if err != nil || sameRes.Version != "0.4.1" {
		t.Fatalf("expected auto-bumped patch 0.4.1, got res=%+v err=%v", sameRes, err)
	}
}
