# Kế Hoạch Triển Khai Phần Mở Rộng DTKit v0.7 (`EXTENSIONS-V07-PLAN.md`)

> **Mục tiêu:** Xây dựng tầng Mở Rộng & Tự Động Hoá (`E01` – `E03`) trên nền tảng Phần Lõi (`D01`–`D04`, commit `8baa09c`), giúp DTKit:
> 1. **Hấp thụ mọi Repo Skills/Rules ngoài cộng đồng** (như `obra/superpowers`, `.cursor/rules/*.mdc`, `.claude/commands/*.md`, Anthropic skills...) thành gói DTKit chuẩn chạy trên **cả 5 Agent Runtimes** (`dtkit integrate import`).
> 2. **Kết nối & Cache thông minh cho các công cụ CLI bên ngoài** (`repomix`, `graphify`, `ast-grep`, `semgrep`) theo mã băm SHA-256 của workspace (`dtkit integrate list|tool|run`).
> 3. **Khởi tạo dự án chỉ bằng 1 lệnh duy nhất (`dtkit init`) & Tự động gắn MCP Server (`dtkit mcp status|enable|disable`)** vào cấu hình của từng runtime mà không làm mất cấu hình cá nhân của người dùng.

---

## 1. Bối Cảnh & Ràng Buộc Kỹ Thuật Bất Di Bất Dịch

1. **Baseline hiện tại:**
   - Nhánh: `main` (`8baa09c`, đã hoàn tất `D01`–`D04`: `internal/scout`, `internal/guard`, `engineer@0.5.0`, `internal/mcp` STDIO server; CI 5/5 xanh trên Run `44239605436`).
   - Môi trường kiểm thử trên Windows host: `go.exe` native bị chặn bởi Application Control/Device Guard; **luôn chạy mọi lệnh Go qua Docker `golang:1.27.4`**:
     ```powershell
     .\scripts\verify-docker.ps1
     ```
   - PowerShell 5.1 trên Windows không hỗ trợ `&&` hoặc `||`; luôn dùng `; if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE };`.
2. **Các Invariants hiện có không được phép làm gãy:**
   - `schemas/embed_test.go` kiểm tra `len(entries) == 6` trong thư mục `schemas/`. **Không thêm file mới vào `schemas/`** trừ khi cập nhật `schemas/embed_test.go` tương ứng.
   - `internal/adapter/engineer_eval_test.go`, `internal/installer/selection_test.go`, và `docs/verification/last-known-good-compat.json` giữ nguyên từ `D03`.
   - Mọi chế độ `--dry-run`, `list`, `status` phải **tuyệt đối chỉ đọc (strictly read-only)** — không tạo thư mục, không ghi cache, không sửa file cấu hình.
   - Không bao giờ chạy lệnh ngoài qua shell (`sh -c` hay `cmd /c`); luôn dùng `exec.CommandContext` với mảng `argv` tường minh và kiểm tra `exec.LookPath`.

---

