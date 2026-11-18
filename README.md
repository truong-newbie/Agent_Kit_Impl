# DTKit

**DTKit** (`dtkit`) is a single-binary Go CLI for packaging, verifying, and safely installing reusable AI engineering skills, rules, workflows, and quality hooks across multiple coding-agent runtimes from one portable source.

## Why DTKit?

- **Write once, install anywhere:** Target **Codex CLI**, **Claude Code**, **OpenCode**, **Gemini CLI**, and **Cursor** side-by-side without path collisions.
- **Non-destructive by default:** User instructions outside DTKit-managed blocks are preserved byte-for-byte (including UTF-8 BOM and CRLF/LF). Modified files halt changes (`exit 3`) rather than silently overwriting your edits.
- **Crash-safe transactions:** Every install, update, remove, hook change, and multi-kit sync runs under PID/token locks with transaction journals and `dtkit recover`.
- **Reproducible & verifiable teams:** Pin kit digests with `dtkit.lock`, verify Ed25519 publisher signatures, export secret-free onboarding bundles with `dtkit team`, and preview changes via `dtkit diff`.
- **Opt-in quality checks & hooks:** Define repo-neutral project checks in `.dtkit/project.json` (`dtkit check`) and wire runtime hooks/plugins (`dtkit hooks enable`) with explicit trust approval.

---

## Installation

### Option 1: Prebuilt Release Binary (Recommended)

Checksum-verified installers download pinned archives (`windows_amd64.zip`, `linux_amd64.tar.gz`, `darwin_arm64.tar.gz`) and verify `SHA256SUMS` before replacing any binary.

**Linux / macOS (`~/.local/bin`):**

```bash
bash scripts/install.sh --version 0.6.0 --bin-dir "$HOME/.local/bin"
export PATH="$HOME/.local/bin:$PATH"
dtkit version --json
```

**Windows PowerShell (`%LOCALAPPDATA%\Programs\dtkit\bin`):**

```powershell
.\scripts\install.ps1 -Version "0.6.0"
dtkit version --json
```

> See [docs/release.md](docs/release.md#terminal-install--checksum-verification-pinned-version-no-admin) for standalone manual verification commands and upgrade notes.

### Option 2: Build from Source

Requires Go `1.27.4+` (or Docker Desktop via `.\scripts\verify-docker.ps1`):

```bash
go install github.com/truong-newbie/Agent_Kit_Impl/cmd/dtkit@v0.6.0
```

---

## Quick Start

```bash
# 1. Check runtime readiness on your machine
dtkit doctor
dtkit doctor --runtime claude

# 2. Browse and inspect the bundled engineering kit
dtkit list
dtkit info engineer

# 3. Preview, then install for your project (full kit or a preset: minimal | review-only | engineering)
dtkit install engineer --project --runtime claude --diff
dtkit install engineer --project --runtime claude --preset minimal --yes

# 4. Inspect active installations across all 5 runtimes
dtkit status --all-runtimes

# 5. Scout polyglot repos, run trusted quality checks & enforce phase firewalls
dtkit scout --write-summary
dtkit project inspect
dtkit project init --yes
dtkit check --all --yes
dtkit guard lock --mode read-only
dtkit guard verify
dtkit hooks enable check-changed --runtime claude --project --dry-run

# 6. Pin & share across your team
dtkit lock generate
dtkit team export --out team-bundle.json
```

Enable shell tab-completion (`bash`, `zsh`, or `powershell`):

```bash
dtkit completion bash   # or zsh / powershell
```

---

## Supported Runtimes & Current Status

DTKit installs and manages files across all five runtimes below. Status reflects real verified evidence rather than automated unit tests alone (see [docs/compatibility.md](docs/compatibility.md)):

| Runtime | Managed Files (Project Scope) | Quality Hooks / Plugin | Current Status |
|---|---|---|---|
| **Codex CLI** (`codex`) | `AGENTS.md` block, `.agents/skills/` | `.codex/hooks.json` (`SessionStart` `session-context`) | **Partial** (discovery verified on `0.169.0`; live hook firing & sandboxed commands unverified) |
| **OpenCode** (`opencode`) | Namespaced `AGENTS.md` block, `.opencode/skills/` | `.opencode/plugins/dtkit-quality.js` (`tool.execute.after`) | **Partial** (14-skill discovery verified on `1.19.5`; live LLM & plugin execution unverified without credentials) |
| **Claude Code** (`claude`) | `CLAUDE.md` block, `.claude/skills/` | `.claude/settings.json` (`PostToolUse` `check-changed`, `PreToolUse` `guard-edit`) | **Partial** (automated hook merge & lifecycle tested; live session/A-B eval blocked on credentials) |
| **Gemini CLI** (`gemini`) | `GEMINI.md` block, `.gemini/skills/` | `.gemini/settings.json` (`AfterTool`, `BeforeTool`) | **Experimental** (`gemini` CLI absent on verification host; offline tests pass) |
| **Cursor** (`cursor`) | `.cursor/rules/dtkit-*.mdc`, `.cursor/skills/` | `.cursor/hooks.json` (`afterFileEdit`, `version: 1`) | **Experimental** (`cursor` CLI/IDE absent on verification host; global User Rules remain UI-only) |

Add `.dtkit/local/` to your `.gitignore` so machine-local locks, transaction journals, and trust approvals are not committed.

---

## Command Reference

| Area | Commands | Purpose |
|---|---|---|
| **Install & Manage** | `list`, `info`, `install`, `update`, `remove`, `status`, `diff`, `recover` | Preview, install, co-exist multiple kits, update, inspect all runtimes, or recover interrupted writes |
| **Author & Customize** | `create kit\|skill`, `validate`, `pack`, `fork` | Scaffold kits/skills, validate manifests, build deterministic `.tar.gz` + `.sha256`, or fork verified kits |
| **Lockfile & Teams** | `lock <generate\|verify\|diff\|sign\|keygen>`, `sync`, `team <export\|import\|template>` | Reproducible installs, Ed25519 provenance verification, and secret-free team onboarding |
| **Quality, Guard & Hooks** | `scout`, `project inspect\|init`, `check`, `guard <status\|lock\|unlock\|verify>`, `hooks <list\|status\|enable\|disable>`, `mcp serve` | Polyglot repo scout, `.dtkit/project.json` checks, phase/TDD edit firewall, SHA-256 verification receipts, opt-in runtime hooks, built-in STDIO MCP server |
| **Diagnostics & Maintainers** | `doctor [--redact]`, `compat <snapshot\|diff>`, `completion`, `version` | Environment diagnostics, cross-runtime compatibility snapshots, shell completion |

Most commands support `--dry-run`, `--diff`, `--yes`, and `--json`. See [docs/cli.md](docs/cli.md) for every flag, schema rule, and exit code (`0` success, `1` error, `2` usage/validation, `3` conflict/trust block, `4` target not found, `130` cancelled).

---

## Documentation

- **User Guides:** [CLI Reference](docs/cli.md) · [Runtime Compatibility Matrix](docs/compatibility.md) · [Version & Deprecation Policy](docs/compatibility-policy.md) · [Release & Upgrade Guide](docs/release.md) · [Support Policy](docs/support.md)
- **Architecture & Development:** [Development Guide](docs/development.md) · [Evaluation Harness](docs/evaluation/README.md) · [Progress & Changelog](docs/progress.md)
- **License:** [MIT](LICENSE)
