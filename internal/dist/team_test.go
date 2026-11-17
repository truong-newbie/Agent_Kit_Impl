package dist

import (
	"strings"
	"testing"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/project"
)

func TestTeamBundleSchemaValidationAndTemplate(t *testing.T) {
	pub, _, err := GenerateEd25519Keypair()
	if err != nil {
		t.Fatal(err)
	}
	bundle := TeamBundle{
		SchemaVersion:  1,
		Name:           "acme-platform",
		DefaultRuntime: "claude",
		Runtimes:       []string{"claude", "codex"},
		Lockfile: Lockfile{
			SchemaVersion: 1,
			Kits: []LockedKit{{
				ID:             "engineer",
				Version:        "0.4.0",
				PackageDigest:  strings.Repeat("a", 64),
				Source:         LockSource{Kind: "bundled"},
				RuntimeTargets: []string{"codex", "claude"},
			}},
		},
		TrustPolicy: &TrustPolicy{
			SchemaVersion:    1,
			RequireSignature: false,
			Keys: []TrustedKey{{
				KeyID:     "k1",
				Publisher: "acme",
				Algorithm: "ed25519",
				PublicKey: pub,
				Status:    "active",
			}},
		},
		ProjectProfile: &project.Profile{
			SchemaVersion: 1,
			Name:          "acme-platform",
			Roots:         []project.WorkspaceRoot{{Path: ".", Stack: "go"}},
			Checks:        []project.CheckSpec{{ID: "go-test", Kind: "test", Executable: "go", Argv: []string{"test", "./..."}, Cwd: ".", TimeoutSec: 120}},
		},
		HookRecommendations: []string{"check-changed", "guard-edit"},
	}

	raw, err := MarshalTeamBundle(bundle)
	if err != nil {
		t.Fatalf("MarshalTeamBundle failed: %v", err)
	}
	parsed, err := ParseAndValidateTeamBundle(raw)
	if err != nil {
		t.Fatalf("ParseAndValidateTeamBundle failed: %v", err)
	}
	if parsed.Name != "acme-platform" || len(parsed.Runtimes) != 2 {
		t.Fatalf("unexpected parsed bundle: %+v", parsed)
	}

	guide := OnboardingGuideMarkdown(parsed)
	if !strings.Contains(guide, "dtkit sync --runtime claude") || !strings.Contains(guide, ".dtkit/local/") || !strings.Contains(guide, "check-changed") {
		t.Fatalf("unexpected onboarding guide:\n%s", guide)
	}

	// Reject unknown fields (e.g. machine-local state or privateKey).
	bad := strings.Replace(string(raw), `"schemaVersion": 1,`, `"schemaVersion": 1, "privateKey": "secret",`, 1)
	if _, err := ParseAndValidateTeamBundle([]byte(bad)); err == nil {
		t.Fatal("expected unknown field privateKey rejected")
	}

	// Reject non-allowlisted hook recommendation.
	badHook := strings.Replace(string(raw), `"check-changed"`, `"arbitrary-shell"`, 1)
	if _, err := ParseAndValidateTeamBundle([]byte(badHook)); err == nil {
		t.Fatal("expected unknown hook recommendation rejected")
	}
}