## 2. Kiến Trúc Tổng Thể Phần Mở Rộng (`E01` – `E03`)

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                     PHẦN MỞ RỘNG DTKIT v0.7 (`E01` – `E03`)                  │
├────────────────────────┬──────────────────────────┬──────────────────────────┤
│ E01: Universal Importer│ E02: Companion Bridge    │ E03: One-Command Init    │
│ (`internal/dist`)      │ (`internal/companion`)   │ & Auto-MCP Wiring        │
├────────────────────────┼──────────────────────────┼──────────────────────────┤
│ • `dtkit integrate     │ • `dtkit integrate       │ • `dtkit mcp             │
│   import <dir>`        │   list|tool|run`         │   <serve|status|enable|  │
│ • Tự nhận diện:        │ • Blueprints an toàn cho │   disable>`              │
│   - `skills/*/SKILL.md`│   `repomix`, `graphify`, │ • Tự merge/unmerge MCP   │
│     (Superpowers/Claude│   `ast-grep`, `semgrep`  │   JSON cho Claude,       │
│   - `.cursor/rules/`   │ • Cache kết quả theo     │   Cursor, Gemini,        │
│     (`*.mdc`, `.cursor-│   `WorkspaceDigest` tại  │   OpenCode, Codex        │
│     rules`)            │   `.dtkit/local/         │ • `dtkit init` chạy trọn │
│   - `.claude/commands/`│   companion-cache/`      │   gói: Scout Summary +   │
│   - `AGENTS.md` /      │ • Tự fallback về         │   Project Profile +      │
│     `CLAUDE.md`        │   `dtkit scout` khi máy  │   Install `engineer` +   │
│ • Biên dịch sang gói   │   chưa cài tool ngoài    │   MCP + Hooks + Lockfile │
│   DTKit chuẩn 5 runtime│                          │                          │
└────────────────────────┴──────────────────────────┴──────────────────────────┘
```

---

## 3. Đặc Tả Chi Tiết Từng Hạng Mục Triển Khai

### E01 — Universal Community Repo & Skill Importer (`internal/dist/importer.go`, `internal/dist/importer_test.go`)

#### 3.1. Tạo `internal/dist/importer.go`
Cho phép chuyển đổi bất kỳ thư mục chứa skills/rules/workflows/commands của cộng đồng (ví dụ `obra/superpowers`, bộ `.cursor/rules/*.mdc`, bộ `.claude/commands/*.md`, hoặc repo convention nội bộ) thành một gói DTKit hợp lệ (`package.json` + `rules/` + `skills/*/SKILL.md.tmpl` + `workflows/` + `templates/`):

- **Structs dữ liệu:**
  ```go
  type ImportOptions struct {
      SourceDir       string
      TargetParentDir string
      KitID           string
      Name            string
      Version         string // mặc định "0.1.0" nếu rỗng
      DryRun          bool
      Redact          bool
  }

  type ImportedItem struct {
      Kind        string `json:"kind"`        // "rule" | "skill" | "workflow" | "template"
      ComponentID string `json:"componentId"` // ví dụ "skill/brainstorm", "rule/cursor-style"
      SourceRel   string `json:"sourceRel"`
      TargetRel   string `json:"targetRel"`
  }

  type ImportResult struct {
      KitID          string          `json:"kitId"`
      Name           string          `json:"name"`
      Version        string          `json:"version"`
      OutputDir      string          `json:"outputDir"`
      PackageDigest  string          `json:"packageDigest"`
      DetectedFormat string          `json:"detectedFormat"` // "skills-repo" | "cursor-rules" | "claude-config" | "mixed"
      Items          []ImportedItem  `json:"items"`
      Package        catalog.Package `json:"package"`
      Warnings       []string        `json:"warnings,omitempty"`
  }
  ```

- **Quy tắc quét & chuyển đổi (`ImportKit(opts ImportOptions) (ImportResult, error)`):**
  1. Kiểm tra `opts.KitID` khớp `slugPattern` (`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`, tối đa 32 ký tự); kiểm tra `opts.Version` hợp lệ semver (mặc định `"0.1.0"`).
  2. Quét `opts.SourceDir` an toàn:
     - Từ chối nếu `SourceDir` không tồn tại hoặc là symlink/reparse point.
     - Bỏ qua `.git`, `.dtkit`, `node_modules`, `dist`, `build`, `vendor`, `.venv`, `__pycache__`, `.env*`, `*.pem`, `*.key`, file nhị phân (chứa byte `0x00`), hoặc symlink bên trong cây thư mục.
  3. **Tự động phát hiện và chuẩn hoá các thành phần:**
     - **Skills (`kind: "skill"`):**
       - Nhận diện các file:
         - `skills/<slug>/SKILL.md` hoặc `skills/<slug>/SKILL.md.tmpl` (chuẩn **Superpowers** / Anthropic Skills / DTKit)
         - `.claude/skills/<slug>/SKILL.md`, `.cursor/skills/<slug>/SKILL.md`, `.agents/skills/<slug>/SKILL.md`, `.gemini/skills/<slug>/SKILL.md`, `.opencode/skills/<slug>/SKILL.md`
         - `.claude/commands/<slug>.md` hoặc `commands/<slug>.md` (chuyển mỗi slash command `<slug>.md` thành một skill `skill/<slug>`)
       - Chuẩn hoá nội dung sang `skills/<slug>/SKILL.md.tmpl`:
         - Bóc tách YAML frontmatter (nếu có) để lấy `description` (nếu thiếu thì tạo description mặc định từ tiêu đề hoặc tên `<slug>`).
         - Ghi lại frontmatter chuẩn của DTKit:
           ```markdown
           ---
           name: {{.SkillName}}
           description: <single-line sanitized description>
           ---
           ```
         - Nếu thân bài chưa có các mục chuẩn (`## When to use`, `## When not to use`, `## Steps`, `## Constraints`, `## Examples`), giữ nguyên toàn bộ nội dung gốc của tác giả dưới phần hướng dẫn chính và bổ sung các mục khung tối thiểu ở cuối để luôn vượt qua mọi bài kiểm tra chất lượng của DTKit.
     - **Rules (`kind: "rule"`):**
       - Nhận diện: `rules/*.md`, `.cursor/rules/*.mdc`, `.cursorrules`, `CLAUDE.md`, `AGENTS.md`, `GEMINI.md`.
       - Với file `.mdc` của Cursor: bóc bỏ khối YAML frontmatter (`description`, `globs`, `alwaysApply`) ở đầu file để lấy phần nội dung Markdown sạch đưa vào `rules/<slug>.md`.
       - Nếu nguồn không có bất kỳ file rule nào, tự động tạo `rules/imported-baseline.md` (`rule/imported-baseline`) để làm rule gốc.
     - **Workflows & Templates (`kind: "workflow"` / `"template"`):**
       - Nhận diện `workflows/*.md`, `.claude/workflows/*.md` -> `workflows/<slug>.md`.
       - Nhận diện `templates/*.md` -> `templates/<slug>.md`.
  4. Nếu sau khi quét không tìm thấy bất kỳ `skill` nào:
     - Nếu nguồn chỉ có `rules` (ví dụ repo chỉ chứa `.cursorrules` hoặc `.cursor/rules/*.mdc`), tự động sinh 1 skill cầu nối `skill/apply-rules` (`skills/apply-rules/SKILL.md.tmpl`) tham chiếu tới toàn bộ `rule/*` vừa import để gói thoả mãn `requiresCapabilities: ["skills"]` và cài được trên cả 5 runtimes!
     - Nếu không có cả `skill` lẫn `rule`: trả lỗi `apperr.Usage("empty_import_source", "no recognizable skills, rules, commands, or workflows found in source directory")`.
  5. Kiểm định gói vừa tạo trong bộ nhớ (`fstest.MapFS`) bằng `catalog.Validate` và thử `adapter.RenderForRuntime` cho cả 5 runtimes (`codex`, `gemini`, `opencode`, `cursor`, `claude`) + tính `catalog.ComputePackageDigest`.
  6. Nếu `opts.DryRun == true`: trả về `ImportResult` ngay lập tức (không tạo thư mục hay ghi file nào xuống đĩa).
  7. Nếu `opts.DryRun == false`:
     - Kiểm tra `<TargetParentDir>/<KitID>` chưa tồn tại (nếu đã tồn tại trả `apperr.Conflict("target_exists", ...)`).
     - Ghi toàn bộ `package.json` và các file thành phần vào `<TargetParentDir>/<KitID>`.

---

### E02 — External Companion Tool Bridge & Digest Context Cache (`internal/companion/companion.go`, `internal/companion/companion_test.go`)

#### 3.2. Tạo package `internal/companion`
Cung cấp cầu nối an toàn cho 4 công cụ CLI nổi tiếng (`repomix`, `graphify`, `ast-grep`, `semgrep`) kèm bộ nhớ đệm theo mã băm workspace (`.dtkit/local/companion-cache/`):

- **Danh mục Blueprints định sẵn (`BuiltInSpecs()`):**
  ```go
  type CompanionSpec struct {
      ID          string   `json:"id"`          // "repomix" | "graphify" | "ast-grep" | "semgrep"
      Category    string   `json:"category"`    // "context" | "analyzer"
      Executable  string   `json:"executable"`  // "repomix" | "graphify" | "sg" | "semgrep"
      DefaultArgv []string `json:"defaultArgv"`
      Description string   `json:"description"`
      Fallback    string   `json:"fallback"`    // ví dụ "dtkit scout --pack"
  }
  ```
  1. **`repomix`** (`Category: "context"`, `Executable: "repomix"`, `DefaultArgv: ["--stdout", "--style", "markdown", "--no-security-check"]`, `Fallback: "dtkit scout --pack"`).
     - Khi truyền `--zone <subpath>`: thêm `["--include", "<subpath>/**"]` để chỉ đóng gói đúng phân khu cần thiết, chống tràn token!
  2. **`graphify`** (`Category: "context"`, `Executable: "graphify"`, `DefaultArgv: ["query"]`, `Fallback: "dtkit scout --write-summary"`).
  3. **`ast-grep`** (`Category: "analyzer"`, `Executable: "sg"`, `DefaultArgv: ["scan"]`, `Fallback: "dtkit check"`).
  4. **`semgrep`** (`Category: "analyzer"`, `Executable: "semgrep"`, `DefaultArgv: ["scan", "--error", "--quiet"]`, `Fallback: "dtkit check"`).

- **Các hàm chính trong `internal/companion`:**
  1. `InspectCompanions(lookPathFn func(string) (string, error)) []CompanionStatus`:
     - Kiểm tra từng công cụ trong `BuiltInSpecs()` xem đã được cài trên `PATH` hay chưa (`Available: true|false`, `ResolvedPath`, `Fallback`).
  2. `RegisterProjectCheck(repoRoot, toolID string, dryRun bool) (project.Profile, bool, error)`:
     - Dùng cho `dtkit integrate tool <toolID>`:
     - Nếu `.dtkit/project.json` chưa tồn tại thì khởi tạo qua `project.Inspect`; thêm `CheckSpec` an toàn cho `ast-grep` hoặc `semgrep` (hoặc ghi chú companion cho `repomix`/`graphify`) vào `.dtkit/project.json` mà **không tự động ghi vào `check-trust.json`** (người dùng vẫn phải duyệt qua `dtkit check --yes`).
  3. `RunCompanion(ctx context.Context, repoRoot, toolID, zone string, noCache bool, lookPathFn func(string) (string, error)) (RunResult, error)`:
     - Struct `RunResult`:
       ```go
       type RunResult struct {
           ToolID          string `json:"toolId"`
           Zone            string `json:"zone,omitempty"`
           Status          string `json:"status"` // "ok" | "cached" | "missing_tool" | "failed"
           CacheHit        bool   `json:"cacheHit"`
           WorkspaceDigest string `json:"workspaceDigest"`
           Output          string `json:"output,omitempty"`
           FallbackUsed    string `json:"fallbackUsed,omitempty"`
       }
       ```
     - Tính `digest, _, _ := guard.ComputeWorkspaceStateDigest(repoRoot)`.
     - Khóa cache: SHA-256 của `toolID + "|" + zone + "|" + digest`, lưu tại `.dtkit/local/companion-cache/<key>.json`.
     - Nếu `!noCache` và file cache tồn tại -> trả ngay kết quả từ cache (`Status: "cached"`, `CacheHit: true`).
     - Nếu binary của `toolID` không có trên `PATH` (`lookPathFn` trả lỗi):
       - Không bao giờ báo `ok` giả! Trả `Status: "missing_tool"`, `FallbackUsed: spec.Fallback`, và nếu `spec.Category == "context"` thì tự động kèm tóm tắt từ `scout.Run` để Agent vẫn có dữ liệu làm việc!
     - Nếu binary có trên `PATH`: thực thi với timeout 60s, giới hạn output `64 KiB`, áp dụng `scout.RedactText` lên output, lưu vào `.dtkit/local/companion-cache/<key>.json` (khi exit code == 0), và trả kết quả.

---

### E03 — Auto-MCP Config Wiring & One-Command Project Autopilot (`internal/mcp/config.go`, `internal/cli/init.go`, `internal/cli/mcp.go`, `internal/cli/integrate.go`)

#### 3.3. Tự động cấu hình MCP Server cho 5 Runtimes (`internal/mcp/config.go`, `internal/mcp/config_test.go`)
Thêm các hàm quản lý cấu hình `dtkit mcp serve` theo chuẩn của từng runtime ở phạm vi `project` (bảo toàn mọi MCP server khác của người dùng, kiểm tra symlink containment):
- **Đường dẫn & cấu trúc JSON theo từng runtime:**
  - `claude`: `.mcp.json` -> `{"mcpServers": {"dtkit": {"command": "<exe>", "args": ["mcp", "serve"]}}}`
  - `cursor`: `.cursor/mcp.json` -> `{"mcpServers": {"dtkit": {"command": "<exe>", "args": ["mcp", "serve"]}}}`
  - `gemini`: `.gemini/settings.json` -> `{"mcpServers": {"dtkit": {"command": "<exe>", "args": ["mcp", "serve"]}}}`
  - `opencode`: `opencode.json` -> `{"mcp": {"dtkit": {"type": "local", "command": ["<exe>", "mcp", "serve"], "enabled": true}}}`
  - `codex`: `.codex/mcp.json` -> `{"mcpServers": {"dtkit": {"command": "<exe>", "args": ["mcp", "serve"]}}}`
- **Các hàm trong `internal/mcp/config.go`:**
  - `ConfigRelPath(runtimeID string) (string, error)`
  - `InspectMCPConfig(repoRoot, runtimeID string) (MCPConfigStatus, error)`
  - `EnableMCPConfig(repoRoot, runtimeID, exePath string, dryRun bool) (MCPConfigStatus, error)`
  - `DisableMCPConfig(repoRoot, runtimeID string, dryRun bool) (MCPConfigStatus, error)`:
    - Chỉ xóa đúng key `"dtkit"` trong `mcpServers` (hoặc `mcp`); nếu sau khi xóa mà file `.mcp.json` / `.cursor/mcp.json` / `.codex/mcp.json` chỉ còn `{}` rỗng và không có key nào khác của user thì giữ JSON sạch `{ "mcpServers": {} }`. Tuyệt đối không xóa `settings.json` hay `opencode.json` của user!
- **Mở rộng lệnh CLI `dtkit mcp` (`internal/cli/mcp.go`):**
  ```text
  dtkit mcp serve [--path <dir>]
  dtkit mcp status [--runtime <rt>|--all-runtimes] [--path <dir>] [--json]
  dtkit mcp enable [--runtime <rt>|--all-runtimes] [--path <dir>] [--executable <abs>] [--dry-run] [--yes] [--json]
  dtkit mcp disable [--runtime <rt>|--all-runtimes] [--path <dir>] [--dry-run] [--yes] [--json]
  ```

#### 3.4. Lệnh CLI `dtkit integrate` (`internal/cli/integrate.go`)
```text
dtkit integrate list [--path <dir>] [--json]
dtkit integrate import <source-dir> --as <kit-id> [--dir <out-parent>] [--version <x.y.z>] [--name <title>] [--install] [--runtime <rt>] [--project|--global] [--path <dir>] [--dry-run] [--redact] [--yes] [--json]
dtkit integrate tool <repomix|graphify|ast-grep|semgrep> [--path <dir>] [--dry-run] [--yes] [--json]
dtkit integrate run <repomix|graphify|ast-grep|semgrep> [--path <dir>] [--zone <subpath>] [--no-cache] [--json]
```
- Khi `dtkit integrate import` có cờ `--install`:
  - Sau khi chuyển đổi `source-dir` sang `<out-parent>/<kit-id>` (mặc định `<out-parent>` là `.dtkit/imported` trong project hoặc thư mục chỉ định qua `--dir`), tự động cài đặt gói vừa import vào `--project` (mặc định) hoặc `--global` cho `--runtime` đã chọn thông qua engine giao dịch chuẩn!

#### 3.5. Lệnh CLI `dtkit init` (`internal/cli/init.go`)
Lệnh khởi tạo trọn gói "1 lệnh ăn ngay" cho bất kỳ dự án đơn ngôn ngữ hay đa ngôn ngữ nào:
```text
dtkit init [--path <dir>] [--runtime <codex|gemini|opencode|cursor|claude>|--all-runtimes] [--preset <name>] [--with-mcp] [--with-hooks] [--lock] [--dry-run] [--yes] [--json]
```
- Struct kết quả `InitSummary` (trong `Envelope.InitSummary`):
  ```go
  type InitSummary struct {
      Root              string   `json:"root"`
      Runtimes          []string `json:"runtimes"`
      KitID             string   `json:"kitId"`
      KitVersion        string   `json:"kitVersion"`
      Preset            string   `json:"preset,omitempty"`
      CodebaseSummary   string   `json:"codebaseSummary,omitempty"`
      ProjectProfile    string   `json:"projectProfile,omitempty"`
      MCPConfigured     []string `json:"mcpConfigured,omitempty"`
      HooksEnabled      []string `json:"hooksEnabled,omitempty"`
      LockfilePath      string   `json:"lockfilePath,omitempty"`
  }
  ```
- **Luồng thực thi của `dtkit init`:**
  1. Nếu `--dry-run`: tính toán toàn bộ kết quả xem trước (danh sách `Runtimes`, `CodebaseSummary`, `ProjectProfile`, `MCPConfigured`, `HooksEnabled`, `LockfilePath`) mà **không ghi bất kỳ file nào xuống đĩa**.
  2. Nếu thực thi thật (cần `--yes` khi chạy non-interactive hoặc `--json`):
     - **Bước 1 (Codebase Summary):** Nếu `docs/codebase-summary.md` chưa tồn tại, chạy `scout.Run` + `scout.WriteCodebaseSummary` để tạo bản đồ dự án.
     - **Bước 2 (Project Profile):** Nếu `.dtkit/project.json` chưa tồn tại, chạy `project.Init` để tạo profile đa ngôn ngữ. (Nếu đã tồn tại thì giữ nguyên, không báo lỗi).
     - **Bước 3 (Install `engineer@0.5.0`):** Với mỗi runtime mục tiêu, nếu `engineer` chưa được cài ở `project` scope thì cài `engineer@0.5.0` (với `--preset` nếu có); nếu đã cài rồi thì giữ nguyên.
     - **Bước 4 (`--with-mcp`):** Gọi `mcp.EnableMCPConfig` cho từng runtime mục tiêu.
     - **Bước 5 (`--with-hooks`):** Bật hook mặc định (`session-context` cho `codex`, `check-changed` cho `claude`/`gemini`/`cursor`/`opencode`).
     - **Bước 6 (`--lock`):** Tạo/cập nhật `dtkit.lock` ở thư mục gốc của project.

---

## 4. Ma Trận Nghiệm Thu (Acceptance Criteria)

| Kiểm tra | Kết quả bắt buộc |
|---|---|
| `dtkit integrate import` trên thư mục kiểu **Superpowers / Claude** (`skills/*/SKILL.md` + `.claude/commands/*.md`) | Tự động chuẩn hoá frontmatter `name: {{.SkillName}}`, bổ sung các mục bắt buộc, sinh `package.json` hợp lệ, pass `catalog.Validate` và render được trên cả 5 runtimes |
| `dtkit integrate import` trên thư mục kiểu **Cursor Rules** (`.cursor/rules/*.mdc`) | Bóc tách frontmatter `.mdc`, tạo `rules/*.md` + skill cầu nối `skill/apply-rules`, và cài đặt song song với `engineer@0.5.0` qua `--install` (Multi-Kit v3) |
| `dtkit integrate import --dry-run` | Không tạo thư mục đích và không ghi bất kỳ file nào xuống ổ cứng |
| `dtkit integrate list` & `dtkit integrate run` khi thiếu tool ngoài | Báo đúng `missing_tool`, không báo `ok` giả, tự động trả kèm dữ liệu fallback từ `dtkit scout` |
| `dtkit integrate run` khi có binary & chạy lần 2 trên cùng code | Lần 1 thực thi, redact secret và lưu cache tại `.dtkit/local/companion-cache/`; lần 2 trả ngay từ cache (`cacheHit: true`); khi sửa 1 file trong repo thì cache cũ tự mất hiệu lực |
| `dtkit mcp enable` / `status` / `disable` | Ghi đúng cấu trúc MCP cho `claude` (`.mcp.json`), `cursor` (`.cursor/mcp.json`), `gemini` (`.gemini/settings.json`), `opencode` (`opencode.json`), `codex` (`.codex/mcp.json`) mà vẫn giữ nguyên các `mcpServers` khác của user |
| `dtkit init --runtime claude --with-mcp --with-hooks --lock --yes` | Tạo `docs/codebase-summary.md`, `.dtkit/project.json`, cài `engineer@0.5.0`, cấu hình `.mcp.json`, bật hook `check-changed`, và sinh `dtkit.lock` trong 1 lệnh duy nhất; chạy lại lần 2 idempotent không lỗi |
| Full Verification Suite | `.\scripts\verify-docker.ps1` exit `0` (100% packages pass `go test -race ./...`, `go vet ./...`, `gofmt -l`, `go build ./cmd/dtkit`) |

---

## 5. Prompt Giao Việc Cho AI (Copy Nguyên Văn Để Chạy)

Bạn có thể copy khối prompt dưới đây gửi cho AI để triển khai toàn bộ Phần Mở Rộng (`E01` -> `E03`):

```text
Bạn hãy đọc kỹ file `docs/EXTENSIONS-V07-PLAN.md` và triển khai trọn vẹn Phần Mở Rộng DTKit v0.7 (`E01` -> `E03`) theo đúng thứ tự:

1. Triển khai `E01` (`internal/dist/importer.go` + `internal/dist/importer_test.go`):
   - Xây dựng `ImportKit` chuyển đổi các thư mục cấu trúc cộng đồng (`skills/*/SKILL.md`, `.claude/skills/*/SKILL.md`, `.claude/commands/*.md`, `.cursor/rules/*.mdc`, `.cursorrules`, `CLAUDE.md`, `AGENTS.md`, `workflows/*.md`, `templates/*.md`) thành một gói DTKit hợp lệ vượt qua `catalog.Validate` và render được trên cả 5 runtimes.
   - Đảm bảo `--dry-run` chỉ đọc 100%, chặn symlink escape / file nhạy cảm / binary.
2. Triển khai `E02` (`internal/companion/companion.go` + `internal/companion/companion_test.go`):
   - Cung cấp `BuiltInSpecs` (`repomix`, `graphify`, `ast-grep`, `semgrep`), `InspectCompanions`, `RegisterProjectCheck`, và `RunCompanion` kèm bộ nhớ đệm theo `guard.ComputeWorkspaceStateDigest` tại `.dtkit/local/companion-cache/`, redact secret bằng `scout.RedactText`, và fallback về `scout.Run` khi `missing_tool`.
3. Triển khai `E03`:
   - Tạo `internal/mcp/config.go` (`InspectMCPConfig`, `EnableMCPConfig`, `DisableMCPConfig`) hỗ trợ merge/unmerge an toàn cấu hình MCP cho cả 5 runtimes (`claude`, `cursor`, `gemini`, `opencode`, `codex`) và mở rộng `internal/cli/mcp.go` (`dtkit mcp <serve|status|enable|disable>`).
   - Tạo `internal/cli/integrate.go` (`dtkit integrate <list|import|tool|run>`, hỗ trợ `--install` để cài gói vừa import song song với `engineer` qua Multi-Kit v3).
   - Tạo `internal/cli/init.go` (`dtkit init [--runtime <rt>|--all-runtimes] [--preset <name>] [--with-mcp] [--with-hooks] [--lock] [--dry-run] [--yes] [--json]`) và bài test tích hợp `internal/cli/extensions_v07_test.go`.
   - Cập nhật `internal/cli/{cli,completion,envelope}.go`, `README.md`, `docs/cli.md`, `docs/progress.md`.
4. Kiểm chứng:
   - Chạy `gofmt` và `.\scripts\verify-docker.ps1` trong Docker `golang:1.27.4` cho tới khi 100% packages pass với `-race`.
```
