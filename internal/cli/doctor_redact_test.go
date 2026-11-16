package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

func TestDoctorRedactMasksHomeDirectoriesAndTokens(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fakeHome := filepath.Join(base, "secret-user-home")
	projDir := filepath.Join(fakeHome, "work", "my-repo")
	binPath := filepath.Join(fakeHome, "bin", "codex")
	codexHome := filepath.Join(fakeHome, ".codex")
	for _, d := range []string{projDir, codexHome, filepath.Dir(binPath)} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	var stdout, stderr bytes.Buffer
	deps := Deps{
		Stdout: &stdout,
		Stderr: &stderr,
		Env: paths.Environment{
			Getwd:       func() (string, error) { return projDir, nil },
			UserHomeDir: func() (string, error) { return fakeHome, nil },
			Getenv: func(k string) string {
				switch k {
				case "HOME", "USERPROFILE":
					return fakeHome
				case "CODEX_HOME":
					return codexHome
				}
				return ""
			},
		},
		LookPath: func(name string) (string, error) {
			if name == "codex" {
				return binPath, nil
			}
			return "", nil
		},
	}

	// JSON output with --redact must scrub fakeHome across checks while preserving diagnostic labels.
	if err := runDoctor(context.Background(), []string{"--global", "--redact", "--json"}, deps); err != nil {
		t.Fatalf("doctor --global --redact --json failed: %v\nstdout=%s", err, stdout.String())
	}
	rawJSON := stdout.String()
	if strings.Contains(rawJSON, "secret-user-home") {
		t.Fatalf("home directory leaked in redacted JSON output:\n%s", rawJSON)
	}
	var parsed Envelope
	if err := json.Unmarshal([]byte(rawJSON), &parsed); err != nil {
		t.Fatalf("redacted output invalid JSON: %v", err)
	}
	if parsed.Command != "doctor" || !parsed.Ok {
		t.Fatalf("unexpected envelope metadata: %+v", parsed)
	}
	foundHomePlaceholder := false
	for _, c := range parsed.Checks {
		if strings.Contains(c.Message, "<HOME>") {
			foundHomePlaceholder = true
		}
		if strings.Contains(c.Message, "secret-user-home") || strings.Contains(c.Hint, "secret-user-home") {
			t.Fatalf("secret home leaked in parsed check: %+v", c)
		}
	}
	if !foundHomePlaceholder {
		t.Fatalf("expected <HOME> placeholder in parsed checks: %+v", parsed.Checks)
	}

	// Text output with --redact also scrubs fakeHome.
	stdout.Reset()
	stderr.Reset()
	if err := runDoctor(context.Background(), []string{"--global", "--redact"}, deps); err != nil {
		t.Fatalf("doctor --global --redact failed: %v", err)
	}
	if strings.Contains(stdout.String(), "secret-user-home") || !strings.Contains(stdout.String(), "<HOME>") {
		t.Fatalf("expected redacted text output without home dir:\n%s", stdout.String())
	}

	// Direct secret/URL redaction check across diagnostic text & envelopes.
	sample := "failed fetching https://alice:supersecret@registry.example.com/kit.tgz?token=xyz123 from " +
		filepath.ToSlash(fakeHome) + "/cache with Bearer eyJhbGciOi.payload.sig and api_key=sk_live_999"
	redacted := redactDiagnosticText(sample, collectHomeCandidates(deps))
	for _, forbidden := range []string{"secret-user-home", "supersecret", "xyz123", "eyJhbGciOi", "sk_live_999"} {
		if strings.Contains(redacted, forbidden) {
			t.Fatalf("secret/path %q leaked in redacted text: %s", forbidden, redacted)
		}
	}
	if !strings.Contains(redacted, "<HOME>") || !strings.Contains(redacted, "<REDACTED>") {
		t.Fatalf("expected <HOME> and <REDACTED> placeholders in %q", redacted)
	}
}
