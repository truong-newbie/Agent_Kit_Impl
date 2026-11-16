# DTKit — Kế hoạch triển khai sản phẩm

> Phiên bản kế hoạch: 1.0 — ngày 2026-11-16.
> Trạng thái: tài liệu bàn giao để triển khai; chưa có mã nguồn sản phẩm.
> Ngôn ngữ sản phẩm: Go. CLI: `dtkit`. Runtime đầu tiên: Codex CLI.
> MVP bắt buộc hỗ trợ cả project-level và global-level trên Windows.

Điểm đọc nhanh:

- [Phạm vi phiên bản](#4-phạm-vi-các-phiên-bản).
- [Kiến trúc và cấu trúc repository](#6-kiến-trúc-go).
- [Contract CLI](#11-cli-command-contract).
- [Kiểm thử và nghiệm thu](#17-chiến-lược-kiểm-thử).
- [Milestone triển khai](#20-milestone-và-backlog-thực-thi).
- [Prompt bàn giao cho AI](#24-hướng-dẫn-bàn-giao-cho-ai-triển-khai).

## 1. Mục đích và cách sử dụng tài liệu

Tài liệu này là kế hoạch thực thi cho AI coding agent hoặc lập trình viên xây dựng DTKit từ thư mục hiện tại. Đọc toàn bộ trước khi viết code, sau đó thực hiện theo các milestone tại mục 20. Mỗi milestone có đầu ra, công việc và tiêu chí kiểm chứng; không đánh dấu hoàn thành chỉ vì đã tạo file.

Nguồn ý tưởng:

- [Đặc tả sản phẩm ban đầu](ai-agent-toolkit-spec.md).
- [AI Engineering Kit Global](AI-Engineering-Kit-Global.md).

Các quyết định mới của chủ dự án được ghi tại mục 2 và thay thế những lựa chọn khác trong hai tài liệu trên. Cụ thể: dùng Go, tên `dtkit`, Codex trước Claude Code, hỗ trợ global ngay trong MVP, mở rộng Gemini CLI/OpenCode/Cursor về sau.

Đây là tài liệu thiết kế DTKit, không phải instruction để áp dụng trực tiếp toàn bộ vào một dự án của người dùng. Những đoạn mô tả hành vi Codex có nguồn chính thức ở mục 26; những schema, lệnh và chính sách còn lại là quyết định thiết kế của DTKit.

Quy tắc cho AI triển khai:

1. Giữ nguyên hai tài liệu nguồn; nếu phát hiện mâu thuẫn, ghi quyết định vào `docs/decisions.md`.
2. Dùng kế hoạch này làm phạm vi công việc; không hỏi lại các lựa chọn đã được chốt.
3. Hoàn thành và kiểm chứng từng milestone rồi tiếp tục milestone kế tiếp khi đủ điều kiện.
4. Có thể tự quyết các chi tiết nội bộ phù hợp contract; ghi lý do khi thay đổi contract công khai.
5. Chỉ cần hỏi chủ dự án khi có quyết định về quyền sở hữu, phát hành công khai, license hoặc mở rộng đáng kể phạm vi.
6. Phân biệt `implemented`, `automated-tested`, `runtime-verified`, `released`. Bốn trạng thái này không thay thế nhau.
7. Không chạy các lệnh cài global vào home thật trong test tự động. Dùng filesystem và environment cô lập.

## 2. Các quyết định đã chốt

| Hạng mục | Quyết định |
|---|---|
| Tên sản phẩm | DTKit |
| Tên executable | `dtkit`; Windows là `dtkit.exe` |
| Ngôn ngữ | Go |
| Runtime v0.1 | Codex CLI |
| Phạm vi cài kit | Project và global, đều thuộc v0.1 |
| Hệ điều hành ưu tiên | Windows native; thiết kế portable, có CI Linux/macOS |
| Kit đầu tiên | `engineer` |
| Catalog v0.1 | Bundle theo binary; nhận package local để phát triển/kiểm thử |
| Tài khoản/backend | Không cần trong v0.1 |
| Phân phối đầu tiên | Git repository + release binaries + tài liệu terminal |
| Runtime tiếp theo | Gemini CLI, OpenCode, Cursor; nghiên cứu và kiểm thử riêng |
| Hook thực thi | Milestone riêng sau MVP core, mặc định opt-in |
| Multi-agent | Chưa xây orchestration; mở rộng sau khi core ổn định |
| Nội dung | Tự viết; không sao chép nội dung có quyền sử dụng chưa rõ |
| Hành vi cập nhật | Bảo toàn nội dung người dùng; phát hiện conflict trước khi ghi |

Chủ dự án đã giao quyền lựa chọn các chi tiết kỹ thuật còn lại. Các lựa chọn trong tài liệu là baseline có thể triển khai ngay.

## 3. Kết quả người dùng cần nhận được

DTKit đóng gói phương pháp làm việc cho AI thành kit có phiên bản. Người dùng tải một executable, rồi cài kit vào đúng vị trí runtime đọc được.

Hai luồng cần được làm đầy đủ:

1. **Phân phối chương trình:** lấy `dtkit.exe` từ release hoặc build từ source, đưa vào một thư mục trên `PATH`, chạy `dtkit --version`.
2. **Cài nội dung:** chạy `dtkit install engineer --runtime codex --project` hoặc `--global`, sau đó sử dụng các skills trong Codex.

Trải nghiệm mục tiêu sau khi bản build tồn tại:

```powershell
dtkit --version
dtkit doctor --runtime codex
dtkit list
dtkit info engineer

# Cài cho dự án hiện tại; xem trước rồi mới áp dụng.
dtkit install engineer --runtime codex --project --dry-run
dtkit install engineer --runtime codex --project
dtkit status --project

# Cài dùng chung trong tài khoản hệ điều hành hiện tại.
dtkit install engineer --runtime codex --global --dry-run
dtkit install engineer --runtime codex --global
dtkit status --global

# Dùng catalog đóng gói trong phiên bản dtkit đang chạy.
dtkit update engineer --project --dry-run
dtkit remove engineer --project --dry-run
```

Ví dụ gọi skill trong Codex sau khi cài project:

```text
$dtkit-project-plan Phân tích yêu cầu thêm chức năng tìm kiếm nhân viên.
Chỉ tạo kế hoạch, chưa sửa code.

$dtkit-project-implement Thực hiện kế hoạch đã thống nhất trong docs/plans/search.md.

$dtkit-project-review Review thay đổi hiện tại, ưu tiên lỗi logic và regression.
```

Các tên skill trên là lựa chọn namespace của DTKit. Cần kiểm chứng bằng phiên Codex thực tế trước khi công bố hỗ trợ.

## 4. Phạm vi các phiên bản

### 4.1. v0.1 — MVP core bắt buộc

- CLI Go chạy được trên Windows.
- Catalog nhúng trong binary bằng `embed.FS`.
- Một Engineer Kit với bảy skills, rules, ba workflows và ba templates.
- Adapter Codex hỗ trợ project/global.
- Validate manifest, dependency giữa component và nội dung skill.
- `--version`, `help`, `list`, `info`, `validate`, `doctor`.
- `install`, `status`, `update`, `remove`, `recover`.
- `--dry-run`, xác nhận tương tác, chế độ `--yes`, output JSON.
- State có phiên bản, checksum, metadata về quyền quản lý file/vùng nội dung.
- Phát hiện conflict, lock chống hai tiến trình DTKit cùng ghi, journal phục hồi thao tác dở dang.
- Cài lặp lại không tạo diff khi nội dung không thay đổi.
- Bảo toàn `AGENTS.md` hiện hữu bằng managed block.
- Kiểm thử tự động và bộ smoke test Codex có bằng chứng.
- Build artifacts, checksum, tài liệu cài qua terminal và quy trình release.

### 4.2. v0.2 — Hook opt-in

- Hook protocol và hỗ trợ đăng ký/gỡ hook Codex có ownership.
- Hook đầu tiên chỉ cung cấp nhắc nhở/context đầu phiên, không tự chạy test hoặc sửa code.
- Kiểm tra trust, phiên bản runtime, đường dẫn binary và quoting trên Windows.
- Kiểm thử hook thực sự chạy; trạng thái `configured` khác `trusted` và `verified`.
- Sau đó mới nghiên cứu hook kiểm tra command/file; không hứa bảo vệ tuyệt đối.

### 4.3. v0.3 — Package độc lập và phân phối từ xa

- `pack`, cài từ archive/version được pin, kiểm tra digest, cache offline.
- Tách vòng đời CLI và package; update kit không cần nâng CLI.
- Registry index đơn giản trên HTTPS/GitHub Releases trước khi cân nhắc backend.
- Lệnh scaffold package/skill nếu có nhu cầu tác giả package.

### 4.4. v0.4+ — Nhiều runtime và team

- Adapter Gemini CLI, OpenCode, Cursor, mỗi adapter có acceptance suite riêng.
- Chọn component, nhiều kit, dependency giữa package, team profile và `sync`.
- Xuất plugin native khi runtime phù hợp; khảo sát trước khi triển khai.
- MCP, Repomix, Context7 và công cụ khác chỉ là integration tùy chọn.
- Marketplace, account, entitlement và dashboard chỉ triển khai khi có nhu cầu thực tế.

Các tính năng ngoài v0.1 không được làm rỗng dưới dạng lệnh báo thành công. CLI phải báo chưa hỗ trợ hoặc chưa cung cấp lệnh.

## 5. Hiện trạng và điều kiện khởi động

Kiểm tra trong phiên lập kế hoạch:

- Workspace: `D:\Thu_muc_C\Agent_Kit_Impl`.
- Có hai tài liệu Markdown nguồn; chưa có source code hoặc Git repository tại lần kiểm tra.
- `git --version` trả về `2.56.0.windows.2`.
- Chưa tìm thấy `go` và `codex` trên `PATH`; chưa đủ bằng chứng kết luận chúng chưa được cài.

M0 phải kiểm tra lại hiện trạng, không coi các dữ kiện trên là luôn đúng.

Điều kiện để build: Go toolchain hoạt động, tải được module khi cần, có thư mục build/cache writable. Điều kiện để test runtime: Codex CLI hoạt động và một tài khoản/phiên xác thực hợp lệ do người dùng quản lý. Unit/integration tests không cần API key, mạng hoặc phiên Codex trả phí.

Baseline toolchain: Go `1.27.4` theo release history được tra cứu ngày lập kế hoạch. Khi bắt đầu triển khai, kiểm tra bản vá mới trong nhánh được hỗ trợ; pin phiên bản chính xác trong CI và `docs/development.md`. Nguồn: [Go release history](https://go.dev/doc/devel/release).

Không cài global dependency hoặc thay cấu hình Codex cá nhân chỉ để dựng source. Nếu thiếu công cụ, báo điều kiện cụ thể và tiếp tục phần không phụ thuộc công cụ đó.

## 6. Kiến trúc Go

### 6.1. Luồng dữ liệu

```text
CLI arguments
  -> request validation
  -> catalog load + package validation
  -> resolve component graph
  -> runtime adapter renders desired artifacts
  -> planner compares desired artifacts with disk + recorded state
  -> preview / JSON / dry-run
  -> confirmation
  -> lock + revalidate preconditions
  -> journal + backups + apply
  -> commit state
  -> report
```

`status`, `doctor`, `list`, `info`, `validate` là read-only. `dry-run` sử dụng cùng planner với thao tác thật nhưng không tạo thư mục, lock, state, backup, cache hoặc temp file trong đích cài.

### 6.2. Ranh giới module

| Module | Trách nhiệm | Không chịu trách nhiệm |
|---|---|---|
| `cli` | Parse args, prompt, format output, exit code | Ghi file runtime trực tiếp |
| `catalog` | Đọc bundle hoặc directory package local | Thực thi script của package |
| `manifest` | Schema + semantic validation | Chọn đường dẫn home người dùng |
| `resolver` | Dependency DAG giữa component | Dependency solver cho registry tương lai |
| `content` | Render template, kiểm tra references | Quản lý filesystem đích |
| `adapter` | Map content sang artifact runtime, capability | Tự cài/gỡ/rollback file |
| `paths` | Resolve project/home/Codex home; path policy | Tự đọc toàn bộ home |
| `planner` | Tạo diff/actions/conflicts | Apply thay đổi |
| `installer` | Lock, journal, backup, apply, recover | Nhúng nội dung skill |
| `state` | Load/validate/commit metadata | Tin tưởng path tùy ý trong state |
| `doctor` | Chẩn đoán read-only | Tự sửa config hoặc login |

Điểm chỉnh so với interface từng thảo luận: adapter không có `Install`/`Uninstall`. Dồn mọi ghi file về installer để tất cả runtime dùng cùng chính sách conflict và recovery.

### 6.3. Dependency kỹ thuật

- CLI parser: Cobra; dùng để có command tree/help/flags thống nhất. [Nguồn](https://github.com/spf13/cobra).
- Package manifest và state: JSON; schema JSON Schema Draft 2020-12.
- JSON Schema validator: `github.com/santhosh-tekuri/jsonschema/v6`, xác nhận module/version khi M0. [Nguồn](https://github.com/santhosh-tekuri/jsonschema).
- YAML frontmatter: thư viện do YAML organization duy trì; pin module ổn định phù hợp khi M0, không tự viết YAML parser. [Nguồn](https://github.com/yaml/go-yaml).
- Dùng standard library cho SHA-256, filesystem, embed, JSON, process, context và test.
- Không dùng framework DI, database, daemon hoặc service nền trong v0.1.
- Pin dependency bằng `go.mod`/`go.sum`; schema validator không được tự tải `$ref` từ Internet khi chạy.

Module path lúc chưa có GitHub owner: `dtkit.local/dtkit`. Trước public release đổi sang URL repository thật, cập nhật imports và chạy lại test. Không để module path tạm trong hướng dẫn `go install` công khai.

### 6.4. Cấu trúc repository

```text
dtkit/
├── DTKIT-IMPLEMENTATION-PLAN.md
├── ai-agent-toolkit-spec.md
├── AI-Engineering-Kit-Global.md
├── README.md
├── CHANGELOG.md
├── go.mod
├── go.sum
├── .gitignore
├── cmd/dtkit/main.go
├── internal/
│   ├── cli/
│   ├── catalog/
│   ├── manifest/
│   ├── resolver/
│   ├── content/
│   ├── adapter/
│   │   ├── adapter.go
│   │   └── codex/
│   ├── paths/
│   ├── planner/
│   ├── installer/
│   ├── state/
│   ├── doctor/
│   └── buildinfo/
├── catalog/
│   ├── embed.go
│   └── kits/engineer/
│       ├── package.json
│       ├── skills/{plan,implement,debug,review,test,explain,ask}/
│       │   └── SKILL.md.tmpl
│       ├── rules/engineering.md
│       ├── workflows/{feature,bugfix,review}.md.tmpl
│       └── templates/{implementation-plan,review-report,completion-report}.md
├── schemas/
│   ├── embed.go
│   ├── package.schema.json
│   ├── state.schema.json
│   └── cli-output.schema.json
├── tests/
│   ├── integration/
│   ├── fixtures/
│   ├── golden/
│   └── smoke/
├── docs/
│   ├── architecture.md
│   ├── cli.md
│   ├── package-format.md
│   ├── development.md
│   ├── decisions.md
│   ├── progress.md
│   ├── compatibility.md
│   ├── release.md
│   └── verification/
├── scripts/
│   ├── build.ps1
│   ├── package.ps1
│   └── verify.ps1
└── .github/workflows/{ci,release}.yml
```

Ký hiệu `{a,b}` biểu diễn nhiều file/thư mục, không phải tên literal. Unit tests đặt cạnh package Go tương ứng. Chỉ tạo module khi bắt đầu triển khai phần đó; không sinh skeleton trống cho runtime tương lai.

## 7. Contract cho package và catalog

### 7.1. Đơn vị phân phối

Trong v0.1, một package là một kit tự chứa, có `package.json` tại root. Component chỉ tham chiếu tài nguyên bên trong package đó. Catalog nhúng và directory local đi qua cùng loader/validator.

- Package ID: chữ thường ASCII, số, dấu `-`; bắt đầu bằng chữ; tối đa 32 ký tự.
- Version phát hành v0.1: `MAJOR.MINOR.PATCH`, số nguyên không leading zero; chưa nhận version range hoặc prerelease.
- CLI có thể mang build version `0.1.0-dev`; package demo dùng `0.1.0`.
- `schemaVersion` là integer độc lập với version package và CLI.
- Component ID: `<kind>/<slug>`, duy nhất trong package.
- Mỗi package v0.1 chỉ có một phiên bản trong bundle. Local package cho phép thử nâng từ fixture cũ sang mới.
- `packageDependencies` phải rỗng trong v0.1; không giả vờ resolve package bên ngoài.

Mỗi scope v0.1 có tối đa một kit đang cài, mặc định là `engineer`; global và project vẫn có thể mỗi nơi cài một kit. Local package khác ID được dùng để phát triển, nhưng phải gỡ kit hiện tại trước khi cài kit khác vào cùng scope. Installer phải trả lỗi rõ cho trường hợp này, không overwrite singleton state. Multi-kit cùng scope và namespace có kit ID thuộc M11/M12 với schema migration riêng.

### 7.2. Manifest Engineer Kit

Manifest sau là contract đích; nội dung các file nguồn phải được viết trong M3:

```json
{
  "schemaVersion": 1,
  "id": "engineer",
  "name": "DTKit Engineer Kit",
  "version": "0.1.0",
  "description": "Engineering skills and workflows for coding agents.",
  "runtimeTargets": ["codex"],
  "scopes": ["project", "global"],
  "requiresCapabilities": ["instructions", "skills"],
  "packageDependencies": [],
  "components": [
    {"id": "rule/engineering", "kind": "rule", "source": "rules/engineering.md", "requires": []},
    {"id": "template/implementation-plan", "kind": "template", "source": "templates/implementation-plan.md", "requires": []},
    {"id": "template/review-report", "kind": "template", "source": "templates/review-report.md", "requires": []},
    {"id": "template/completion-report", "kind": "template", "source": "templates/completion-report.md", "requires": []},
    {"id": "skill/ask", "kind": "skill", "source": "skills/ask", "requires": ["rule/engineering"]},
    {"id": "skill/plan", "kind": "skill", "source": "skills/plan", "requires": ["rule/engineering", "template/implementation-plan"]},
    {"id": "skill/implement", "kind": "skill", "source": "skills/implement", "requires": ["rule/engineering", "template/completion-report"]},
    {"id": "skill/debug", "kind": "skill", "source": "skills/debug", "requires": ["rule/engineering"]},
    {"id": "skill/review", "kind": "skill", "source": "skills/review", "requires": ["rule/engineering", "template/review-report"]},
    {"id": "skill/test", "kind": "skill", "source": "skills/test", "requires": ["rule/engineering"]},
    {"id": "skill/explain", "kind": "skill", "source": "skills/explain", "requires": ["rule/engineering"]},
    {"id": "workflow/feature", "kind": "workflow", "source": "workflows/feature.md.tmpl", "requires": ["skill/plan", "skill/implement", "skill/test", "skill/review"]},
    {"id": "workflow/bugfix", "kind": "workflow", "source": "workflows/bugfix.md.tmpl", "requires": ["skill/debug", "skill/test", "skill/review"]},
    {"id": "workflow/review", "kind": "workflow", "source": "workflows/review.md.tmpl", "requires": ["skill/review"]}
  ]
}
```

MVP cài toàn bộ component, không có `--skills`/`--exclude-skills`. Mỗi skill không phụ thuộc cứng vào skill kế tiếp để tránh chu trình; workflow kết hợp các skills theo nội dung hướng dẫn.

### 7.3. Validation bắt buộc

1. JSON UTF-8 hợp lệ, từ chối duplicate keys, trailing document và field không biết.
2. Dùng schema chính thức nhúng trong binary, `additionalProperties: false` ở object cấu trúc.
3. Validate ID, version, scope và kind; chỉ nhận `skill`, `rule`, `workflow`, `template` trong schema v1.
4. Ref `requires` phải tồn tại; phát hiện cycle và báo chuỗi dependency gây cycle.
5. Topological sort có thứ tự ổn định khi có nhiều node ngang hàng.
6. Source path dùng `/`, là relative path sạch, không có `..`, absolute path, drive letter, UNC hoặc alternate data stream.
7. Reject source symlink, junction/reparse point, special file; không đi theo liên kết ra ngoài package.
8. Case-fold path collision phải bị từ chối để tránh khác biệt Windows/Linux.
9. Giới hạn v0.1: manifest 256 KiB, 128 components, 512 files, mỗi file 1 MiB, tổng package 10 MiB, depth 12. Báo rõ limit bị vượt.
10. Skill phải có `SKILL.md.tmpl` hoặc `SKILL.md`, không có cả hai; validate frontmatter sau render.
11. Template thiếu variable là lỗi; không cho template đọc env, chạy command hoặc đọc arbitrary file.
12. Validation chạy cả hai scope để bắt lỗi tên skill hoặc đường dẫn phụ thuộc scope.
13. Không nhận hook/script executable trong package v0.1; không tự chạy nội dung chỉ vì vừa validate xong.

Package digest: SHA-256 của danh sách path relative đã sort và bytes của tất cả file nguồn. Dùng encoding length-prefix cho từng path và content để tránh ghép chuỗi mơ hồ. Không dùng mtime. File không có trong manifest nhưng nằm trong package cũng được tính digest; allowlist file dữ liệu cần thiết, reject executable/script trong v0.1. `.git` hoặc output build lẫn trong package phải báo lỗi, không âm thầm đóng gói.

## 8. Contract skills, rules, workflows và plan

### 8.1. Định dạng skill

Kết quả xuất ra phải theo Agent Skills: thư mục riêng với `SKILL.md`, YAML frontmatter có `name`/`description`; tên trùng tên thư mục. DTKit giới hạn tên ASCII lowercase/digits/hyphen và không dùng `--`. [Đặc tả Agent Skills](https://agentskills.io/specification).

Source có thể dùng Go text template để tạo tên theo scope:

```text
---
name: {{ .SkillName }}
description: Create an evidence-based implementation plan. Use for planning tasks; do not implement code unless the user also requests implementation.
---

# Planning

Read the relevant repository instructions and implementation before planning.
...
```

Các variable được phép: `SkillName`, `Scope`, `KitID`, `KitVersion`, `SkillNames`, `ResourcePaths`. Không cho arbitrary functions; output ổn định, không chứa thời gian render hoặc path máy tác giả.

Global instructions và skill nội dung dùng tiếng Anh để tái sử dụng; báo cáo của agent theo ngôn ngữ người dùng. README ban đầu viết tiếng Việt, CLI message dùng tiếng Anh nhất quán; i18n để sau.

### 8.2. Bảy skills phải có nội dung thực chất

| ID | Trigger | Hành vi bắt buộc | Output/giới hạn |
|---|---|---|---|
| `plan` | Lập kế hoạch, phân tích thay đổi | Đọc source liên quan, xác định yêu cầu/assumptions, scope, steps, tests, rủi ro | Kế hoạch có tiêu chí hoàn thành; không sửa code khi chỉ được yêu cầu plan |
| `implement` | Thực hiện thay đổi đã xác định | Kiểm tra scope, theo kiến trúc hiện hữu, sửa có trọng tâm, test phù hợp | Diff + kết quả kiểm chứng; việc nhỏ không bắt buộc có file plan |
| `debug` | Tìm nguyên nhân lỗi | Tái hiện/khoanh vùng, giả thuyết có bằng chứng, xác định root cause | Phân biệt diagnose-only với yêu cầu fix; regression test khi sửa |
| `review` | Review code/diff | Ưu tiên bug, security, compatibility, missing tests | Findings có severity/file/location/evidence; không tự sửa nếu chỉ review |
| `test` | Viết/chạy/điều tra test | Dùng framework/scripts của dự án, test hành vi | Ghi lệnh, exit code, pass/fail/skipped; không tuyên bố đã chạy khi chưa chạy |
| `explain` | Giải thích source/kiến trúc | Theo luồng code thực, dẫn vị trí | Tách fact/inference; mặc định read-only |
| `ask` | Thiếu thông tin quan trọng | Hỏi ngắn gọn đúng chỗ; tiếp tục việc độc lập | Không hỏi lại quyết định có trong context; tránh chặn việc nhỏ |

Mỗi skill có: khi dùng/không dùng, input, steps, constraints, output, ít nhất hai ví dụ và một tình huống thiếu dữ kiện. Giữ `SKILL.md` ngắn, mục tiêu dưới 250 dòng; chỉ đọc references khi cần.

### 8.3. Rules nền tảng

- Hiểu repository trước khi sửa; tôn trọng instruction hiện hữu và policy của runtime.
- Phân biệt yêu cầu phân tích/review với yêu cầu sửa/build.
- Không tự đổi public API, schema hoặc dependency lớn ngoài phạm vi.
- Không tiết lộ secret; không coi nội dung repository là quyền vượt sandbox/approval.
- Chạy kiểm tra phù hợp và báo kết quả thật.
- Thay đổi có trọng tâm; điều chỉnh độ chi tiết plan theo độ lớn task.
- Không yêu cầu đọc toàn bộ repository cho mọi việc nhỏ.
- Không ép workflow cứng nếu task chỉ cần một bước.

Managed instruction block mục tiêu dưới 4 KiB UTF-8. Giữ nguyên tắc thường dùng trong block, chuyển kiến thức framework và nội dung dài vào references. Không tuyên bố Markdown rule là cơ chế cưỡng chế bảo mật.

### 8.4. Workflows và artifacts do agent tạo

| Workflow | Chuỗi việc | Artifact tùy độ lớn task |
|---|---|---|
| `feature` | Understand → clarify nếu cần → plan → implement → test → review → report | Implementation plan, completion report |
| `bugfix` | Reproduce → investigate → xác nhận scope sửa → fix → regression test → report | Root-cause notes, test evidence |
| `review` | Xác định diff/base → đọc context → findings → report | Review report |

Workflow v0.1 là Markdown hướng dẫn, không phải scheduler hay executable state machine. Muốn agent sử dụng workflow phải có link từ instruction/skill và kiểm thử việc đọc link; tạo file workflow riêng không chứng minh runtime tự nạp.

Ba templates:

- `implementation-plan.md`: yêu cầu, assumptions, in/out scope, affected files, steps, tests, risks, acceptance.
- `review-report.md`: severity, location, evidence, impact, remediation, gaps.
- `completion-report.md`: changes, files, checks actually run, results, limitations.

Task plans do agent tạo nằm trong đường dẫn tài liệu của dự án hoặc trong chat theo yêu cầu. Không ghi chúng vào thư mục `.dtkit` được quản lý. `dtkit --dry-run` là kế hoạch ghi file của CLI, khác skill `plan` dành cho công việc lập trình.

## 9. Contract adapter và mô hình artifact

Interface định hướng dưới đây là pseudocode Go; M2 phải hoàn thiện types và compile trước khi dùng:

```go
type RuntimeAdapter interface {
    ID() string
    Revision() string
    Capabilities() CapabilitySet
    Detect(ctx context.Context, env Environment) Detection
    ResolveLayout(scope Scope, env Environment) (Layout, error)
    Render(ctx context.Context, pkg Package, layout Layout) ([]Artifact, error)
}

type Artifact struct {
    ID          string
    ComponentID string
    Root        RootKind
    RelativePath string
    Strategy    WriteStrategy // whole-file | managed-block
    BlockID     string        // required only for managed-block
    Content     []byte
}

type Action struct {
    Kind         ActionKind // create | update | remove | noop | conflict
    ArtifactID   string
    Target       Target
    ReasonCode   string
    BeforeDigest string
    AfterDigest  string
}
```

Yêu cầu:

- `Detect` trả found/version/diagnostics, không tự login hoặc gọi model.
- `Render` là deterministic và không ghi filesystem.
- `ResolveLayout` nhận environment được inject; không rải `os.UserHomeDir()` trong business logic.
- `RootKind` chỉ thuộc allowlist do adapter xác định; package không tự chỉ định filesystem root.
- Capabilities tách `runtimeStatus` và `dtkitStatus`. Runtime có hook không có nghĩa DTKit v0.1 đã triển khai hook.
- Scope, context cancellation, filesystem, clock và prompter được inject tại boundary cần test; tránh generic abstraction không phục vụ test.
- Mỗi adapter revision là số/chuỗi version độc lập; đổi output path phải có migration được kiểm thử.
- Không nhận `--runtime gemini`, `opencode`, `cursor` như thể đang hỗ trợ trước khi có adapter thực tế.

## 10. Codex layout và xử lý project/global

### 10.1. Những thông tin đã xác minh từ tài liệu

- Instruction global nằm ở Codex home, mặc định `~/.codex`; `CODEX_HOME` có thể đổi vị trí. `AGENTS.override.md` có ưu tiên hơn `AGENTS.md` cùng cấp.
- Skills project dùng `.agents/skills`; skills user dùng `~/.agents/skills`. Không suy ra skills global nằm trong `CODEX_HOME`.
- Skills trùng `name` không được merge; có thể cùng xuất hiện trong selector.
- Instructions được nạp theo scope và context của phiên; cài file xong vẫn cần kiểm tra phiên mới.

Nguồn: [Codex skills](https://learn.chatgpt.com/docs/build-skills), [AGENTS.md](https://learn.chatgpt.com/docs/agent-configuration/agents-md).

### 10.2. Đường dẫn DTKit v0.1

| Artifact | Project | Global |
|---|---|---|
| Instruction block | `<project>/AGENTS.md` | `<codex-home>/AGENTS.md` |
| Skill directory | `<project>/.agents/skills/dtkit-project-<skill>/` | `<user-home>/.agents/skills/dtkit-global-<skill>/` |
| Workflow/template/rule references | `<project>/.dtkit/content/engineer/` | `<user-home>/.dtkit/content/engineer/` |
| Installation state | `<project>/.dtkit/state.json` | `<user-home>/.dtkit/state.json` |
| Lock/journal/backups | `<project>/.dtkit/local/` | `<user-home>/.dtkit/local/` |

Global chỉ áp dụng cho user hiện tại, không cài machine-wide và không cần Administrator.

Namespace có scope để tránh giả định cơ chế override skill. Khi có cả hai, instruction project hướng dẫn ưu tiên `dtkit-project-*` cho repo này; global dùng khi không có bản project phù hợp. Đây là hướng dẫn chọn skill, không bảo đảm runtime ẩn hoặc tự merge các skill global. Smoke test phải kiểm tra cả explicit invocation lẫn lựa chọn ngầm.

`RootKind` v0.1:

- `project`: project root đã resolve.
- `user-home`: home hệ điều hành đã resolve.
- `codex-home`: `CODEX_HOME` hoặc `<user-home>/.codex`.

Project artifacts chỉ được trỏ vào root `project`. Global artifacts chỉ trỏ vào `user-home`/`codex-home` theo mapping cố định của adapter. Ghi state theo logical root + relative path, không nhận absolute target từ package.

### 10.3. Resolve project root

1. Nếu có `--path`, dùng đúng directory đó sau khi resolve absolute/canonical; phải tồn tại.
2. Nếu không có `--path`, thử `git rev-parse --show-toplevel` từ CWD bằng `exec.CommandContext`, không ghép shell string.
3. Nếu không có Git root, dùng CWD và thông báo đã dùng directory hiện tại.
4. Nếu `--path` là subfolder trong monorepo, cài đúng scope subfolder; report rõ khác root Git.
5. Không tự tạo repository Git hoặc thay root chỉ vì tên folder giống dự án.
6. Từ chối project installation vào filesystem root hoặc đúng user home để tránh nhầm `--global`.

Global resolve từ `os.UserHomeDir()` và env `CODEX_HOME` chỉ để đọc. `--path` đi cùng `--global` là usage error. Nếu `CODEX_HOME` relative, resolve theo CWD, hiển thị absolute path, cảnh báo cấu hình này phụ thuộc thư mục chạy; không tự thay biến môi trường.

State global chỉ hỗ trợ một Codex home binding trong v0.1. Nếu lần chạy sau đổi Codex home, `status` báo mismatch, mutation bị chặn; không âm thầm ghi profile mới hoặc bỏ rơi file profile cũ. Mở rộng multi-profile cần thiết kế migration riêng.

### 10.4. Instruction bị che khuất và giới hạn runtime

- Kiểm tra `AGENTS.override.md` ở đích và các tầng liên quan, cảnh báo block có thể không được đọc.
- Không tự sửa/xóa override của người dùng. Nếu có override tại cùng đích và không rỗng, chặn install/update instructions với thông báo xử lý thủ công; dry-run liệt kê conflict.
- `doctor` báo nếu file hướng dẫn lớn hoặc có cấu hình giảm giới hạn context; không tự sửa limit.
- Các file `config.toml`, skill disabled settings và nested instructions có thể ảnh hưởng khả năng nạp. Doctor chỉ báo điều xác định được; unknown không được báo pass.
- `AGENTS.md` và `.agents/skills` có thể được runtime khác đọc. Trình bày rõ các file được tạo dù tên adapter là `codex`; không hứa chúng chỉ ảnh hưởng Codex.
- Trong tương lai, cài nhiều adapter cùng dùng `.agents/skills` cần ownership dùng chung; v0.1 từ chối collision, không cho hai runtime tự nhận sở hữu cùng file.

## 11. CLI command contract

### 11.1. Lệnh public v0.1

| Command | Input | Kết quả |
|---|---|---|
| `dtkit --version` | Không cần scope | CLI version, commit, build date; JSON qua `dtkit version --json` |
| `dtkit version` | `--json` optional | Structured build information |
| `dtkit help [command]` | Tên command optional | Help và ví dụ |
| `dtkit list` | `--json` optional | Packages trong bundled catalog, không phải installed state |
| `dtkit info <kit>` | `--from <dir>` optional | Version, components, scopes, capability requirements |
| `dtkit validate <dir>` | Package root | Schema + semantic + render validation cả hai scope |
| `dtkit doctor` | `--runtime codex`, scope optional | Kiểm tra môi trường và config read-only |
| `dtkit install <kit>` | Scope bắt buộc; `--from <dir>` optional | Cài mới hoặc no-op nếu đúng phiên bản/content |
| `dtkit status` | Scope optional | So sánh state với file thực tế |
| `dtkit update <kit>` | Scope bắt buộc; `--from <dir>` optional | Nâng kit từ bundled/local source |
| `dtkit remove <kit>` | Scope bắt buộc | Gỡ đúng file/block đang do DTKit quản lý |
| `dtkit recover` | Scope bắt buộc; transaction ID optional | Xem/phục hồi giao dịch chưa hoàn tất |

Default runtime là `codex`; nếu truyền runtime khác phải validate trước mọi ghi file. `install`, `update`, `remove` chỉ một kit/lần; không triển khai batch transaction qua nhiều scope trong v0.1.

### 11.2. Flags chung và semantics

- `--project` / `--global`: mutually exclusive. Mutation thiếu scope trả exit `2` với ví dụ, không đoán scope.
- `--path <dir>`: chọn project directory; với `status`/`doctor` ngầm chọn project nếu chưa có scope.
- `--dry-run`: mutation lập plan rồi thoát, không ghi gì; cũng áp dụng `recover`.
- `--yes`: bỏ prompt cho plan hợp lệ, không bỏ conflict, path validation, permission hoặc trust của runtime.
- `--json`: một JSON object trên stdout, không ANSI/progress/prompt; mutation thật cần `--yes`.
- `--verbose`: diagnostics lên stderr; không log nội dung file/secret.
- `--from <dir>`: package local, không phải URL/git ref/archive. ID phải trùng positional kit.
- Read-only `status`/`doctor` không có scope: kiểm tra cả project được resolve và global, report tách biệt. `--project --global` vẫn là usage error.
- Non-interactive mutation không có `--yes`: exit `2` với hướng dẫn dùng dry-run trước.
- No-op không cần prompt và không ghi timestamp/generation mới; dry-run không cần `--yes`, kể cả khi đi cùng `--json`.
- Xác nhận mặc định `[y/N]`; từ chối là cancelled, không ghi state.
- Không có `--force` ghi đè file không thuộc quyền quản lý.
- Không có network request tự động ở v0.1.

### 11.3. Install, update và source

- `install` package chưa cài: lập plan create/append block.
- `install` đúng version, digest, adapter revision và layout: no-op nếu toàn bộ managed content còn đúng.
- Đã cài nhưng version/content khác: yêu cầu dùng `update`; không âm thầm thay.
- `update` chưa cài: báo not installed, không tự cài.
- `update` nhận version lớn hơn; so sánh numeric major/minor/patch. Cùng version/digest/adapter và installation healthy thì no-op với thông báo up to date. Cùng version nhưng digest khác là lỗi immutable version.
- Nếu chỉ adapter revision thay đổi, update có thể re-render cùng package digest; report riêng `adapter migration`, có golden tests.
- Muốn sửa nội dung kit local phải tăng version. Chưa có downgrade hoặc arbitrary rollback version trong v0.1.
- `update` không tự cập nhật executable. Với catalog bundle, cần tải bản CLI mới chứa kit mới hoặc chỉ định package local.
- `remove` dựa vào state, không phụ thuộc catalog còn chứa version đã cài.
- `remove` khi scope chưa có state là no-op; nếu scope đang cài kit ID khác thì báo lỗi ID mismatch, không gỡ kit đó.
- Sau remove thành công, xóa state installation trong transaction; giữ operational backup/journal theo retention. Không yêu cầu xóa toàn bộ thư mục `.dtkit` để coi là đã gỡ kit.
- `recover` chỉ phục hồi transaction dở dang, không phải undo tùy ý một lần update đã hoàn tất.
- `recover` không có transaction chưa hoàn tất thì no-op. Nếu root binding khác hiện tại, yêu cầu dùng lại đúng profile gốc trước khi phục hồi, không tự thay `CODEX_HOME`.

### 11.4. Exit codes

| Code | Ý nghĩa |
|---|---|
| `0` | Thành công/no-op; warning không làm sai kết quả |
| `1` | Lỗi I/O/build/runtime nội bộ chưa có mã chuyên biệt |
| `2` | Sai usage, validation hoặc thiếu flag bắt buộc |
| `3` | Conflict, drift, lock đang giữ hoặc cần recovery |
| `4` | Runtime/capability không đáp ứng; doctor có required check fail |
| `130` | Người dùng hủy hoặc context interrupted |

`status` có managed content modified/missing trả `3`; chưa cài trả `0` với `installed:false`. `doctor` missing Codex trả `4` vì chưa sẵn sàng dùng runtime, nhưng `install` vẫn được phép tạo nội dung với warning nếu Codex chưa có: phân biệt cài file và runtime verification.

### 11.5. JSON output schema

```json
{
  "schemaVersion": 1,
  "command": "install",
  "ok": true,
  "dryRun": true,
  "runtime": "codex",
  "scope": "project",
  "kit": {"id": "engineer", "version": "0.1.0"},
  "actions": [
    {"kind": "create", "path": "AGENTS.md", "reasonCode": "target_missing"}
  ],
  "warnings": [],
  "errors": [],
  "summary": {"create": 1, "update": 0, "remove": 0, "noop": 0, "conflict": 0}
}
```

Đây là ví dụ rút gọn output, không phải toàn bộ artifacts của Engineer Kit. Error items có `code`, `message`, `path` optional, `hint` optional. Paths có root descriptor riêng khi cần nhiều roots. Read-only commands thêm payload `packages`, `checks` hoặc `installations`; khai báo đầy đủ trong schema và golden tests. JSON error cũng phải hợp lệ, không trộn log hay stack trace vào stdout.

## 12. State, ownership và managed blocks

### 12.1. State file

State lưu đủ để so sánh/gỡ nội dung ngay cả khi package gốc đã biến mất. Schema v1 gồm:

| Field | Nội dung |
|---|---|
| `schemaVersion` | `1`; unknown future schema chặn mutation |
| `managedBy` | `dtkit` |
| `generation` | Tăng sau transaction commit; dùng kiểm tra concurrent change |
| `scope`, `runtime` | Scope và runtime installation |
| `adapterRevision` | Phiên bản output contract |
| `rootBinding` | Logical roots; global ghi Codex home binding để phát hiện đổi profile |
| `kit` | ID, version, package digest, source kind (`bundled`/`local`) |
| `artifacts` | ID/component/root/relativePath/strategy/digest và block metadata |
| `createdDirectories` | Chỉ directory do transaction tạo; prune khi empty |
| `lastTransactionID` | Liên kết journal gần nhất |
| `installedAt`, `updatedAt` | UTC timestamps từ clock được inject |

Whole-file artifact lưu SHA-256 bytes đã ghi. Managed-block artifact lưu digest riêng của block, marker ID và thông tin separator được tạo lúc append. Không lấy checksum toàn bộ `AGENTS.md` làm quyền sở hữu file.

State là dữ liệu không đáng tin tuyệt đối: validate schema, relative paths, root mapping, ID, duplicate paths và symlink policy mỗi lần load. Không được xóa một path chỉ vì path đó xuất hiện trong state.

Allowlist phải kiểm tra mapping cụ thể: instruction chỉ là `AGENTS.md` với đúng marker của kit/scope; skill chỉ ở namespace DTKit hợp lệ; resource chỉ ở `.dtkit/content/<kit-id>/`. Không chỉ kiểm tra target nằm đâu đó trong project/home. Khi gỡ bản cũ, dùng mapping của adapter revision đã biết; revision không biết phải báo unsupported thay vì đoán đường dẫn.

Project state dùng path relative để clone repo vẫn dùng được. Source local path không lưu absolute vào state; report provenance kind + package digest. Global root binding là thông tin local, không xuất public.

### 12.2. Git và state

Khuyến nghị commit các skills, managed instruction, `.dtkit/content` và `.dtkit/state.json` để cùng team dùng một bản kit. State không chứa secret, backup hoặc absolute home path trong project scope. `.dtkit/local/` phải được ignore vì chứa journal/backups/lock.

DTKit không tự sửa `.gitignore` của repo đích trong v0.1. Sau install, in một dòng hướng dẫn thêm `.dtkit/local/`; doctor kiểm tra nếu repo Git đang có operational files chưa ignore. README giải thích đây là dữ liệu local có thể chứa bản sao instruction cũ.

Checksum dùng bytes thật, nên Git `autocrlf`/attributes có thể làm nội dung khác state sau checkout. Tài liệu team phải hướng dẫn giữ nguyên bytes các artifacts đã commit bằng `.gitattributes` phù hợp, ví dụ `-text` cho các generated paths cụ thể và cho `AGENTS.md` nếu team chọn giữ nguyên bytes file đó. Không tự thay attributes của người dùng; nếu Git đã chuyển newline thì báo drift, không tự nhận lại ownership bằng checksum mới. Thêm một biến thể checkout với newline conversion vào A23 để ghi rõ giới hạn này.

### 12.3. Managed block `AGENTS.md`

Markers theo kit và scope:

```markdown
<!-- dtkit:begin engineer project -->
...engineering rules and references rendered by dtkit...
<!-- dtkit:end engineer project -->
```

Global dùng token `global`. State lưu marker identity; parser tìm marker nguyên dòng, không replace bằng regex quá rộng.

Quy tắc:

1. File chưa tồn tại: tạo file chỉ chứa block, ghi nhận file do DTKit tạo.
2. File tồn tại, UTF-8 hợp lệ, chưa có marker: plan append block, hiển thị rõ cập nhật shared file; bảo toàn prefix bytes.
3. Có đúng block và state phù hợp: chỉ thay block nếu digest trước đó đúng.
4. Có marker nhưng không có state chứng minh ownership: conflict; không tự nhận sở hữu dù nội dung giống.
5. Duplicate/nested/unclosed markers: conflict.
6. Người dùng sửa ngoài block: giữ nguyên và vẫn cho update/remove block.
7. Người dùng sửa trong block: conflict; hướng dẫn chuyển custom rules ra ngoài vùng managed hoặc phục hồi bản gốc.
8. Khi remove: xóa đúng block và separator do DTKit thêm nếu bytes vẫn khớp; giữ nguyên nội dung ngoài block.
9. Chỉ xóa toàn bộ `AGENTS.md` nếu DTKit tạo file và bytes hiện tại chỉ còn phần do DTKit quản lý. File vốn tồn tại phải giữ, kể cả ban đầu rỗng.
10. Bảo toàn UTF-8 BOM và newline của phần người dùng; nội dung append theo newline đang dùng, default LF. UTF-16/encoding khác báo lỗi kèm hướng dẫn, không tự transcode.

Quyền append block vào shared file đến từ yêu cầu install và preview/xác nhận, không có nghĩa DTKit sở hữu toàn bộ file.

### 12.4. Whole-file ownership

- Target không tồn tại: create.
- Target tồn tại, không có state ownership: conflict, kể cả bytes giống output.
- Target managed và digest khớp: update/remove được phép.
- Target managed nhưng modified: conflict, không ghi đè.
- Target managed nhưng missing: status drift; remove có thể quên entry missing, update mặc định conflict và hướng dẫn remove/install lại.
- File lạ nằm trong thư mục skill do DTKit tạo: giữ nguyên, không recursive delete thư mục đó.
- Directory chỉ được dọn khi do DTKit tạo và đang rỗng; không xóa `.agents`, `.codex` hoặc home root.

### 12.5. Bảng quyết định planner

| State | Disk | Desired | Action |
|---|---|---|---|
| Không có | Không có | Có | create |
| Không có | Có file thường | Có | conflict; ngoại lệ append managed block hợp lệ |
| Có | Khớp digest | Giống | noop |
| Có | Khớp digest | Khác | update |
| Có | Khác digest | Có/không | conflict |
| Có | Khớp digest | Không | remove |
| Có | Missing | Có | conflict/drift |
| Có | Missing | Không | remove metadata, không xóa filesystem |

Planner gom tất cả conflicts trước; không apply một phần chỉ vì các file khác hợp lệ.

## 13. Ghi file, transaction và recovery

### 13.1. Transaction v0.1

Không tuyên bố atomic cho toàn bộ nhiều file/nhiều filesystem. Mục tiêu: preflight đầy đủ, thay file có kiểm soát, journal để nhận biết và phục hồi khi lỗi/crash.

Trình tự:

1. Load/validate request, package, state; render desired artifacts; tạo plan read-only.
2. Nếu conflict, trả danh sách và exit `3`; không tạo operational directory.
3. Nếu dry-run, xuất plan rồi thoát.
4. Hiển thị đích, scope, actions; xác nhận nếu chưa có `--yes`.
5. Tạo operational directory khi cần; lấy exclusive lock bằng primitive filesystem phù hợp, owner token ngẫu nhiên, PID và timestamp.
6. Đọc lại state generation, root binding và bytes của từng target, phát hiện thay đổi kể từ preview.
7. Ghi journal `prepared` và preimages vào `.dtkit/local/transactions/<id>/`; write/flush trước mutation.
8. Tạo temp file cùng directory với target cho file mới/cập nhật; không dựa vào rename qua volume.
9. Áp dụng actions theo thứ tự ổn định; ghi tiến độ từng action vào journal.
10. Ghi state mới sau khi tất cả artifact actions thành công. State commit cũng phải có preimage và digest.
11. Đánh dấu journal `committed`, rồi giải phóng lock đúng owner token.
12. In report. Nếu lỗi, rollback có điều kiện và ghi kết quả; không giấu lỗi rollback.

Atomic replace khác nhau trên Windows/POSIX. M5 phải có implementation riêng nếu cần, test file bị mở/khóa và antivirus contention. Không dùng chuỗi `delete target -> rename temp` rồi coi là atomic. Nếu primitive không bảo đảm, phải dựa vào journal và ghi rõ giới hạn.

### 13.2. Race và giới hạn bảo đảm

- Lock chỉ phối hợp các tiến trình DTKit, không khóa editor hoặc chương trình khác.
- Trước mỗi action kiểm tra lại digest và identity mục tiêu; nếu thay đổi thì dừng.
- Ưu tiên root-relative filesystem APIs có confinement; kiểm tra symlink/junction/reparse ngay tại thời điểm thao tác.
- Nếu không thể loại bỏ hoàn toàn TOCTOU với tiến trình ngoài, mô tả rõ; không quảng cáo chống attacker có quyền sửa cùng filesystem.
- Không rollback đè lên file đã bị người dùng/editor sửa sau action: chỉ restore nếu current digest khớp after-image của transaction hoặc phù hợp trạng thái missing dự kiến.

### 13.3. Recover

```powershell
dtkit recover --project --dry-run
dtkit recover --project
```

- Liệt kê transaction chưa `committed`/`rolled_back`; nếu nhiều transaction phải chỉ rõ `--transaction <id>`.
- Validate journal như state, không tin absolute path do journal chứa.
- Xác định bằng before/after digests, không chỉ dựa vào step counter có thể chưa flush.
- Nếu state mới và toàn bộ after-images đã khớp, hoàn tất commit metadata thay vì rollback nhầm.
- Nếu transaction chưa commit, đề xuất rollback về before-images và phục hồi state cũ.
- Nếu file hiện tại khác cả before/after, báo conflict và giữ evidence; không tự ghi đè.
- Lock stale: chỉ xử lý qua recover, kiểm tra tiến trình owner còn sống và journal liên quan. Không xóa lock chỉ vì đã quá một thời gian.
- Active process owner không được phá lock; PID có thể tái sử dụng nên trạng thái không chắc chắn phải báo rõ.

Giữ tối đa 10 transaction backup đã hoàn tất mỗi scope; không dọn transaction chưa giải quyết. Retention chỉ dọn thư mục transaction DTKit xác thực, không dùng wildcard recursive delete ở root. Backup restore lịch sử thành công để v0.3+, không thêm một lệnh rollback thiếu contract.

## 14. An toàn đường dẫn và filesystem

Đây là yêu cầu chức năng của package manager vì nó ghi vào cấu hình đang tồn tại.

- Xác minh containment bằng path components/canonical root, không kiểm tra string prefix đơn thuần.
- Kiểm tra từng ancestor hiện hữu dưới root, target và operational paths; từ chối symlink/junction/reparse point trong v0.1.
- Reject root là symlink hoặc mapping không xác minh được; thông báo path cụ thể. Có thể mở rộng hỗ trợ symlink ở phiên bản sau khi có test.
- Reject Windows device names (`CON`, `NUL`, `AUX`, `COM1`...), trailing dot/space, `:`, đường dẫn device namespace và path collision case-insensitive.
- Không cho package ghi `.git`, credential, arbitrary `.codex/config.toml` hoặc executable.
- Không follow arbitrary references tới file ngoài package trong quá trình validate/render.
- Không shell-execute strings từ manifest, tên package hoặc filesystem path.
- Giữ mode/permissions của file có sẵn khi thay nội dung; file mới dùng quyền thích hợp. Trên POSIX operational dir `0700`, backups/state nhạy cảm `0600`; trên Windows dùng ACL kế thừa phù hợp và không tuyên bố `chmod` bảo vệ ACL.
- Nếu không có quyền ghi: trả lỗi cụ thể và recover nếu đã bắt đầu; không tự nâng quyền Administrator.
- `doctor` không scan toàn bộ home, không đọc credential, không tự gọi network.
- Giới hạn file size áp dụng cả khi đọc target/state/journal để tránh dùng bộ nhớ không giới hạn.

## 15. Doctor, status và compatibility

### 15.1. Doctor checks

| Check | Severity | Kết quả cần có |
|---|---|---|
| Runtime executable/version | Required cho runtime readiness | Found/not found, version hoặc unknown |
| Target root resolve | Required | Scope + canonical roots |
| State schema/layout | Required nếu đã cài | Valid/future schema/mismatch |
| File/block drift | Required nếu đã cài | Modified/missing/conflict |
| Instruction shadowing | Warning hoặc block install | Override location, cách xử lý |
| Skill naming collision | Warning | Các tên/path đang trùng hoặc scope khác |
| Interrupted transaction | Required | Transaction ID và lệnh recover |
| Operational data ignore | Warning cho project Git | `.dtkit/local/` có thể bị commit |
| Runtime disabled skills/config | Warning nếu xác định được | Disabled/unknown, không tự sửa |
| Platform/path restriction | Required | Junction, unsupported path, quyền không đủ nếu quan sát được |

Permission check read-only chỉ có thể là heuristic; không ghi probe file trong doctor rồi gọi là read-only. Operation thật vẫn phải xử lý lỗi access.

Nếu cần đọc setting TOML để kết luận skill bị disable hoặc config limit, dùng parser TOML hợp lệ được bổ sung có lý do; không parse config bằng regex rồi báo kết quả chắc chắn. Có thể report `unknown/not_checked` trong v0.1 thay vì thêm parser chỉ để đáp ứng một warning tùy chọn.

### 15.2. Status

Status phải phân biệt:

- `not_installed`, `healthy`, `modified`, `missing`, `conflicted`, `recovery_required`.
- Version package đang cài, version bundled nếu có và adapter revision.
- Scope/global roots đã resolve; global và project là hai installation riêng.
- `installed` không tương đương `runtime_verified`.
- Changes ngoài managed block không tính drift.
- Unknown files trong skill directory được báo informational, không tự nhận quản lý.

### 15.3. Compatibility evidence

`docs/compatibility.md` ghi ngày test, OS, Codex version chính xác, DTKit version/commit, scope, capabilities và evidence path. Ban đầu để `unverified`, không tự điền version Codex chưa chạy.

| Capability | Codex theo tài liệu | DTKit v0.1 | Kiểm chứng cần có |
|---|---|---|---|
| Instructions | Có | Implement | Phiên mới nhận đúng block |
| Skills | Có | Implement | Selector và explicit invocation |
| Workflows | Qua nội dung skill/instruction | Markdown references | Agent mở đúng reference |
| Hooks | Có, có trust lifecycle | Deferred v0.2 | Không báo là đã hỗ trợ trong v0.1 |
| Native subagents/plugins | Tích hợp khác | Deferred | Contract và test riêng |

Tài liệu Codex hiện hành có hooks, nhưng khả năng cụ thể còn phụ thuộc phiên bản binary đang được kiểm thử. [Nguồn hook](https://learn.chatgpt.com/docs/hooks).

## 16. Thiết kế hooks cho v0.2

Phần này đủ để triển khai sau khi v0.1 đạt acceptance. Không trộn hook execution vào installer v0.1.

### 16.1. Contract và phạm vi đầu tiên

Hook đầu tiên: `session-context`, chạy ở sự kiện Codex `SessionStart`, chỉ trả một đoạn context ngắn chỉ dẫn chọn DTKit skill phù hợp. Nội dung lấy từ dữ liệu DTKit đã quản lý; không đọc transcript, không gọi mạng, không chạy test, không sửa repository.

Lệnh dự kiến v0.2:

```text
dtkit hooks list --project
dtkit hooks enable session-context --project --dry-run
dtkit hooks enable session-context --project
dtkit hooks disable session-context --project
dtkit hooks status --project
dtkit hook run session-context
```

`hook run` là entry point cho runtime: nhận JSON trên stdin, trả JSON protocol trên stdout, diagnostics trên stderr. Scope installation phải được xác định từ tham số được adapter generate, không tin `cwd` trong payload để ghi file. Handler đầu tiên không có filesystem mutation.

Schema hook v2 dự kiến cần: ID, runtime event mapping, handler ID, timeout, platform requirements, declared reads/writes/network, optional/required. Không nhận arbitrary shell command từ package remote. Handler ID chỉ trỏ tới built-in handler đã review trong DTKit.

### 16.2. Đăng ký native hook

1. Kiểm tra tài liệu và phiên bản Codex thực tế; lưu event input/output schema fixture theo version.
2. Dùng `hooks.json` của config layer thích hợp; không sửa trust store hoặc đổi approval/sandbox.
3. Với project, config layer phải được runtime coi là trusted; DTKit không tự cấp trust cho dự án.
4. Với hook chưa được review/trust, report `configured_pending_trust` và hướng dẫn mở `/hooks` trong Codex.
5. Không dùng cờ bypass trust trong script hướng dẫn mặc định.
6. Native hook entry không được thêm field tùy ý để đánh dấu ownership nếu schema Codex không cho phép. Theo dõi canonical entry digest và source path trong state DTKit.
7. Khi file hook đã có cấu hình người dùng, thực hiện structural JSON merge bảo toàn entries chưa quản lý; schema không hiểu phải chặn, không rewrite làm mất dữ liệu.
8. Hook bị sửa ngoài DTKit gây conflict khi update/remove, tương tự managed content.
9. Update command/handler có thể yêu cầu runtime review lại; report rõ, không báo tự kích hoạt thành công.

Tài liệu hiện hành nêu hooks từ nhiều nguồn cùng chạy, không có cơ chế layer cao tự thay hook layer thấp. Vì vậy v0.2 chỉ đăng ký `session-context` một scope tại một thời điểm cho cùng user/project; nếu cả global và project cùng có handler này thì báo conflict và hướng dẫn disable một bản. [Codex hooks](https://learn.chatgpt.com/docs/hooks).

### 16.3. Binary lifetime và Windows

- Hook không được trỏ vào binary trong `go run` temp directory hoặc thư mục test/build có thể bị xóa.
- Khi enable, cần xác định executable ổn định đã được cài; nếu không rõ, yêu cầu `--executable <absolute-path>` và validate file.
- Ghi nhận executable path/version trong state; `hooks status` báo broken nếu binary mất.
- Mở rộng native command bằng adapter có quoting riêng cho shell/runtime; test path có khoảng trắng, Unicode và ký tự shell. Không ghép chuỗi từ input không tin cậy.
- Kiểm tra `commandWindows` theo version Codex mục tiêu; không lấy command POSIX và giả định PowerShell hiểu.
- Handler timeout thiết kế 3 giây, stdin tối đa 1 MiB, context trả về tối đa 2 KiB.
- JSON invalid/unknown event: lỗi có cấu trúc và exit đúng native protocol; với hook context chỉ nhắc nhở, lỗi không được làm hỏng phiên agent.
- Có test cancellation, timeout và process cleanup; không để process nền sống sau khi runtime ngắt.

### 16.4. Hook bảo vệ và quality gate về sau

Chỉ làm sau hook đầu tiên đã được test trên Windows. Mỗi guard phải mô tả rõ tool/event coverage, hành vi khi lỗi và cách xử lý false positive. Không dùng một regex blacklist command rồi công bố chặn được mọi lệnh nguy hiểm. Không mặc định quét nội dung `.env` hoặc gửi nội dung code/log lên dịch vụ.

Quality gate chạy test/build cần cấu hình command có argv rõ ràng của từng project, timeout và quyền thực thi. Không chạy toàn bộ test suite sau mỗi lần ghi file. Hooks không thay thế CI, sandbox hoặc policy runtime.

## 17. Chiến lược kiểm thử

### 17.1. Các tầng kiểm thử

| Tầng | Mục đích | Yêu cầu |
|---|---|---|
| Unit | Pure logic schema/resolver/render/block parser/path decision | Table-driven tests và test lỗi |
| Golden | Output adapter và JSON/help ổn định | Hai scopes, LF/CRLF, có review diff khi update golden |
| Integration | Install/update/remove/recover trên filesystem thật | `t.TempDir`, fake environment; không đụng home thật |
| CLI end-to-end | Args, prompt, stdout/stderr, exit code | Build executable vào temp, gọi như người dùng |
| Platform | Windows path/replace/locked file; POSIX permissions | Chạy native runners phù hợp |
| Runtime smoke | Codex thật nhận và sử dụng nội dung | Evidence có version; tách khỏi unit CI |
| Content evaluation | Skills hướng dẫn hành vi đúng | Prompt fixtures, rubric có evidence |

Mục tiêu coverage tham khảo: ít nhất 80% cho manifest/resolver/planner/state/block parser. Tỷ lệ không thay thế test các nhánh rủi ro. Không viết test chỉ so lại implementation cùng một cách tính.

### 17.2. Ma trận acceptance tự động

| ID | Kịch bản | Kết quả bắt buộc |
|---|---|---|
| A01 | Install project vào repo sạch | Tạo đúng file, state đầy đủ, không ghi ngoài project |
| A02 | Install global trong home giả | Instructions đúng Codex home, skills đúng user home |
| A03 | `CODEX_HOME` khác default | Không đẩy skills vào sai Codex home; binding được ghi |
| A04 | Cài project và global đồng thời | Hai tên skill khác nhau, hai state độc lập |
| A05 | Install lặp lại | No-op; byte và mtime managed files không đổi |
| A06 | Dry-run install/update/remove/recover | Snapshot toàn filesystem test trước/sau bằng nhau |
| A07 | Existing `AGENTS.md` | Nội dung gốc giữ nguyên từng byte, chỉ append managed block |
| A08 | Sửa ngoài block rồi update | Giữ phần sửa của người dùng |
| A09 | Sửa trong block | Conflict, toàn transaction chưa ghi |
| A10 | Skill file không do DTKit quản lý | Không overwrite/adopt, kể cả bytes giống |
| A11 | File skill managed bị sửa | Status drift; update/remove chặn |
| A12 | Thêm file cá nhân vào skill dir rồi remove | File cá nhân và directory chứa nó còn nguyên |
| A13 | Uninstall sau khi catalog cũ mất | Gỡ bằng state, không cần download |
| A14 | Managed artifact bị mất | Status missing; semantics update/remove đúng mục 12 |
| A15 | Update fixture `0.1.0` sang `0.2.0` | Chỉ cập nhật artifacts thay đổi, digest/version mới |
| A16 | Cùng version khác package digest | Validation error immutable version |
| A17 | Downgrade | Bị từ chối, không ghi |
| A18 | Invalid/cyclic/missing dependency | Lỗi giải thích được, không tạo output |
| A19 | Path traversal, absolute, drive, UNC, ADS | Bị chặn trước write |
| A20 | Symlink/junction/reparse trong source/target | Không thoát root; thông báo path cụ thể |
| A21 | Windows reserved names/case collision | Bị chặn trên mọi OS để package portable |
| A22 | UTF-8 tiếng Việt, path có spaces | CLI và artifact đúng encoding, không shell injection |
| A23 | LF/CRLF/BOM/no trailing newline | Bảo toàn bytes ngoài managed block |
| A24 | Duplicate/unclosed markers hoặc state missing | Conflict, không tự nhận ownership |
| A25 | `AGENTS.override.md` che cùng cấp | Cảnh báo/chặn theo contract, không sửa override |
| A26 | Disk full/permission failure/action thứ N lỗi | Không báo thành công; rollback hoặc recoverable journal |
| A27 | Crash trước/sau state commit | Recover phân biệt được before/after/committed |
| A28 | Editor sửa file trong lúc apply/rollback | Không overwrite thay đổi ngoài transaction |
| A29 | Hai tiến trình cùng scope | Chỉ một tiến trình giữ lock; tiến trình kia exit `3` |
| A30 | Stale lock/PID không xác định | Không xóa mù; recover báo rõ trạng thái |
| A31 | State/journal sửa path sang file khác | Validation chặn xóa/ghi ngoài allowlist |
| A32 | Không có Codex trên PATH | Install file được với warning; doctor readiness fail |
| A33 | Non-interactive mutation thiếu `--yes` | Exit `2`, không treo chờ stdin |
| A34 | `--json` error/success/cancel | Một JSON object hợp schema; exit code khớp |
| A35 | `--project --global` / global + path | Usage error trước I/O mutation |
| A36 | Global state trỏ Codex home cũ | Mutation chặn profile mismatch |
| A37 | Golden paths/resource references | Không còn template variable chưa render, link tới file tồn tại |
| A38 | Directory không có Git | Resolve CWD đúng, warning hướng dẫn scope |
| A39 | File Windows đang bị khóa | Lỗi được báo, file và state không bị bỏ dở không theo dõi |
| A40 | Future state/schema version | Chỉ báo unsupported, không reset state |
| A41 | Too-large package/target/state | Bị chặn có giới hạn bộ nhớ; target text cap 8 MiB, state/journal cap 4 MiB |
| A42 | Source local bị thay giữa validate/apply | Dùng snapshot content đã validate hoặc phát hiện digest đổi |
| A43 | Remove file `AGENTS.md` vốn tồn tại rỗng | Giữ file rỗng; không nhầm là DTKit-created |
| A44 | Remove toàn bộ rồi install lại | State clean, không còn ownership mồ côi; backup operational vẫn có thể còn |

Test junction/symlink có thể cần capability runner. Nếu skip phải ghi rõ lý do; không dùng skip để tuyên bố Windows path security đã pass. Fault injection cần bao gồm lỗi của rollback và lỗi commit state, không chỉ lỗi render.

### 17.3. Smoke test Codex thật

Tạo fixture repo nhỏ có code/test dễ hiểu, không chứa secret. Dùng tài khoản OS hoặc môi trường test tách biệt khi kiểm tra global; không giả định đổi `CODEX_HOME` cũng cô lập `~/.agents/skills`.

Quy trình:

1. Ghi Codex version, DTKit version/commit, OS, ngày và scope.
2. Cài project kit vào fixture, khởi động phiên Codex mới.
3. Xác nhận selector nhận các tên `dtkit-project-*`.
4. Gọi explicit `plan`, yêu cầu thêm một hành vi nhỏ; kiểm tra agent đọc source và tạo plan, code không đổi khi chưa yêu cầu implement.
5. Gọi `implement` với scope đã rõ; kiểm tra diff có trọng tâm và test results thật.
6. Gọi `review` trên một diff có bug được seed; kiểm tra finding có evidence.
7. Yêu cầu explain/diagnose-only; kiểm tra không sửa source ngoài yêu cầu.
8. Lặp lại cho global trên fixture thứ hai không cài project.
9. Cài cả hai scopes vào cùng môi trường; xác nhận tên khác nhau và lời gọi explicit chọn đúng bản.
10. Sửa rule trong fixture package và tăng version, update, mở phiên mới và kiểm tra nội dung mới.
11. Remove project, mở phiên mới: project skills biến mất, global và nội dung người dùng vẫn còn.
12. Lưu báo cáo có các file/skill thực sự được đọc, lệnh test thực sự chạy và kết quả; không chỉ dựa vào agent nói đã hiểu.

Nếu không có quyền chạy Codex/xác thực, giữ trạng thái `runtime verification pending`; vẫn hoàn thiện code và automated tests. Thiếu bằng chứng runtime là release gate cho tuyên bố hỗ trợ Codex, không phải lý do giả lập kết quả.

### 17.4. Benchmark chất lượng và chi phí

Dùng ít nhất ba task: feature nhỏ, bug có regression test, review diff có lỗi seed. Mỗi task chạy baseline và with-kit trên cùng commit, model/config và yêu cầu. Dùng phiên sạch, cố định dataset; chạy ba lần mỗi cấu hình nếu chi phí cho phép.

Chấm: correctness, phạm vi sửa, test coverage hành vi, bằng chứng review, thời gian và token nếu runtime cung cấp số liệu. Không tự suy ra token từ độ dài text như số liệu đo thật. Báo biến thiên giữa lần chạy; chưa công bố phần trăm cải thiện khi chưa đo.

## 18. Build, CI và phát hành

### 18.1. Build local

Sau M1, những lệnh sau phải hoạt động từ root source:

```powershell
go version
go mod download
go run ./cmd/dtkit --version
go test ./...
go vet ./...
gofmt -l .

New-Item -ItemType Directory -Force -Path .\dist | Out-Null
go build -trimpath -o .\dist\dtkit.exe ./cmd/dtkit
.\dist\dtkit.exe --version
```

`gofmt -l .` phải không có output; CI cần kiểm tra output chứ không chỉ exit code. Script build inject version/commit/date bằng `-ldflags` vào `internal/buildinfo`. Bản build chính thức dùng tag/source cố định; không lấy thời gian ngẫu nhiên làm input cho test/golden.

`scripts/verify.ps1` chạy format check, unit/integration tests, vet và CLI smoke offline; không cài kit vào repo nguồn hoặc home thật để kiểm tra.

### 18.2. CI

Pull request/push CI:

- Go version pin; restore cache dependency/build phù hợp, không cache credentials.
- Jobs Windows, Linux, macOS cho `go test ./...`, `go vet ./...`, build và CLI smoke.
- Race test ít nhất trên Linux runner có toolchain phù hợp; không bắt Windows race khi chưa đủ cgo/C compiler.
- Kiểm tra format, schema/content validation, golden snapshots và package digest deterministic.
- Binary distribution có thể build với `CGO_ENABLED=0`; race job là cấu hình riêng.
- Fuzz ngắn cho managed block parser/path validation sau M5; corpus regression lưu trong repo.
- Workflow permissions tối thiểu, pin actions theo commit SHA đã kiểm tra tại lúc triển khai.
- Không cần LLM credential để pass CI thường xuyên.

### 18.3. Artifacts release

Tạo archive theo mẫu:

```text
dtkit_0.1.0_windows_amd64.zip
dtkit_0.1.0_linux_amd64.tar.gz
dtkit_0.1.0_darwin_arm64.tar.gz
SHA256SUMS
```

Windows amd64 là artifact release bắt buộc đầu tiên. Chỉ publish/ghi supported cho OS/architecture có kiểm chứng phù hợp; cross-compile thành công không chứng minh runtime compatibility. Có thể thêm các architecture khác khi có test.

Mỗi archive chứa binary, README cài đặt ngắn, changelog và license đã được chủ dự án quyết định. Tạo SHA-256 cho archive, test giải nén vào thư mục tạm và chạy executable trên runner phù hợp. Checksum phát hiện hỏng dữ liệu nhưng không tự xác thực tác giả nếu binary và checksum cùng bị thay thế; signing/provenance nâng cấp sau.

### 18.4. Cài qua terminal

MVP phải có hướng dẫn PowerShell tải archive từ release cụ thể, kiểm tra SHA-256, giải nén và đặt executable trong thư mục user chọn. Khi chưa có repository thật, chỉ ghi hướng dẫn build local; không cung cấp URL giả như thể đang hoạt động.

Một lệnh `dtkit install engineer` chỉ hoạt động sau khi executable đã có trên PATH. Không mô tả nó như lệnh bootstrap cài chương trình từ Internet.

Installer script tiện lợi là phần sau release cơ bản. Nếu làm trong M9, script phải:

- Nhận version/repository rõ ràng, tải HTTPS, kiểm tra checksum trước sử dụng.
- Validate archive không có traversal/symlink bất thường.
- Cài trong user directory, không mặc định yêu cầu Administrator.
- Phát hiện binary đang chạy/bị khóa và có phương án thay phiên bản an toàn.
- Hiển thị thay đổi PATH; không ghi system PATH ngầm.
- Có uninstall binary riêng với remove kit; gỡ binary không tự xóa kit/config.

Không dùng mặc định `download | execute` cho script chưa được kiểm tra. Native plugin distribution của Codex có thể là kênh bổ sung tương lai; DTKit vẫn giữ package format chung để hỗ trợ nhiều runtime.

### 18.5. Public release gate

Trước khi publish cần xác nhận repository owner/URL, visibility và license từ chủ dự án. Việc chuẩn bị source, tests, archive và release notes thực hiện được trước đó. Không tự tạo remote, push, tag release public hoặc chọn license thay người dùng khi chưa có ủy quyền.

Git module path tạm phải được đổi sang repository thật; dependency/license inventory được kiểm tra; release notes ghi rõ v0.1 chưa có hook execution hoặc remote kit update.

## 19. Định nghĩa hoàn thành v0.1

Chỉ đánh dấu MVP core hoàn thành khi đáp ứng tất cả mục dưới đây hoặc nêu riêng release gate chưa có điều kiện thực hiện:

- [ ] Build từ clean checkout bằng tài liệu.
- [ ] Một binary tự chứa catalog; không cần Node.js/Python cho người dùng cuối.
- [ ] `list`, `info`, `validate`, `doctor`, `install`, `status`, `update`, `remove`, `recover` đúng contract.
- [ ] Cài được cả project và global, kể cả cùng tồn tại.
- [ ] Giữ nguyên nội dung cá nhân trong `AGENTS.md` và thư mục skills.
- [ ] Dry-run hoàn toàn không ghi filesystem đích.
- [ ] Conflict được phát hiện trước mutation; `--yes` không bỏ qua conflict.
- [ ] Install lặp lại là no-op; update version và adapter có test.
- [ ] Remove không xóa file chưa được quản lý hoặc file managed đã bị sửa.
- [ ] Crash recovery và rollback conflict có test bằng fault injection.
- [ ] Paths/encoding/spaces/Windows locked files được kiểm tra.
- [ ] Seven skills, rules, workflows, templates có nội dung hữu ích và liên kết đúng.
- [ ] Automated test suite đạt trên Windows; matrix còn lại có kết quả rõ.
- [ ] Codex CLI thật đọc instruction và skills, với evidence cho hai scopes.
- [ ] Có hướng dẫn terminal lấy binary/cài kit/update/remove/troubleshooting.
- [ ] Release archive/checksum được tạo và smoke test.
- [ ] Các tuyên bố runtime/OS support khớp evidence, không suy từ số file được tạo.

Tách trạng thái `MVP implemented and automated-tested` khỏi `Codex verified` và `publicly released` nếu đang thiếu tài khoản test hoặc repository phát hành. Không dùng từ completed cho toàn bộ sản phẩm khi một yêu cầu bắt buộc còn pending.

## 20. Milestone và backlog thực thi

Thứ tự dependency chính: `M0 → M1 → M2 → M3 → M4 → M5 → M6 → M7 → M8 → M9`.

M3 content có thể chạy song song phần M2 schema sau khi contract ổn định. Tests được viết trong từng milestone; M7 tập trung cross-platform/adversarial, không chờ đến M7 mới viết test.

### M0 — Audit và chuẩn bị môi trường

Đầu ra: `docs/development.md`, `docs/decisions.md`, `docs/progress.md` và môi trường build được xác định.

- [ ] Đọc plan và hai tài liệu nguồn, kiểm tra file hiện hữu/dirty worktree.
- [ ] Kiểm tra `go`, `git`, `codex`; nếu launcher Windows là `.cmd`/`.ps1`, xử lý bằng adapter process phù hợp, không coi là native exe.
- [ ] Xác minh Go toolchain, module version cho ba dependency đã chọn.
- [ ] Kiểm tra lại tài liệu Codex nếu bắt đầu sau ngày lập plan; ghi source/version/date.
- [ ] Khởi tạo Git local nếu chưa có khi bắt đầu triển khai source; chưa tạo remote hoặc commit tự động nếu không có yêu cầu.
- [ ] Chốt baseline runtime contract, module path tạm và danh sách quyết định đã ủy quyền.
- [ ] Lập `docs/progress.md` với milestone/task/test/blocker/next-step.

Acceptance: hiểu rõ điều kiện build/test, các lựa chọn không còn mơ hồ; không thay user config thật.

### M1 — CLI skeleton và test harness

Đầu ra: executable skeleton, buildinfo, command registration, helper chạy CLI trong test.

- [ ] Tạo `go.mod`, entry point, Cobra root, `version`, `help`.
- [ ] Dependency injection tối thiểu cho stdout/stderr, prompter, environment, clock.
- [ ] Error types/exit code mapper, JSON envelope và output schema.
- [ ] Context cancellation, `--json`, `--verbose`, scope flag validation.
- [ ] Scripts build/verify cơ bản và `.gitignore` cho `dist/`, coverage, operational temp.
- [ ] Tests version/help, bad flags, JSON stdout purity, cancelled command.

Acceptance: `go run ./cmd/dtkit --version`, `go test ./...`, `go vet ./...` chạy; command chưa implement không xuất thông báo thành công giả.

### M2 — Manifest, catalog, resolver và interface

Đầu ra: schemas, package loader, component DAG, catalog API, adapter types.

- [ ] Viết `package.schema.json`/state/output schema và schema embedding.
- [ ] Parse JSON strict, duplicate-key detection, semantic limits và path validation.
- [ ] Loader local directory qua `fs.FS`/safe traversal, snapshot file bytes cần render.
- [ ] Package digest ổn định và immutable version rule.
- [ ] Resolver DAG, deterministic ordering, cycle diagnostics.
- [ ] `list`, `info`, `validate` hoạt động trên fixture tối thiểu.
- [ ] Hoàn thiện `RuntimeAdapter`, `Artifact`, `Layout`, `Action`, environment types.
- [ ] Test malformed manifest, path escape, missing refs, duplicate components và cycles.

Acceptance: validate package đúng/sai có kết quả rõ, không tải schema/network, không tạo target files.

### M3 — Engineer Kit nội dung thật

Đầu ra: package `engineer@0.1.0` hoàn chỉnh và embed catalog.

- [ ] Viết bảy skill theo mục 8, không để `TODO` thay phần hướng dẫn chính.
- [ ] Viết engineering rules ngắn, không lặp toàn bộ skill vào global instruction.
- [ ] Viết ba workflows và ba templates; xác định link từ block/skills tới resources.
- [ ] Render tên theo scope và mô tả tránh trigger chồng lấn không cần thiết.
- [ ] Chỉ copy resource cần thiết vào output; giữ link tương đối project hợp lệ.
- [ ] Global resource links được resolve theo layout người dùng, không nhúng home máy tác giả.
- [ ] Validate frontmatter và mọi reference sau render cả hai scopes.
- [ ] Content tests cho prompt plan-only/debug-only/review-only và output expectations.

Acceptance: `dtkit validate ./catalog/kits/engineer` pass; bảy skills có thể đọc độc lập; chưa tuyên bố agent thật đã tuân thủ.

### M4 — Codex adapter và read-only planner

Đầu ra: layouts, rendered artifacts, file diff/plan và golden snapshots.

- [ ] Resolve project root/Codex home/user home đúng mục 10.
- [ ] Render skill names `dtkit-project-*` / `dtkit-global-*` và references.
- [ ] Managed block parser giữ byte ngoài block, xử lý BOM/CRLF.
- [ ] Planner so sánh state/disk/desired theo bảng quyết định.
- [ ] Kiểm tra override, target ownership, modified/missing files và root restrictions.
- [ ] In action preview có reason codes; JSON có summary khớp actions.
- [ ] `install --dry-run`, `update --dry-run`, `remove --dry-run` dùng fixture state.
- [ ] Test snapshot filesystem chứng minh dry-run không tạo `.dtkit` hay lock.

Acceptance: golden cho hai scopes, preexisting `AGENTS.md`, drift/collision pass; mọi conflict được liệt kê trước ghi.

### M5 — Installer, state và recover

Đầu ra: write engine dùng chung, journal và recovery thực thi được.

- [ ] State loader/validator/generation/root binding và ownership metadata.
- [ ] Lock có owner token; không dùng lock timeout làm lý do xóa mù.
- [ ] Preflight + revalidation sau xác nhận.
- [ ] Temp file/replace per-platform, preserve mode và error handling.
- [ ] Journal before/after bytes, digests, phase và commit state.
- [ ] Conditional rollback và `recover` command.
- [ ] Crash/fault injection ở từng boundary quan trọng.
- [ ] Chỉ prune directories empty và do DTKit tạo.

Acceptance: tests write failure/crash/modified-during-rollback/concurrent installer pass; không có đường code recursive-delete parent shared directory.

### M6 — Hoàn thiện trải nghiệm CLI end-to-end

Đầu ra: toàn bộ command v0.1 được nối vào engine.

- [ ] `install` mới, install idempotent, prompt và `--yes`.
- [ ] `update` version mới, same-version mismatch, adapter migration.
- [ ] `remove` dựa vào state và giữ unknown files.
- [ ] `status` single/both scope, healthy/drift/missing/recovery.
- [ ] `doctor` checks, warnings và readiness exit codes.
- [ ] Non-interactive/JSON errors có hint khắc phục, không stack trace mặc định.
- [ ] Kiểm tra install/update/remove trong path Windows có Unicode/spaces.
- [ ] Smoke offline từ executable built, không chỉ gọi internal functions.

Acceptance: luồng ở mục 3 chạy được trong sandbox; output dễ hiểu và không ghi home thật.

### M7 — Hardening và CI

Đầu ra: test matrix A01–A44, CI và báo cáo giới hạn platform.

- [ ] Hoàn thiện tests còn thiếu, tập trung path/state corruption và recovery.
- [ ] Chạy Windows/Linux/macOS native jobs; document skipped capabilities.
- [ ] Windows junction/reparse, locked file, long path behavior, case collision.
- [ ] POSIX permissions, symlink và replace semantics.
- [ ] Race test và fuzz parser/path logic có thời gian hữu hạn.
- [ ] Audit logs/output không lộ nội dung file hoặc secrets.
- [ ] Kiểm tra deterministic artifacts và source snapshot chống package thay trong lúc apply.

Acceptance: CI pass, không còn lỗi mất dữ liệu đã biết; limitations có test/evidence rõ ràng.

### M8 — Kiểm chứng Codex và đánh giá nội dung

Đầu ra: `docs/verification/codex-<version>-<date>.md`, compatibility matrix.

- [ ] Làm smoke test đầy đủ mục 17.3 với phiên mới và môi trường cô lập.
- [ ] Kiểm tra skill list/path thực, không chỉ hỏi agent có thấy kit không.
- [ ] Kiểm tra global và project cùng tồn tại, không bị trùng tên.
- [ ] Đánh giá plan-only, review-only, debug-only và implementation scope.
- [ ] Benchmark baseline/with-kit ở quy mô nhỏ, ghi rõ sample size.
- [ ] Sửa nội dung hoặc adapter nếu phát hiện load/trigger sai; tăng version nếu nội dung đã được phát hành.
- [ ] Chạy lại tests bị ảnh hưởng và cập nhật golden có review.

Acceptance: có evidence Codex sử dụng thật trên hai scopes; nếu thiếu access thì giữ `pending` rõ ràng, không claim runtime verified.

### M9 — Tài liệu, đóng gói và release readiness

Đầu ra: README, CLI docs, archive/checksum, release notes và checklist publish.

- [ ] Hướng dẫn build/cài binary/cài kit/gọi skills/update/remove/recover.
- [ ] Tài liệu khác biệt scope, shared paths, conflict và `CODEX_HOME`.
- [ ] Package format/architecture/development docs khớp code thật.
- [ ] Build archive, generate checksum, extract/run smoke.
- [ ] CI release workflow chuẩn bị ở trạng thái có thể review; không tự publish.
- [ ] Ghi rõ features đã triển khai/chưa có, verified OS/runtime versions.
- [ ] Khi có repository owner/license, thay module path tạm và hoàn thiện release metadata.
- [ ] Có xác nhận phát hành rồi mới push/tag/upload public theo phạm vi được cho phép.

Acceptance: người khác theo README có thể lấy binary và cài kit; mọi link công khai là thật, không có `OWNER/REPO` giả trong bản phát hành.

### M10 — Hooks v0.2

Dependency: M5/M7/M8 và tài liệu hook theo runtime version hiện tại.

- [ ] Schema v2 + native mapping event/protocol + migration state.
- [ ] Built-in `session-context` handler, stdin limits và unit tests.
- [ ] Structural merge native hook JSON với ownership digest.
- [ ] Stable executable detection và Windows quoting tests.
- [ ] `hooks enable/disable/status/list`, trust-pending report.
- [ ] Real runtime test configured → trust review → invoked → disabled.
- [ ] Kiểm tra duplicate execution khi có global/project; không cho silent double-run.
- [ ] Update/review lại hook và uninstall bảo toàn entries người dùng.

Acceptance: hook chạy thực, gỡ sạch phần DTKit, không bypass trust, không làm phiên agent mắc vòng lặp.

### M11 — Package từ xa và tác giả package

Dependency: core installer ổn định và có nhu cầu phân phối kit độc lập.

- [ ] `dtkit pack <dir>` tạo archive deterministic từ package validate.
- [ ] `dtkit install <id>@<version> --source <url>` thiết kế contract trước khi code.
- [ ] Digest pin trong lock; archive size/file count/decompression ratio limits.
- [ ] Reject path traversal, symlink/hardlink/device entries khi extract.
- [ ] Download vào temp/cache, verify trước khi đưa vào installer hiện hữu.
- [ ] Cache key theo digest, atomic download, interrupted download cleanup có scope.
- [ ] Update tra version remote có explicit network behavior/offline mode.
- [ ] Version resolution/minimum CLI/schema compatibility rõ ràng.
- [ ] Tác giả có `create kit`, `create skill`, `validate`, `pack`; chưa cần marketplace.

Acceptance: package được phát hành độc lập với CLI, cài/reinstall offline bằng lock digest, không chạy installer script từ package.

### M12 — Gemini CLI, OpenCode, Cursor

Làm tuần tự mỗi runtime. Không dùng mapping đường dẫn của Codex để suy ra runtime khác.

- [ ] Tra tài liệu chính thức và pin phiên bản runtime test.
- [ ] Lập capability matrix instructions/skills/hooks/agents/plugin theo scope.
- [ ] Xác định file cấu hình, precedence, trust và loading/reload behavior.
- [ ] Thiết kế coexistence/ownership khi dùng chung `.agents/skills` hoặc `AGENTS.md`.
- [ ] Implement adapter render/layout; reuse planner/installer.
- [ ] Golden tests, conflict/removal tests và runtime smoke cho project/global.
- [ ] Chỉ mở runtime ID trong CLI sau khi acceptance pass.

Acceptance: cùng package engineer được dùng thật trên runtime mới, có fallback rõ cho capability không có; không chỉ tạo thư mục đúng tên.

## 21. Các user stories để kiểm tra sản phẩm

### US1 — Lập trình viên dùng một repository

Tôi tải DTKit, xem Engineer Kit, dry-run rồi cài vào project. Khi mở Codex, tôi dùng skill plan và agent đọc cấu trúc source trước khi đề xuất thay đổi. Tôi biết file nào được tạo và có thể gỡ phần DTKit.

Acceptance liên quan: A01/A05/A06/A07, smoke project.

### US2 — Dùng chung trên nhiều repository

Tôi cài global một lần. Repository không có project kit vẫn dùng được global skills. Repository có project kit nhận tên skill riêng; không có việc update project tự thay global installation.

Acceptance: A02/A03/A04/A36, smoke global + combined.

### US3 — Người dùng đã có instruction riêng

Tôi đã viết `AGENTS.md` và có skills cá nhân. Cài DTKit thêm đúng vùng nội dung của nó; phần cá nhân không biến mất. Khi tôi sửa ngoài vùng managed, update vẫn giữ phần đó.

Acceptance: A07–A12/A23/A24/A43.

### US4 — Upgrade có conflict

Tôi đã sửa một skill của DTKit. Khi update, CLI cho biết file nào khác bản đã cài và không ghi đè. Tôi có thể sao chép custom changes sang skill riêng hoặc xử lý thủ công trước khi chạy lại.

Acceptance: A09/A11/A15/A16. `--yes` không được biến conflict thành overwrite.

### US5 — Cài đặt bị ngắt

Tiến trình bị ngắt khi đang cập nhật. Lần chạy sau báo transaction chưa hoàn tất và có lệnh recover. Recover không ghi đè các chỉnh sửa mới của tôi.

Acceptance: A26–A31.

### US6 — Tác giả kit chuẩn bị chia sẻ

Tôi có package local theo schema và chạy validate. Tôi cài thử ở project trước khi đưa vào bundled catalog. Giai đoạn sau tôi đóng gói/phát hành version độc lập mà không cần viết lại installer.

Acceptance v0.1: validate/local source/version tests. Remote distribution thuộc M11.

## 22. Những quyết định không được suy diễn khi triển khai

| Tình huống | Cách xử lý đã chốt |
|---|---|
| Runtime hỗ trợ một feature | Chưa đủ để công bố DTKit hỗ trợ feature đó |
| File được tạo đúng đường dẫn | Chưa đủ để công bố agent đã đọc/sử dụng |
| Có skills project và global | Không giả định merge/override theo tên; dùng namespace scope |
| Có instruction global | Không có quyền ghi đè project instructions hoặc policy runtime |
| Task chỉ yêu cầu plan/review/diagnose | Nội dung kit phải giữ scope đó |
| Package cần hook không có trong adapter | Báo unsupported, không âm thầm bỏ qua required component |
| `--yes` | Chỉ bỏ xác nhận CLI, không bỏ kiểm tra hoặc grant quyền runtime |
| Metadata có path | Vẫn phải validate path/ownership/digest trước mọi write/delete |
| Người dùng sửa file của kit | Conflict, không tự merge toàn file bằng AI |
| Build nhiều OS thành công | Chưa chứng minh install/runtime hoạt động trên mọi OS |
| Tài liệu cũ đề xuất công cụ bên thứ ba | Không tự cài tất cả; integration sau khi có nhu cầu |
| Package/CLI dùng cùng version lúc đầu | Vẫn giữ version fields độc lập |
| Chưa có remote repository | Có thể build/test/archive local, chưa publish |

## 23. Rủi ro chính và cách giảm

| Rủi ro | Tác động | Giảm rủi ro bằng |
|---|---|---|
| Codex thay loading path/format | Kit cài xong không được nhận | Adapter revision, official docs, versioned smoke evidence |
| Ghi đè config cá nhân | Mất dữ liệu/niềm tin | Managed block, digest, preview, refusal on conflict |
| Crash nhiều file | State và disk lệch | Journal, conditional recovery, fault injection |
| Project/global cùng tồn tại | Chọn nhầm skill/trùng context | Scope namespace, explicit examples, combined smoke |
| Skills quá dài/trùng lặp | Tốn context, trigger sai | Small instructions, narrow descriptions, content evaluation |
| Hook thiếu trust hoặc sai quoting | Không chạy hoặc chạy sai | Separate milestone, native protocol, Windows tests |
| Remote package không đáng tin | Supply-chain/command execution | Deferred remote, digest pin, no install scripts, package review |
| CLI quá nhiều abstractions | Chậm có sản phẩm dùng được | Một Go module, một kit, một adapter, stdlib-first |
| Tuyên bố tương thích quá sớm | Người dùng cài nhưng không dùng được | Separate installed/tested/verified/released states |

## 24. Hướng dẫn bàn giao cho AI triển khai

### 24.1. Prompt khởi động có thể dùng nguyên văn

```text
Hãy triển khai DTKit theo DTKIT-IMPLEMENTATION-PLAN.md trong repository này.

Đọc toàn bộ kế hoạch và hai tài liệu nguồn trước khi bắt đầu.
Các quyết định đã chốt: Go, CLI dtkit, Codex-first, cả project và global.
Thực hiện M0 đến M9 theo dependency và acceptance criteria.
Chưa triển khai M10–M12 nếu chưa có yêu cầu mở rộng sang các phiên bản đó.

Audit trạng thái thực tế, bảo toàn thay đổi hiện hữu, rồi triển khai code và
tests từng phần. Không hỏi lại lựa chọn đã chốt. Ghi tiến độ/evidence vào
docs/progress.md để có thể tiếp tục sau khi đổi phiên.

Kiểm thử global bằng môi trường cô lập. Không thay cấu hình Codex cá nhân,
push hoặc phát hành public nếu chưa được cho phép thực hiện hành động đó.
Nếu thiếu Go/Codex/quyền test runtime, báo chính xác điều kiện còn thiếu,
tiếp tục phần độc lập và không giả lập kết quả.

Cuối mỗi milestone ghi files changed, commands run, actual results,
known limitations và next step. Chỉ đánh dấu hoàn thành khi có bằng chứng.
```

### 24.2. Progress record

Mỗi record trong `docs/progress.md`:

```markdown
## Mx — Milestone name

Status: not-started | in-progress | blocked | implemented | verified

- Completed tasks:
- Main files:
- Verification commands and actual results:
- Runtime/OS/version evidence:
- Known limitations:
- Decisions made and reason:
- Next concrete task:
```

Một milestone code có thể `verified` bằng automated tests; M8 chỉ `verified` khi có runtime smoke thật. Public release tracked riêng, không biến thiếu GitHub URL thành blocker cho M1–M7.

### 24.3. Khi tiếp tục sau phiên bị ngắt

1. Đọc `docs/progress.md`, kiểm tra filesystem và Git diff hiện tại.
2. Xác định milestone đang làm và test cuối cùng đã chạy; không tin checklist nếu code không khớp.
3. Nếu có installer integration fixture dở dang, chỉ cleanup bên trong test temp được xác định, không xóa home hoặc workspace.
4. Chạy lại kiểm tra liên quan khi có thay đổi mới/chưa chắc kết quả; tránh lặp full suite không cần thiết.
5. Tiếp tục task nhỏ kế tiếp; không dựng lại toàn bộ cấu trúc từ đầu.

## 25. Thông tin còn cần trước từng thời điểm

| Thông tin/điều kiện | Khi nào cần | Có chặn viết code không? |
|---|---|---|
| Go toolchain hoạt động | M1 build/test | Chặn kiểm chứng build; không chặn viết thiết kế/content |
| Codex CLI + tài khoản test | M8 runtime smoke | Không chặn unit/integration tests |
| Môi trường global cô lập | M8 | Không được thay bằng chỉnh home thật ngầm |
| GitHub owner/repository URL | M9 public release | Không chặn development/local archive |
| Public/private và license | Trước publish | Không tự quyết quyền phân phối thay chủ dự án |
| Runtime version cụ thể đã test | M8 và mỗi adapter mới | Không được điền bằng suy đoán |

Không cần hỏi thêm về tên CLI, ngôn ngữ, runtime đầu tiên hoặc project/global: đã có quyết định.

## 26. Nguồn và thời điểm xác minh

Tra cứu ngày 2026-11-16. Nguồn chính thức được dùng để xác định behavior runtime; thiết kế sản phẩm DTKit trong tài liệu là đề xuất triển khai riêng.

1. [Codex manual](https://developers.openai.com/codex/codex-manual.md) — bản tổng hợp đã tải để đối chiếu customization và config.
2. [Codex skills](https://learn.chatgpt.com/docs/build-skills) — discovery path, explicit/implicit invocation, tên skill trùng nhau.
3. [Codex AGENTS.md](https://learn.chatgpt.com/docs/agent-configuration/agents-md) — global/project instructions, override và discovery.
4. [Codex advanced configuration](https://learn.chatgpt.com/docs/config-file/config-advanced) — config layers và hooks.
5. [Codex hooks](https://learn.chatgpt.com/docs/hooks) — lifecycle, trust và native protocol; kiểm tra lại khi làm M10.
6. [OpenAI plugin packaging](https://developers.openai.com/plugins/build/plugins) — kênh phân phối native để khảo sát sau; không phải dependency v0.1.
7. [Agent Skills specification](https://agentskills.io/specification) — cấu trúc skill và frontmatter.
8. [Go release history](https://go.dev/doc/devel/release) — baseline toolchain tại ngày lập kế hoạch.
9. [Cobra](https://github.com/spf13/cobra) — CLI parser.
10. [YAML organization Go library](https://github.com/yaml/go-yaml) — parser frontmatter.
11. [JSON Schema validator for Go](https://github.com/santhosh-tekuri/jsonschema) — validate schema.

Khi tài liệu mới khác kế hoạch: xác định thay đổi nào là behavior runtime, cập nhật adapter/compatibility tests và ghi quyết định. Không dùng tài liệu cũ để khẳng định feature vẫn hoạt động nếu runtime smoke cho kết quả khác.

