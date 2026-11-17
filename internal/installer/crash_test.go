package installer

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter/codex"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
)

type crashRecoverEnvelope struct {
	Report   RecoverReport `json:"report"`
	Error    string        `json:"error,omitempty"`
	ExitCode int           `json:"exitCode"`
}

func makeCrashV2Artifacts(t testing.TB, version string) (catalog.Package, fs.FS, []adapter.Artifact) {
	pkg, src, err := catalog.Engineer()
	if err != nil {
		if t != nil {
			t.Fatal(err)
		}
		os.Exit(2)
	}
	artifacts, err := codex.Render(pkg, src, "project")
	if err != nil {
		if t != nil {
			t.Fatal(err)
		}
		os.Exit(2)
	}
	if version != "" {
		pkg.Version = version
		mutated := make([]adapter.Artifact, len(artifacts))
		copy(mutated, artifacts)
		for i := range mutated {
			if mutated[i].Strategy == "managed-block" {
				mutated[i].Content = bytes.Replace(mutated[i].Content, []byte("<!-- dtkit:end "), []byte("<!-- crash-v2 -->\n<!-- dtkit:end "), 1)
			} else if strings.HasSuffix(mutated[i].Path, ".md") {
				mutated[i].Content = append(append([]byte{}, mutated[i].Content...), []byte("\n# crash-v2\n")...)
			}
		}
		artifacts = mutated
	}
	return pkg, src, artifacts
}

func TestMain(m *testing.M) {
	mode := os.Getenv("DTKIT_CRASH_SUBPROCESS")
	if mode == "" {
		os.Exit(m.Run())
	}

	proj := os.Getenv("DTKIT_CRASH_ROOT")
	roots := map[string]string{"project": proj}

	switch mode {
	case "apply":
		applyKind := os.Getenv("DTKIT_CRASH_MODE")
		boundary := os.Getenv("DTKIT_CRASH_BOUNDARY")
		readyFile := os.Getenv("DTKIT_CRASH_READY_FILE")
		signalAndHang := func() error {
			if err := os.WriteFile(readyFile, []byte("ready\n"), 0644); err != nil {
				os.Exit(4)
			}
			select {}
		}
		hooks := &TxHooks{}
		switch boundary {
		case "before_prepared":
			hooks.BeforePrepared = signalAndHang
		case "after_action":
			hooks.AfterAction = func(index int, action TxAction) error {
				if index == 1 {
					return signalAndHang()
				}
				return nil
			}
		case "before_state_commit":
			hooks.BeforeStateCommit = signalAndHang
		case "before_committed":
			hooks.BeforeCommitted = signalAndHang
		default:
			os.Exit(5)
		}

		if applyKind == "install" {
			pkg, src, artifacts := makeCrashV2Artifacts(nil, "")
			_, _ = FreshInstallWithOptions("project", roots, pkg, src, artifacts, time.Now(), InstallOptions{Hooks: hooks})
		} else {
			pkg, src, artifacts := makeCrashV2Artifacts(nil, "0.9.9")
			_, _ = Update("project", roots, pkg, src, artifacts, time.Now(), UpdateOptions{Hooks: hooks})
		}
		os.Exit(6)

	case "recover":
		rep, err := Recover("project", roots, "", false, time.Now())
		env := crashRecoverEnvelope{
			Report:   rep,
			ExitCode: apperr.ExitCodeOf(err),
		}
		if err != nil {
			env.Error = err.Error()
		}
		raw, _ := json.Marshal(env)
		_, _ = os.Stdout.Write(append(raw, '\n'))
		os.Exit(env.ExitCode)
	}
	os.Exit(7)
}

func spawnAndKillAtBoundary(t *testing.T, proj, applyMode, boundary string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	readyFile := filepath.Join(t.TempDir(), "ready")
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(),
		"DTKIT_CRASH_SUBPROCESS=apply",
		"DTKIT_CRASH_ROOT="+proj,
		"DTKIT_CRASH_MODE="+applyMode,
		"DTKIT_CRASH_BOUNDARY="+boundary,
		"DTKIT_CRASH_READY_FILE="+readyFile,
	)
	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start child %s/%s: %v", applyMode, boundary, err)
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	deadline := time.Now().Add(15 * time.Second)
	ready := false
	for time.Now().Before(deadline) {
		if _, statErr := os.Stat(readyFile); statErr == nil {
			ready = true
			break
		}
		select {
		case exitErr := <-waitCh:
			t.Fatalf("child %s/%s exited before boundary: %v\n%s", applyMode, boundary, exitErr, outBuf.String())
		case <-time.After(10 * time.Millisecond):
		}
	}
	if !ready {
		_ = cmd.Process.Kill()
		<-waitCh
		t.Fatalf("child %s/%s timed out reaching boundary %s", applyMode, boundary, boundary)
	}

	// Hard OS termination (SIGKILL / TerminateProcess) while blocked in transaction hook.
	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("Process.Kill failed: %v", err)
	}
	<-waitCh
}

