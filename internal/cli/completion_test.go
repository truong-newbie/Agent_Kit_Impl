package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
)

func TestCompletionCommand(t *testing.T) {
	for _, tc := range []struct {
		shell string
		want  string
	}{
		{"bash", "_dtkit_complete"},
		{"zsh", "#compdef dtkit"},
		{"powershell", "Register-ArgumentCompleter"},
	} {
		var out, errBuf bytes.Buffer
		deps := DefaultDeps(nil, &out, &errBuf)
		if err := RunWithDeps(context.Background(), []string{"completion", tc.shell}, deps); err != nil {
			t.Fatalf("completion %s failed: %v", tc.shell, err)
		}
		got := out.String()
		if !strings.Contains(got, tc.want) || !strings.Contains(got, "claude") || !strings.Contains(got, "check") {
			t.Fatalf("completion %s missing expected markers:\n%s", tc.shell, got)
		}
	}

	var out, errBuf bytes.Buffer
	deps := DefaultDeps(nil, &out, &errBuf)
	err := RunWithDeps(context.Background(), []string{"completion", "fish"}, deps)
	if err == nil || apperr.ExitCodeOf(err) != apperr.ExitUsage {
		t.Fatalf("expected ExitUsage for unsupported shell, got %v", err)
	}
}
