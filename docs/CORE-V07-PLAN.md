# Kế Hoạch Triển Khai Phần Lõi DTKit v0.7 (`CORE-V07-PLAN.md`)

> **Mục tiêu:** Nâng cấp Phần Lõi của DTKit theo triết lý **1 Bộ Kit Kỹ Sư Thống Nhất (`engineer` Skill Mesh kiểu AgentKit/ClaudeKit)** kết hợp với **Sức mạnh Thực thi & Cưỡng chế bằng Binary Go (`dtkit scout`, `dtkit guard`, `dtkit mcp`)** — xử lý mượt mọi dự án đơn ngôn ngữ lẫn đa ngôn ngữ (ví dụ: FE TypeScript + BE Java + AI Python + Core Go/C++/Rust) trên cả 5 Agent Runtimes (`codex`, `claude`, `cursor`, `gemini`, `opencode`).

---

## 1. Bối Cảnh Hiện Tại & Nguyên Tắc Bất Di Bất Dịch

1. **Baseline hiện tại:**
   - Nhánh: `main` (đã hoàn tất `P0`–`P6`, `H1`, `D1`, `R1`–`R3`, Mốc A `N00`–`N12`/`A01`–`A04`, Mốc B `B01`–`B08`, Mốc C `C01`–`C06`, release `v0.6.0`).
   - Môi trường kiểm thử trên máy Windows host: `go.exe` native bị chặn bởi Application Control/Device Guard; **luôn chạy kiểm thử qua Docker container `golang:1.27.4`** bằng lệnh:
     ```powershell
     .\scripts\verify-docker.ps1
     ```
   - PowerShell 5.1 trên Windows không hỗ trợ toán tử `&&` hoặc `||`; luôn dùng `; if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE };`.
2. **Không tách vụn thành nhiều Kit theo ngôn ngữ:**
   - Chỉ duy trì **1 bộ kit `engineer` duy nhất** trong `internal/catalog/engineer/`.
   - Đa ngôn ngữ (FE TypeScript + BE Java + AI Python + C++/Rust/Go...) được giải quyết tự nhiên thông qua:
     1. Bước trinh sát **`scout`** (`dtkit scout` + `skill/scout`).
     2. Bộ tài liệu kiến trúc sống của dự án trong `docs/` (`docs/codebase-summary.md`, `docs/system-architecture.md`, `docs/code-standards.md`) do `skill/docs` và `rule/documentation-management` quản lý.
     3. Bộ nhận diện đa thư mục con (`Roots []WorkspaceRoot`) trong `.dtkit/project.json`.
3. **Tôn trọng tuyệt đối các Invariants kiểm thử hiện có:**
   - Mọi thay đổi trong `internal/catalog/engineer/` **phải tuân thủ**:
     - `internal/adapter/engineer_eval_test.go` (`TestEngineerKitEvaluationInvariantsAcrossRuntimesAndScopes`):
       - Dung lượng instruction luôn nạp (`managed-block` / `.mdc`) phải **`<= 8192` bytes** (`8 KiB`) và nhỏ hơn tổng dung lượng các `SKILL.md` (`skillBytes > instructionBytes`).
       - Không được chứa chuỗi bị cấm trong thân `SKILL.md` sau khi render: `"internal/apperr"`, `"AppendBlock"`, `"ReplaceBlock"`, `"RemoveBlock"`, `"AGENTS.md"`, `"Docker"`.
       - Các skill chỉ đọc (`plan`, `review`, `explain`, `ask`, `brainstorm`, `scout`) phải có cụm từ `"Do not"`.
       - `skill/implement` phải có `"Never fabricate"`.
       - `skill/test` phải có `"Never claim tests passed without running them"` và `".dtkit/project.json"`.
       - `skill/debug` phải có `"Never suppress errors"`.
       - Mọi tham chiếu `{{index .ResourcePaths "..."}}` tạo ra đường dẫn chứa `content/engineer/*.md` trong một skill bắt buộc phải nằm trong danh sách `requires` của skill đó (để khi chọn `--preset minimal` hoặc `--preset review-only` không xảy ra lỗi dangling resource!).
     - `internal/installer/selection_test.go`:
       - Preset `"minimal"` phải giữ nguyên tập `ResolvedComponents` là: `["rule/engineering", "skill/ask", "skill/explain", "skill/plan", "template/implementation-plan"]` (6 artifacts khi render).
       - Preset `"review-only"` phải giữ nguyên tập `ResolvedComponents` là: `["rule/engineering", "skill/review", "template/review-report", "workflow/review"]`.
     - Sau khi cập nhật `internal/catalog/engineer/`, bắt buộc chạy lại:
       ```powershell
       docker run --rm -v "${PWD}:/src" -w /src golang:1.27.4 go run ./cmd/dtkit compat snapshot --out docs/verification/last-known-good-compat.json
       ```
       để đồng bộ baseline compatibility snapshot của `C06`.