func runRecoverSubprocess(t *testing.T, proj string) (crashRecoverEnvelope, int) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(),
		"DTKIT_CRASH_SUBPROCESS=recover",
		"DTKIT_CRASH_ROOT="+proj,
	)
	out, err := cmd.Output()
	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			t.Fatalf("recover subprocess exec failed: %v", err)
		}
	}
	var env crashRecoverEnvelope
	if jsonErr := json.Unmarshal(bytes.TrimSpace(out), &env); jsonErr != nil {
		t.Fatalf("failed to decode recover output %q (exit=%d): %v", string(out), exitCode, jsonErr)
	}
	return env, exitCode
}

func TestCrashRecoveryRealSubprocessBoundaries(t *testing.T) {
	cases := []struct {
		name        string
		mode        string
		boundary    string
		wantOutcome string
		seedV1      bool
	}{
		{name: "before_prepared_install", mode: "install", boundary: "before_prepared", wantOutcome: phaseRolledBack, seedV1: false},
		{name: "after_action_update", mode: "update", boundary: "after_action", wantOutcome: phaseRolledBack, seedV1: true},
		{name: "before_state_commit_update", mode: "update", boundary: "before_state_commit", wantOutcome: phaseRolledBack, seedV1: true},
		{name: "before_committed_update", mode: "update", boundary: "before_committed", wantOutcome: phaseCommitted, seedV1: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			proj, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			agentsPath := filepath.Join(proj, "AGENTS.md")
			seedUser := "# User Header\nKeep safe.\n"
			if err := os.WriteFile(agentsPath, []byte(seedUser), 0644); err != nil {
				t.Fatal(err)
			}
			roots := map[string]string{"project": proj}
			v1Pkg, v1Src, v1Artifacts := makeCrashV2Artifacts(t, "")
			if tc.seedV1 {
				if _, err := FreshInstall("project", roots, v1Pkg, v1Src, v1Artifacts, time.Now()); err != nil {
					t.Fatal(err)
				}
			}
			beforeAgents, _ := os.ReadFile(agentsPath)

			spawnAndKillAtBoundary(t, proj, tc.mode, tc.boundary)

			// Fresh child process performs recover.
			env, exitCode := runRecoverSubprocess(t, proj)
			if exitCode != 0 || env.Report.Outcome != tc.wantOutcome {
				t.Fatalf("unexpected recover result after %s crash: exit=%d env=%+v", tc.boundary, exitCode, env)
			}

			// Repeat recover in another fresh child process must be idempotent noop.
			env2, exitCode2 := runRecoverSubprocess(t, proj)
			if exitCode2 != 0 || env2.Report.Outcome != "noop" {
				t.Fatalf("expected idempotent noop on second recover, got exit=%d env=%+v", exitCode2, env2)
			}

			if tc.wantOutcome == phaseRolledBack {
				afterAgents, err := os.ReadFile(agentsPath)
				if err != nil || string(afterAgents) != string(beforeAgents) {
					t.Fatalf("rolled back AGENTS.md mismatch:\ngot:  %q\nwant: %q", string(afterAgents), string(beforeAgents))
				}
				if tc.seedV1 {
					st, err := Inspect("project", roots, v1Artifacts)
					if err != nil || !st.Healthy || st.RecoveryNeeded {
						t.Fatalf("expected healthy v1 state after rollback, got %+v err=%v", st, err)
					}
				}
			} else {
				_, _, v2Artifacts := makeCrashV2Artifacts(t, "0.9.9")
				st, err := Inspect("project", roots, v2Artifacts)
				if err != nil || !st.Healthy || st.RecoveryNeeded || st.KitVersion != "0.9.9" {
					t.Fatalf("expected healthy v2 state after forward recovery, got %+v err=%v", st, err)
				}
			}
		})
	}

	t.Run("after_action_crash_preserves_external_user_edit_on_recover", func(t *testing.T) {
		proj, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		roots := map[string]string{"project": proj}
		v1Pkg, v1Src, v1Artifacts := makeCrashV2Artifacts(t, "")
		if _, err := FreshInstall("project", roots, v1Pkg, v1Src, v1Artifacts, time.Now()); err != nil {
			t.Fatal(err)
		}

		spawnAndKillAtBoundary(t, proj, "update", "after_action")

		// Simulate external user edit before recover runs.
		agentsPath := filepath.Join(proj, "AGENTS.md")
		cur, err := os.ReadFile(agentsPath)
		if err != nil {
			t.Fatal(err)
		}
		edited := append(cur, []byte("\n# EXTERNAL_USER_EDIT_PRESERVED\n")...)
		if err := os.WriteFile(agentsPath, edited, 0644); err != nil {
			t.Fatal(err)
		}

		env, exitCode := runRecoverSubprocess(t, proj)
		if exitCode != apperr.ExitConflict {
			t.Fatalf("expected ExitConflict when user edited modified file before recover, got exit=%d env=%+v", exitCode, env)
		}
		after, err := os.ReadFile(agentsPath)
		if err != nil || !strings.Contains(string(after), "EXTERNAL_USER_EDIT_PRESERVED") {
			t.Fatalf("external user edit was lost during recover conflict: %q (%v)", string(after), err)
		}
	})
}
