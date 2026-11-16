package adapter

import "testing"

func TestAllowlistIsolationBetweenRuntimes(t *testing.T) {
	codex, ok := Profile("codex")
	if !ok {
		t.Fatal("missing codex profile")
	}
	gemini, ok := Profile("gemini")
	if !ok {
		t.Fatal("missing gemini profile")
	}

	// Codex valid paths; rejects Gemini paths.
	if !codex.IsAllowedArtifactPath("project", "engineer", "project", "AGENTS.md", "managed-block") {
		t.Fatal("codex rejected AGENTS.md")
	}
	if !codex.IsAllowedArtifactPath("global", "engineer", "codex-home", "AGENTS.md", "managed-block") {
		t.Fatal("codex rejected global codex-home AGENTS.md")
	}
	if codex.IsAllowedArtifactPath("project", "engineer", "project", "GEMINI.md", "managed-block") {
		t.Fatal("codex allowed GEMINI.md")
	}
	if codex.IsAllowedArtifactPath("project", "engineer", "project", ".gemini/skills/dtkit-gemini-project-plan/SKILL.md", "whole-file") {
		t.Fatal("codex allowed .gemini skill")
	}

	// Gemini valid paths; rejects Codex paths & hooks-json.
	if !gemini.IsAllowedArtifactPath("project", "engineer", "project", "GEMINI.md", "managed-block") {
		t.Fatal("gemini rejected project GEMINI.md")
	}
	if !gemini.IsAllowedArtifactPath("global", "engineer", "gemini-home", "GEMINI.md", "managed-block") {
		t.Fatal("gemini rejected global gemini-home GEMINI.md")
	}
	if !gemini.IsAllowedArtifactPath("project", "engineer", "project", ".gemini/skills/dtkit-gemini-project-plan/SKILL.md", "whole-file") {
		t.Fatal("gemini rejected project skill")
	}
	if !gemini.IsAllowedArtifactPath("global", "engineer", "gemini-home", "skills/dtkit-gemini-global-plan/SKILL.md", "whole-file") {
		t.Fatal("gemini rejected global skill")
	}
	if !gemini.IsAllowedArtifactPath("global", "engineer", "user-home", ".dtkit/runtimes/gemini/content/engineer/rules/engineering.md", "whole-file") {
		t.Fatal("gemini rejected global content path")
	}
	if gemini.IsAllowedArtifactPath("project", "engineer", "project", "AGENTS.md", "managed-block") {
		t.Fatal("gemini allowed AGENTS.md")
	}
	if gemini.IsAllowedArtifactPath("project", "engineer", "project", ".agents/skills/dtkit-project-plan/SKILL.md", "whole-file") {
		t.Fatal("gemini allowed .agents skill")
	}
	if gemini.IsAllowedArtifactPath("project", "engineer", "project", ".dtkit/content/engineer/rules/engineering.md", "whole-file") {
		t.Fatal("gemini allowed codex .dtkit/content")
	}
	if gemini.IsAllowedArtifactPath("project", "engineer", "project", ".gemini/settings.json", "hooks-json") {
		t.Fatal("gemini allowed unsupported hooks-json strategy")
	}
}