---

## 2. Kiến Trúc Tổng Thể Phần Lõi (`D01` – `D04`)

```
┌────────────────────────────────────────────────────────────────────────────┐
│        1 BỘ KIT THỐNG NHẤT: `engineer@0.5.0` (Chạy trên cả 5 Runtimes)     │
│                                                                            │
│  [brainstorm] ──► [scout] ──► [plan] ──► [implement/cook] ──► [test/debug] │
│                      │           │               │                 │       │
│                      ▼           ▼               ▼                 ▼       │
│              `docs/codebase-summary.md`   `plans/*.md`     [review] ──► [docs]
│              `docs/system-architecture.md`                                 │
│              `docs/code-standards.md`                                      │
└──────────────────────▲───────────────────────────▲─────────────────▲───────┘
                       │                           │                 │
        ┌──────────────┴───────────────────────────┴─────────────────┴───┐
        │             4 MODULE LÕI TRONG BINARY GO CỦA DTKIT             │
        ├────────────────────────────────────────────────────────────────┤
        │ D01: `dtkit scout` (`internal/scout` + nâng cấp `project`)     │
        │      Quét nhanh repo đa ngôn ngữ (TS, Java, Python, Go, C++,   │
        │      Rust, C#), lập cây thư mục + thống kê module + đóng gói   │
        │      file liên quan (đã redact secret) phục vụ `scout` & `docs`│
        │ D02: `dtkit guard` (`internal/guard` + `hooks` + `check`)      │
        │      • Phase Firewall (`read-only`, `tdd-red`, `tdd-green`)    │
        │      • Universal Test-Tampering Guard (chặn skip/xóa test)     │
        │      • Verification Receipt SHA-256 & Anti-Retry-Loop (>=3)    │
        │ D03: `engineer@0.5.0` Skill Mesh (`internal/catalog/engineer`) │
        │      Bổ sung `brainstorm`, `scout`, `docs`, `documentation-    │
        │      management`, `bootstrap-docs` và làm sâu toàn bộ skills   │
        │ D04: `dtkit mcp serve` (`internal/mcp`)                        │
        │      STDIO JSON-RPC 2.0 MCP Server để Agent gọi trực tiếp      │
        │      `dtkit_scout`, `dtkit_guard_status`, `dtkit_guard_verify` │
        └────────────────────────────────────────────────────────────────┘
```

---

## 3. Đặc Tả Chi Tiết Từng Hạng Mục Triển Khai

### D01 — Bộ Trinh Sát Đa Ngôn Ngữ & Tóm Tắt Codebase (`internal/scout` + Nâng Cấp `internal/project` + CLI `dtkit scout`)

#### 3.1. Nâng cấp `internal/project/inspect.go` (Nhận diện đa ngôn ngữ cho Monorepo / Polyglot Repo)
Hiện tại `inspect.go` mới nhận diện `go.mod`, `package.json`, `pyproject.toml`/`requirements.txt`, `Cargo.toml`, `Makefile`. Cần bổ sung nhận diện **chỉ đọc (strictly read-only)** thêm các hệ sinh thái phổ biến trong các thư mục gốc và thư mục con (`frontend/`, `backend/`, `ai/`, `services/*`, `apps/*`, `packages/*`):
- **Java / Kotlin:**
  - `pom.xml`: Tạo `WorkspaceRoot{Kind: "java"}` và gợi ý check `mvn test -B` (`Kind: "test"`).
  - `build.gradle` hoặc `build.gradle.kts`: Tạo `WorkspaceRoot{Kind: "java"}` và gợi ý check `gradle test` (`Kind: "test"`).
- **C / C++:**
  - `CMakeLists.txt`: Tạo `WorkspaceRoot{Kind: "cpp"}` và gợi ý check `ctest --output-on-failure` (`Kind: "test"`).
- **C# / .NET:**
  - `*.sln` hoặc `*.csproj`: Tạo `WorkspaceRoot{Kind: "dotnet"}` và gợi ý check `dotnet test` (`Kind: "test"`).
- Giữ nguyên mọi quy tắc bảo mật của `internal/project/schema.go` (executable chỉ là tên lệnh đơn giản không chứa `/` hay `\`, không chạy script tự động khi inspect).

#### 3.2. Tạo package mới `internal/scout` (`internal/scout/scout.go`, `internal/scout/scout_test.go`)
Cung cấp công cụ trinh sát nhanh repo hoàn toàn bằng Go stdlib (thay thế nhu cầu phải cài Node `repomix` cho các tác vụ khảo sát nhanh và tạo `docs/codebase-summary.md`):
- **Struct dữ liệu:**
  ```go
  type Options struct {
      Root       string
      Query      string   // từ khóa hoặc tên module/tính năng cần tìm (tùy chọn)
      Paths      []string // giới hạn trong các thư mục con cụ thể (tùy chọn)
      MaxFiles   int      // mặc định 50, tối đa 200
      MaxBytes   int      // giới hạn dung lượng trích xuất nội dung (mặc định 32768 bytes = 32 KiB)
      IncludeContent bool // nếu true: đính kèm nội dung đã redact secret của các file khớp nhất
  }

  type ModuleZone struct {
      RelPath   string   `json:"relPath"`
      Languages []string `json:"languages"`
      Manifests []string `json:"manifests"`
      FileCount int      `json:"fileCount"`
  }

  type FileEntry struct {
      RelPath   string `json:"relPath"`
      Language  string `json:"language"`
      SizeBytes int64  `json:"sizeBytes"`
      LineCount int    `json:"lineCount"`
      IsTest    bool   `json:"isTest"`
      Role      string `json:"role,omitempty"` // "entrypoint" | "contract" | "config" | "test" | "source"
      Snippet   string `json:"snippet,omitempty"`
  }

  type Report struct {
      Root          string       `json:"root"`
      TotalFiles    int          `json:"totalFiles"`
      TotalLines    int          `json:"totalLines"`
      LanguageStats map[string]int `json:"languageStats"`
      Zones         []ModuleZone `json:"zones"`
      Contracts     []string     `json:"contracts,omitempty"` // openapi.*, *.proto, *.graphql, schema.sql...
      ExistingDocs  []string     `json:"existingDocs,omitempty"` // docs/codebase-summary.md, docs/system-architecture.md...
      MatchedFiles  []FileEntry  `json:"matchedFiles"`
      Truncated     bool         `json:"truncated,omitempty"`
  }
  ```
- **Quy tắc an toàn bắt buộc của `internal/scout`:**
  - Bỏ qua các thư mục rác/nhạy cảm: `.git`, `.dtkit`, `node_modules`, `dist`, `build`, `target`, `vendor`, `.venv`, `venv`, `__pycache__`, `.next`, `bin`, `obj`.
  - **Không bao giờ đọc hoặc xuất nội dung** của các file `.env`, `.env.*` (trừ `.example`/`.sample`), `*.pem`, `*.key`, `id_rsa*`, file binary (chứa byte `0x00`), hoặc symlink/reparse point trỏ ra ngoài `Root`.
  - Áp dụng hàm che mật khẩu/token (`RedactText`) trên mọi `Snippet` được trích xuất.
  - Hàm `FormatMarkdownSummary(rep Report) string`: Xuất ra nội dung Markdown chuẩn bị sẵn cho `docs/codebase-summary.md` (gồm bảng phân khu `Zones` FE/BE/AI/DB, thống kê ngôn ngữ, danh sách API Contracts, và cây cấu trúc file chính).

#### 3.3. Lệnh CLI `dtkit scout` (`internal/cli/scout.go`)
- Cú pháp:
  ```text
  dtkit scout [--path <dir>] [--query <keyword>] [--zone <subpath>] [--pack] [--max-bytes <n>] [--write-summary] [--json]
  ```
  - Mặc định (chế độ chỉ đọc): Quét repo và in báo cáo trinh sát gọn gàng (hoặc JSON khi có `--json` qua `Envelope.Scout`).
  - `--pack`: Đóng gói kèm nội dung rút gọn (đã che secret, giới hạn theo `--max-bytes`) của các file khớp với `--query` / `--zone`.
  - `--write-summary`: Ghi/cập nhật file `docs/codebase-summary.md` một cách an toàn (kiểm tra symlink containment qua `paths.ResolveTarget`).

---

### D02 — Khiên Bảo Vệ Quy Trình, Chống Sửa Bẩn Test & Biên Lai Xác Thực (`internal/guard` + `internal/hooks` + `internal/check` + CLI `dtkit guard`)

#### 3.4. Tạo package mới `internal/guard` (`internal/guard/guard.go`, `internal/guard/tamper.go`, `internal/guard/receipt.go`, `internal/guard/guard_test.go`)

1. **Quản lý trạng thái Phase Firewall (`.dtkit/local/guard.json`):**
   - Vì `.dtkit/local/` đã bị chặn cứng trong `evaluateGuardEditTarget`, Agent không thể tự ý sửa file này qua tool edit.
   - Cấu trúc `Policy`:
     ```go
     type Mode string
     const (
         ModeUnlocked Mode = "unlocked"  // mặc định khi chưa khóa
         ModeReadOnly Mode = "read-only" // dùng cho brainstorm, scout, plan, review, explain, ask
         ModeTDDRed   Mode = "tdd-red"   // chỉ cho phép tạo/sửa file test
         ModeTDDGreen Mode = "tdd-green" // chỉ cho phép sửa file code, khóa cứng không cho sửa file test
         ModeScoped   Mode = "scoped"    // chỉ cho phép sửa trong AllowGlobs và ngoài DenyGlobs
     )

     type Policy struct {
         SchemaVersion int      `json:"schemaVersion"`
         Mode          Mode     `json:"mode"`
         AllowGlobs    []string `json:"allowGlobs,omitempty"`
         DenyGlobs     []string `json:"denyGlobs,omitempty"`
         ProtectTests  bool     `json:"protectTests,omitempty"`
         UpdatedAt     string   `json:"updatedAt"`
     }
     ```
   - Hàm `EvaluateEdit(repoRoot, relPath string) (allowed bool, reason string)`:
     - Nếu `ModeReadOnly`: chặn mọi file (`"blocked by DTKit guard: repository is locked in read-only mode (complete planning/review before editing)"`).
     - Nếu `ModeTDDRed`: chỉ cho phép file test (`IsTestFile(relPath)` hỗ trợ `*_test.go`, `*.test.*`, `*.spec.*`, `test_*.py`, `*_test.py`, `*Test.java`, `*Tests.java`, `*_test.cpp`, `*_test.cc`, `*Tests.cs`, hoặc nằm trong thư mục `test/`, `tests/`, `__tests__/`); chặn file implementation.
     - Nếu `ModeTDDGreen` hoặc `ProtectTests == true`: chặn mọi thao tác sửa vào `IsTestFile(relPath)` (`"blocked by DTKit guard: test files are protected against modification during implementation"`).
     - Nếu có `AllowGlobs` / `DenyGlobs`: kiểm tra khớp đường dẫn tương đối (`/`-normalized).

2. **Bộ phát hiện sửa bẩn bài test đa ngôn ngữ (`internal/guard/tamper.go`):**
   - Hàm `DetectTestTampering(repoRoot string) ([]TamperFinding, error)`:
     - Sử dụng `git diff HEAD --` trên các file `IsTestFile(relPath)` đã sửa đổi trong working tree.
     - Phát hiện 3 mẫu gian lận trên các dòng thêm/bớt (`+` / `-`) của diff:
       1. **`added_skip`**: Thêm mới lệnh bỏ qua bài test (`t.Skip`, `t.Skipf`, `t.SkipNow`, `.skip(`, `pytest.skip(`, `@pytest.mark.skip`, `unittest.skip`, `@Disabled`, `@Ignore`, `GTEST_SKIP`, `#[ignore]`, `[Ignore`).
       2. **`removed_test_case`**: Số lượng định nghĩa hàm test bị xóa (`-`) nhiều hơn số được thêm (`+`) (`func Test`, `def test_`, `@Test`, `it(`, `test(`, `TEST(`, `TEST_F(`).
       3. **`reduced_assertions`**: Tổng số lệnh kiểm tra (`t.Fatal`, `t.Error`, `assert`, `require.`, `expect(`, `ASSERT_`, `EXPECT_`, `assertEquals`, `assertThat`) trong file giảm đi sau khi sửa.

3. **Biên lai Xác thực chạy Check thật (`.dtkit/local/verify-receipt.json`) & Chống lặp lỗi (`.dtkit/local/check-streak.json`):**
   - Hàm `ComputeWorkspaceStateDigest(repoRoot string) (string, []string, error)`:
     - Băm SHA-256 danh sách file thay đổi + nội dung hiện tại của chúng (dựa trên `check.ChangedFiles` hoặc toàn bộ tracked/untracked source files nếu không có git).
   - Cấu trúc `Receipt`:
     ```go
     type Receipt struct {
         SchemaVersion   int      `json:"schemaVersion"`
         WorkspaceDigest string   `json:"workspaceDigest"`
         PassedChecks    []string `json:"passedChecks"`
         VerifiedAt      string   `json:"verifiedAt"`
     }
     ```
   - Khi `check.Run` kết thúc:
     - Nếu tất cả các check được chạy đều `pass` (`Summary.Pass > 0 && Summary.Fail == 0 && Summary.MissingTool == 0 && Summary.Timeout == 0`): tự động ghi `verify-receipt.json` với `WorkspaceDigest` hiện tại và reset `check-streak.json` về `0`.
     - Nếu có check `fail`: tăng bộ đếm `ConsecutiveFailures` trong `.dtkit/local/check-streak.json` kèm tên các check bị lỗi.
   - Hàm `VerifyWorkspace(repoRoot string) (VerificationStatus, error)`:
     - Kiểm tra: (1) Không có `TamperFinding` nào, và (2) `.dtkit/local/verify-receipt.json` tồn tại và có `WorkspaceDigest` **trùng khớp tuyệt đối** với `WorkspaceDigest` hiện tại của repo!

#### 3.5. Tích hợp vào `internal/hooks/claude_runner.go`
- Trong `evaluateGuardEditTarget(stateRoot, targetFile)`:
  - Sau khi kiểm tra `.env`, `.git`, `.dtkit/local` và symlink containment, gọi thêm `guard.EvaluateEdit(cleanRoot, rel)` — nếu không được phép thì trả chuỗi lý do để Hook `PreToolUse` (Claude) / `BeforeTool` (Gemini) chặn (`deny`) ngay lập tức!
- Trong `runCheckChangedForRuntimeWithNow`:
  - Kiểm tra `guard.DetectTestTampering(stateRoot)` — nếu phát hiện sửa bẩn test, gắn cảnh báo `[TEST TAMPERING WARNING: ...]` vào thông điệp trả về.
  - Nếu `ConsecutiveFailures >= 3` trong `.dtkit/local/check-streak.json`, gắn cảnh báo `[RETRY LOOP DETECTED: checks failed 3+ times consecutively; pause and inspect root cause before editing again]`.

#### 3.6. Lệnh CLI `dtkit guard` (`internal/cli/guard.go`)
- Cú pháp:
  ```text
  dtkit guard status [--path <dir>] [--json]
  dtkit guard lock [--path <dir>] --mode <read-only|tdd-red|tdd-green|scoped> [--allow <glob>...] [--deny <glob>...] [--protect-tests] [--json]
  dtkit guard unlock [--path <dir>] [--json]
  dtkit guard verify [--path <dir>] [--json]
  ```
- Nếu `dtkit guard verify` phát hiện chưa có biên lai khớp với code hiện tại hoặc có `TestTampering`: trả về lỗi `apperr.Conflict("verification_failed", ...)` (exit code `3`).

---

### D03 — Nâng Cấp Toàn Diện Kit `engineer@0.5.0` Theo Mô Hình AgentKit Skill Mesh (`internal/catalog/engineer/`)

#### 3.7. Cấu trúc `internal/catalog/engineer/package.json` (`version: "0.5.0"`)
Nâng từ 14 lên **19 components** (giữ nguyên toàn bộ 14 ID cũ để 100% tương thích ngược với các preset và state cũ, bổ sung 5 components mới):
1. **Rules (2):**
   - `rule/engineering` (`rules/engineering.md`): Quy tắc cốt lõi (chu trình `Brainstorm -> Scout -> Plan -> Implement (Cook) -> Test -> Review -> Docs`, YAGNI/KISS/DRY, tôn trọng phạm vi sửa, kiểm chứng bằng `dtkit check` / `dtkit guard verify`). *(Lưu ý: giữ dung lượng gọn để tổng instruction block < 8 KiB)*.
   - `rule/documentation-management` (`rules/documentation-management.md` — **Mới**): Quy chuẩn đọc và duy trì bộ 3 tài liệu kiến trúc dự án trong `docs/`:
     - `docs/codebase-summary.md` (bản đồ các phân khu/thư mục con và ngôn ngữ trong repo)
     - `docs/system-architecture.md` (kiến trúc đa tầng FE/BE/AI/DB và hợp đồng giao tiếp API/Schema)
     - `docs/code-standards.md` (quy ước đặt tên, cấu trúc, test/lint của từng phân khu trong dự án).
2. **Templates (3 — giữ nguyên ID, làm sâu nội dung):**
   - `template/implementation-plan` (`templates/implementation-plan.md`): Cấu trúc kế hoạch chia theo **Phases** (Phase 1..N, Files to Modify per Zone, Verification Commands, Rollback/Risks).
   - `template/review-report` (`templates/review-report.md`): Báo cáo review kết hợp cả **Code Quality + Cross-Zone Contract + Security Audit Checklist**.
   - `template/completion-report` (`templates/completion-report.md`): Báo cáo hoàn thành bắt buộc ghi rõ: Summary, Changed Files per Zone, Executed Check Commands & Exit Codes, Documentation Sync Status (`docs/`), và Remaining Limitations.
3. **Skills (10 — 7 skill cũ được nâng cấp sâu + 3 skill mới):**
   - `skill/brainstorm` (`skills/brainstorm/SKILL.md.tmpl` — **Mới**, `requires: ["rule/engineering"]`):
     - Chế độ `read-only` (`Do not modify repository files during brainstorming`). Hỏi làm rõ yêu cầu mơ hồ (Socratic questioning), phân tích 2–3 phương án kiến trúc kèm ưu/nhược điểm (trade-offs), chốt phương án trước khi chuyển sang `plan`.
   - `skill/scout` (`skills/scout/SKILL.md.tmpl` — **Mới**, `requires: ["rule/engineering", "rule/documentation-management"]`):
     - Chế độ `read-only` (`Do not modify repository files while scouting`). Đọc `docs/codebase-summary.md` / `docs/system-architecture.md` (nếu có) hoặc dùng `dtkit scout` để định vị nhanh các phân khu (FE, BE, AI, DB) và danh sách file cần chạm tới mà không đọc tràn lan làm tốn token.
   - `skill/plan` (`skills/plan/SKILL.md.tmpl` — **Nâng cấp**, giữ nguyên `requires: ["rule/engineering", "template/implementation-plan"]` để không làm đổi `minimal` preset!):
     - Thiết kế kế hoạch thực thi chia theo từng Phase độc lập dựa trên kết quả khảo sát, liệt kê rõ file cần sửa và lệnh kiểm tra cho từng phân khu theo `template/implementation-plan`.
   - `skill/implement` (`skills/implement/SKILL.md.tmpl` — **Nâng cấp theo cơ chế Cook**, giữ `requires: ["rule/engineering", "template/completion-report"]`):
     - Thực thi tuần tự từng Phase trong bản Plan đã duyệt; sửa tới phân khu ngôn ngữ nào (TS, Java, Python, Go, C++...) thì áp dụng đúng chuẩn tại chỗ của phân khu đó; chạy kiểm tra sau mỗi phase; báo cáo theo `template/completion-report`. Giữ câu `"Never fabricate"`.
   - `skill/debug` (`skills/debug/SKILL.md.tmpl` — **Nâng cấp**, `requires: ["rule/engineering"]`):
     - Truy vết lỗi gốc rễ xuyên tầng (từ giao diện -> API -> worker/DB) dựa trên bằng chứng log/reproduction test. Giữ câu `"Never suppress errors"`.
   - `skill/test` (`skills/test/SKILL.md.tmpl` — **Nâng cấp**, `requires: ["rule/engineering"]`):
     - Viết và chạy test theo đúng runner của từng phân khu (`.dtkit/project.json`, JUnit, Pytest, Vitest/Jest, Go test, CTest...), tuân thủ TDD Red/Green, nghiêm cấm xóa assertion hay thêm `skip` để lách lỗi. Giữ câu `"Never claim tests passed without running them"` và `".dtkit/project.json"`.
   - `skill/review` (`skills/review/SKILL.md.tmpl` — **Nâng cấp tích hợp Security**, giữ `requires: ["rule/engineering", "template/review-report"]` để không làm đổi `review-only` preset!):
     - Kiểm tra toàn diện bản diff: lỗi logic, lệch API contract giữa các service, hiệu năng (N+1, leak), và **bảo mật** (Injection, AuthN/AuthZ, Path Traversal/Symlink, SSRF, Secret leak) theo `template/review-report`.
   - `skill/docs` (`skills/docs/SKILL.md.tmpl` — **Mới**, `requires: ["rule/engineering", "rule/documentation-management"]`):
     - Khởi tạo hoặc cập nhật đồng bộ `docs/codebase-summary.md`, `docs/system-architecture.md`, `docs/code-standards.md` phản ánh đúng kiến trúc hiện tại của repo.
   - `skill/explain` & `skill/ask` (`skills/explain/SKILL.md.tmpl`, `skills/ask/SKILL.md.tmpl` — **Nâng cấp**, `requires: ["rule/engineering"]`).
4. **Workflows (4 — 3 workflow cũ nâng cấp + 1 workflow mới):**
   - `workflow/feature` (`workflows/feature.md`): Chu trình khép kín `brainstorm -> scout -> plan -> implement -> test -> review -> docs` (`requires: ["skill/brainstorm", "skill/scout", "skill/plan", "skill/implement", "skill/test", "skill/review", "skill/docs"]`).
   - `workflow/bugfix` (`workflows/bugfix.md`): Chu trình `scout -> debug -> test -> implement -> review` (`requires: ["skill/scout", "skill/debug", "skill/test", "skill/implement", "skill/review"]`).
   - `workflow/review` (`workflows/review.md`): Giữ nguyên `requires: ["skill/review"]` để bảo toàn `review-only` preset.
   - `workflow/bootstrap-docs` (`workflows/bootstrap-docs.md` — **Mới**, `requires: ["skill/scout", "skill/docs"]`): Quy trình quét dự án (đặc biệt hiệu quả cho repo nhiều ngôn ngữ FE+BE+AI) để xây dựng bộ 3 tài liệu `docs/codebase-summary.md`, `docs/system-architecture.md`, `docs/code-standards.md`.
5. **Presets trong `package.json`:**
   - `"minimal"`: `["skill/ask", "skill/explain", "skill/plan"]` *(giữ nguyên 5 resolved components cho backward compatibility tests)*.
   - `"review-only"`: `["workflow/review"]` *(giữ nguyên 4 resolved components cho backward compatibility tests)*.
   - `"engineering"`: `["workflow/feature", "workflow/bugfix", "workflow/review", "workflow/bootstrap-docs", "skill/ask", "skill/explain"]` *(resolve ra toàn bộ 19 components của kit)*.

#### 3.8. Cập nhật các bài test liên quan tới version/component count của `engineer`
- `internal/catalog/catalog_test.go`: Cập nhật `len(p.Components) != 19` trong `TestEmbeddedEngineer`.
- `internal/adapter/engineer_eval_test.go`: Cập nhật `expectedSkills` gồm đủ 10 skills (`"ask"`, `"brainstorm"`, `"debug"`, `"docs"`, `"explain"`, `"implement"`, `"plan"`, `"review"`, `"scout"`, `"test"`).
- `internal/cli/diff_fork_test.go`: Kiểm tra nếu có assert `InstalledVersion` của bundled `engineer` (`0.4.0` -> `0.5.0`, và bump test version tương ứng `0.5.1`).
- Chạy lại `go run ./cmd/dtkit compat snapshot --out docs/verification/last-known-good-compat.json` để cập nhật snapshot `engineer@0.5.0`.

---

### D04 — Built-in STDIO MCP Server (`internal/mcp` + CLI `dtkit mcp serve`)

#### 3.9. Tạo package `internal/mcp` (`internal/mcp/server.go`, `internal/mcp/server_test.go`)
Triển khai MCP Server chuẩn **JSON-RPC 2.0 qua `stdin`/`stdout`** bằng Go stdlib thuần (0 thư viện ngoài), hỗ trợ newline-delimited JSON-RPC messages theo chuẩn MCP (`2024-11-05`):
- Các methods hỗ trợ:
  - `initialize` -> trả `protocolVersion`, `capabilities: {"tools": {}}`, `serverInfo: {"name": "dtkit", "version": buildinfo.Version}`.
  - `notifications/initialized` -> no-op.
  - `ping` -> trả `{}`.
  - `tools/list` -> trả danh sách 4 tools an toàn:
    1. `dtkit_scout`: Quét nhanh cấu trúc repo/module/files theo `query` hoặc `zone` (gọi `scout.Run`).
    2. `dtkit_project_inspect`: Lấy danh sách `Roots` (FE/BE/AI...) và `Checks` của repo (gọi `project.Inspect`).
    3. `dtkit_guard_status`: Xem trạng thái Firewall hiện tại (`mode`, `allowGlobs`, `protectTests`).
    4. `dtkit_guard_verify`: Kiểm tra biên lai pass test (`verify-receipt.json`) và phát hiện `TestTampering` (gọi `guard.VerifyWorkspace`).
  - `tools/call` -> thực thi tool tương ứng và trả `content: [{"type": "text", "text": "..."}]` (kèm `isError: true` nếu lỗi).
- Lệnh CLI:
  ```text
  dtkit mcp serve [--path <dir>]
  ```
- Cập nhật `internal/cli/cli.go`, `internal/cli/envelope.go`, `internal/cli/completion.go`, `docs/cli.md`, `README.md`, và `docs/progress.md`.

---

## 4. Ma Trận Nghiệm Thu (Acceptance Criteria)

| Kiểm tra | Kết quả bắt buộc |
|---|---|
| `dtkit scout` trên repo đa ngôn ngữ (TS + Java + Python + Go) | Phát hiện đúng tất cả `Zones` (`frontend`, `backend`, `ai`...), đếm đúng số file, tự động bỏ qua `.env`/binary/symlink ngoài vùng, `--write-summary` tạo `docs/codebase-summary.md` hợp lệ |
| `dtkit guard lock --mode read-only` + Hook `guard-edit` | Mọi lệnh sửa file qua `PreToolUse` (Claude) và `BeforeTool` (Gemini) đều bị trả `decision: "deny"` |
| `dtkit guard lock --mode tdd-red` vs `tdd-green` | `tdd-red` chỉ cho sửa file test (chặn sửa file code); `tdd-green` chỉ cho sửa file code (chặn sửa file test) |
| AI chèn `t.Skip` / `.skip` / `pytest.skip` hoặc xóa assertion trong file test | `guard.DetectTestTampering` và `dtkit guard verify` bắt được chính xác (`added_skip` / `reduced_assertions`), trả exit code `3` |
| AI sửa file sau khi `dtkit check` đã pass | `dtkit guard verify` phát hiện `workspaceDigest` thay đổi so với `.dtkit/local/verify-receipt.json` và trả exit code `3` (`UNVERIFIED`) |
| `check-changed` fail 3 lần liên tiếp | Hook trả kèm cảnh báo `[RETRY LOOP DETECTED]` |
| `engineer@0.5.0` render trên cả 5 runtimes × 2 scopes | Pass toàn bộ `TestEngineerKitEvaluationInvariantsAcrossRuntimesAndScopes`, `TestBundledEngineerPresetsAndPartialRendering`, và `dtkit compat diff --baseline docs/verification/last-known-good-compat.json` |
| `dtkit mcp serve` | Trả lời đúng `initialize`, `tools/list`, và `tools/call` cho cả 4 tools qua `stdin`/`stdout` |
| Full Verification Suite | `.\scripts\verify-docker.ps1` exit `0` (100% packages pass `go test -race ./...`, `go vet ./...`, `gofmt -l`, `go build ./cmd/dtkit`) |

---

## 5. Prompt Giao Việc Cho AI (Copy Nguyên Văn Để Chạy)

Bạn có thể copy đoạn prompt dưới đây và gửi thẳng cho AI để thực thi từ đầu đến cuối:

```text
Bạn hãy đọc kỹ file `docs/CORE-V07-PLAN.md` và triển khai trọn vẹn Phần Lõi DTKit v0.7 (`D01` -> `D04`) theo đúng thứ tự phụ thuộc:

1. Triển khai `D01`:
   - Mở rộng `internal/project/inspect.go` để nhận diện thêm Java/Kotlin (`pom.xml`, `build.gradle`, `build.gradle.kts`), C/C++ (`CMakeLists.txt`), C#/.NET (`*.sln`, `*.csproj`) ở cả thư mục gốc và các thư mục con.
   - Tạo package `internal/scout` (`scout.go`, `scout_test.go`) và lệnh CLI `dtkit scout [--path <dir>] [--query <kw>] [--zone <subpath>] [--pack] [--max-bytes <n>] [--write-summary] [--json]` để quét cấu trúc repo đa ngôn ngữ, che secret, và sinh `docs/codebase-summary.md`.
2. Triển khai `D02`:
   - Tạo package `internal/guard` (`guard.go`, `tamper.go`, `receipt.go`, `guard_test.go`) quản lý `.dtkit/local/guard.json` (`unlocked`, `read-only`, `tdd-red`, `tdd-green`, `scoped`), phát hiện sửa bẩn bài test đa ngôn ngữ (`DetectTestTampering`), biên lai xác thực `.dtkit/local/verify-receipt.json` (`VerifyWorkspace`), và chuỗi fail liên tiếp `.dtkit/local/check-streak.json`.
   - Tích hợp vào `internal/check/runner.go` (tự ghi receipt khi check pass và cập nhật streak khi fail) và `internal/hooks/claude_runner.go` (`RunGuardEditForRuntime` + `RunCheckChangedForRuntime`).
   - Thêm lệnh CLI `dtkit guard <status|lock|unlock|verify> [--path <dir>] [--json]`.
3. Triển khai `D03`:
   - Nâng cấp `internal/catalog/engineer/` lên version `0.5.0` (19 components: thêm `rule/documentation-management`, `skill/brainstorm`, `skill/scout`, `skill/docs`, `workflow/bootstrap-docs`; nâng cấp sâu các skills/workflows/templates hiện có theo luồng `Brainstorm -> Scout -> Plan -> Implement -> Test -> Review -> Docs`).
   - Đảm bảo giữ nguyên `ResolvedComponents` của preset `minimal` và `review-only`, tuân thủ mọi invariant trong `internal/adapter/engineer_eval_test.go`, cập nhật các test liên quan và chạy lại `dtkit compat snapshot --out docs/verification/last-known-good-compat.json`.
4. Triển khai `D04`:
   - Tạo package `internal/mcp` (`server.go`, `server_test.go`) và lệnh CLI `dtkit mcp serve [--path <dir>]` cung cấp STDIO JSON-RPC 2.0 MCP server cho 4 tools (`dtkit_scout`, `dtkit_project_inspect`, `dtkit_guard_status`, `dtkit_guard_verify`).
   - Cập nhật `internal/cli/{cli,completion,envelope}.go`, `README.md`, `docs/cli.md`, `docs/progress.md`.
5. Kiểm증:
   - Chạy `gofmt` và `.\scripts\verify-docker.ps1` trong Docker `golang:1.27.4` cho tới khi tất cả các package đều pass 100% với `-race`.
```
