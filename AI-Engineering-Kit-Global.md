# AI Engineering Kit Global

## Thiết kế bộ công cụ AI Coding dùng chung cho Claude Code, OpenAI Codex và Gemini CLI

**Mục tiêu:** Xây dựng một bộ quy chuẩn engineering ở phạm vi global
trên Windows, có thể dùng với nhiều AI coding agent. Bộ kit hướng tới ba
mục tiêu cân bằng: chất lượng code, mức độ tự chủ của agent và hiệu quả
sử dụng context/token.

------------------------------------------------------------------------

# 1. Kiến trúc tổng thể

Bộ kit gồm một phần dùng chung và các adapter nhỏ cho từng agent.

``` text
                    AI Engineering Kit
                             |
             +---------------+---------------+
             |               |               |
          Codex           Claude           Gemini
        AGENTS.md        CLAUDE.md        GEMINI.md
             |               |               |
             +---------------+---------------+
                             |
                 Shared Engineering Core
                             |
       +---------------------+----------------------+
       |                     |                      |
   Instructions            Skills                Workflows
       |                     |                      |
       +---------------------+----------------------+
                             |
                 Project Repository
       Spring Boot / React / Angular / Node.js / Python
```

Ba agent có cơ chế instruction riêng:

  Agent          Global instruction
  -------------- -----------------------
  OpenAI Codex   `~/.codex/AGENTS.md`
  Claude Code    `~/.claude/CLAUDE.md`
  Gemini CLI     `~/.gemini/GEMINI.md`

Trên Windows, `~` thường tương ứng với thư mục người dùng, ví dụ
`C:\Users\DELL`.

Nên duy trì một file nội dung chuẩn trong AI Engineering Kit, sau đó
đồng bộ nội dung sang ba file native. Cách này tránh việc ba agent dần
sử dụng các quy tắc khác nhau.

> Lưu ý: Ba agent có thể có cơ chế nạp instruction, skills, plugins và
> MCP khác nhau. Việc đồng bộ nội dung không đồng nghĩa mọi tính năng
> đều hoạt động giống hệt nhau trên cả ba nền tảng.

------------------------------------------------------------------------

# 2. Danh sách repository và công cụ đề xuất

Không cần cài tất cả ngay từ đầu. Nên bắt đầu với instruction chung,
skill manager và workflow; sau đó mới bổ sung công cụ phân tích
codebase.

## 2.1. Core Engineering

### 1. Vercel Skills CLI

-   Repository: https://github.com/vercel-labs/skills
-   Mục đích: Quản lý và cài đặt Agent Skills cho các coding agent tương
    thích.
-   Vai trò trong bộ kit: Cài đặt, liệt kê và cập nhật skills ở phạm vi
    global hoặc project.
-   Ghi chú: Kiểm tra danh sách agent được hỗ trợ và cú pháp CLI ở phiên
    bản đang cài bằng `npx skills --help`.

### 2. Superpowers

-   Repository: https://github.com/obra/superpowers
-   Mục đích: Cung cấp workflow phát triển phần mềm có cấu trúc.
-   Các hoạt động tiêu biểu:
    -   Brainstorm và làm rõ yêu cầu.
    -   Thiết kế giải pháp.
    -   Lập implementation plan.
    -   Chia nhỏ công việc.
    -   Test-driven development (TDD).
    -   Code review.
-   Ghi chú: Cài đặt và kích hoạt theo hướng dẫn riêng của từng agent.
    Không giả định cài cho một agent sẽ tự động kích hoạt trên các agent
    còn lại.

### 3. Vercel Agent Skills

-   Repository: https://github.com/vercel-labs/agent-skills
-   Mục đích: Bộ skill tham khảo cho frontend, web development và các
    tác vụ liên quan.
-   Vai trò: Chọn lọc những skill thực sự phù hợp với stack của dự án.

## 2.2. Codebase Intelligence

### 4. Repomix

-   Repository: https://github.com/yamadashy/repomix
-   Mục đích: Đóng gói source code thành context để LLM phân tích.
-   Có thể dùng khi:
    -   Phân tích tổng quan repository.
    -   Review kiến trúc.
    -   Tìm quan hệ giữa các module.
    -   Cung cấp context cho một lần phân tích rộng.
-   Lưu ý:
    -   Không cần đóng gói toàn bộ repository cho mọi task nhỏ.
    -   Loại trừ `.git`, `node_modules`, `target`, file build, secrets
        và dữ liệu không cần thiết.
    -   Kiểm tra output trước khi gửi cho dịch vụ AI.

### 5. Graphify

-   Repository: https://github.com/Graphify-Labs/graphify
-   Mục đích: Tạo knowledge graph để biểu diễn quan hệ giữa các thành
    phần trong codebase.
-   Phù hợp để thử nghiệm với dự án nhiều module hoặc dependency phức
    tạp.
-   Đây là thành phần tùy chọn; chỉ giữ lại nếu kết quả truy vấn thực sự
    hữu ích.

## 2.3. Skills và tài liệu tham khảo

  --------------------------------------------------------------------------
  Nguồn                                  Vai trò
  -------------------------------------- -----------------------------------
  https://github.com/anthropics/skills   Tham khảo cách thiết kế và đóng gói
                                         skills

  https://github.com/openai/skills       Tham khảo skills và tài nguyên cho
                                         hệ sinh thái OpenAI

  https://agentskills.io                 Tài liệu về định dạng Agent Skills

  https://context7.com                   Tra cứu tài liệu thư viện/framework
                                         theo phiên bản
  --------------------------------------------------------------------------

**Nguyên tắc lựa chọn:** Ưu tiên skill có nội dung rõ ràng, phạm vi hẹp,
có hướng dẫn sử dụng cụ thể và không trùng lặp. Không cài hàng loạt chỉ
vì repository có nhiều skill.

------------------------------------------------------------------------

# 3. Cấu trúc thư mục Global

Đường dẫn đề xuất:

``` text
%USERPROFILE%\\.ai-engineering-kit\\
```

Ví dụ:

``` text
C:\\Users\\DELL\\.ai-engineering-kit\\
```

Cấu trúc:

``` text
.ai-engineering-kit/
│
├── core/
│   ├── GLOBAL.md
│   ├── engineering-principles.md
│   ├── coding-standards.md
│   ├── security.md
│   ├── testing.md
│   └── performance.md
│
├── skills/
│   ├── repository-analysis/
│   │   └── SKILL.md
│   ├── feature-development/
│   │   └── SKILL.md
│   ├── debugging/
│   │   └── SKILL.md
│   ├── code-review/
│   │   └── SKILL.md
│   ├── refactoring/
│   │   └── SKILL.md
│   ├── test-engineering/
│   │   └── SKILL.md
│   ├── security-audit/
│   │   └── SKILL.md
│   └── documentation/
│       └── SKILL.md
│
├── workflows/
│   ├── new-feature.md
│   ├── bug-fix.md
│   ├── refactoring.md
│   ├── code-review.md
│   └── release.md
│
├── templates/
│   ├── implementation-plan.md
│   ├── technical-design.md
│   └── code-review-report.md
│
├── scripts/
│   ├── sync-instructions.ps1
│   ├── doctor.ps1
│   └── update-skills.ps1
│
├── config/
│   ├── mcp-servers.md
│   └── tool-policy.md
│
└── README.md
```

Các thư mục cấu hình native của agent vẫn được giữ riêng:

``` text
%USERPROFILE%\\
│
├── .codex\\
│   └── AGENTS.md
│
├── .claude\\
│   ├── CLAUDE.md
│   └── settings.json
│
├── .gemini\\
│   └── GEMINI.md
│
└── .agents\\
    └── skills\\
```

Không xóa hoặc ghi đè toàn bộ cấu hình hiện có. Trước khi tạo hay cập
nhật file native, cần kiểm tra file đã tồn tại và sao lưu nếu có nội
dung do người dùng tự viết.

------------------------------------------------------------------------

# 4. Global Engineering Principles

Bộ quy tắc cốt lõi gồm 8 nguyên tắc.

1.  **Understand Before Coding**\
    Đọc repository, tài liệu, cấu trúc và code liên quan trước khi chỉnh
    sửa.

2.  **Respect Existing Architecture**\
    Không tự ý thay đổi kiến trúc hoặc thêm framework khi chưa có lý do
    rõ ràng.

3.  **Plan Before Implementation**\
    Task lớn cần có phân tích và implementation plan trước khi code.

4.  **Minimal Correct Changes**\
    Chỉ thay đổi phần cần thiết; không refactor lan man.

5.  **Test What You Change**\
    Chạy test phù hợp và kiểm tra nguy cơ regression.

6.  **Security By Default**\
    Không hardcode secrets; chú ý authentication, authorization,
    validation và quyền truy cập.

7.  **Evidence-Based Completion**\
    Không tuyên bố hoàn thành nếu chưa có bằng chứng về build/test. Nêu
    rõ các bước chưa chạy.

8.  **Optimize Context and Cost**\
    Chỉ đọc context, gọi skill và sử dụng công cụ cần thiết cho task.

## 4.1. Nội dung GLOBAL.md mẫu

``` markdown
# Global AI Engineering Standards

## 1. Core Behavior

Act as a senior software engineer.
Prioritize correctness, maintainability, security,
and simplicity over implementation speed.

Never assume project architecture without inspecting
the existing codebase.

## 2. Repository Understanding

Before modifying code:

- Inspect the project structure.
- Read README and relevant documentation.
- Identify framework, dependencies and versions.
- Trace the existing implementation.
- Identify related tests and conventions.
- Never invent APIs, classes, methods or dependencies.

Do not scan unrelated directories unnecessarily.

## 3. Requirement Analysis

For every task:

- Identify the actual user requirement.
- Separate functional and non-functional requirements.
- Identify edge cases and potential regressions.
- Ask questions when essential information is missing.
- Explicitly state assumptions.

## 4. Planning

For small tasks:
- Make a concise execution plan.

For medium and large tasks:
- Analyze the architecture.
- Propose a technical design.
- List files to be changed.
- Identify risks and test cases.
- Obtain approval for significant architectural changes.

## 5. Implementation

- Follow existing project conventions.
- Prefer minimal, focused changes.
- Apply SOLID where appropriate.
- Avoid unnecessary abstractions.
- Avoid duplicated business logic.
- Do not silently change public APIs.
- Do not modify unrelated files.
- Do not add dependencies without justification.

## 6. Security

- Never expose API keys, passwords or secrets.
- Validate untrusted input.
- Apply authentication and authorization correctly.
- Follow least-privilege principles.
- Check common security vulnerabilities.
- Never disable security controls just to make tests pass.

## 7. Testing

After implementation:

- Run relevant unit tests.
- Run integration tests when applicable.
- Run build and static checks.
- Review changed files.
- Report failed or skipped checks honestly.

Never claim tests passed without executing them.

## 8. Completion Report

Always report:

1. What was changed.
2. Which files were modified.
3. Important implementation decisions.
4. Tests and build results.
5. Known limitations and remaining risks.

## 9. Efficiency

- Read only relevant files.
- Avoid repeatedly scanning the entire repository.
- Use skills only when relevant.
- Prefer targeted code searches.
- Do not generate excessive documentation.
- Do not over-engineer simple tasks.

## 10. Conflict Resolution

Follow this priority:

1. Explicit user requirements.
2. Project-specific instructions.
3. Global engineering standards.
4. General engineering preferences.

Never override project-specific constraints silently.
```

Đây là nội dung nền tảng. Khi triển khai thực tế, nên giữ file global
ngắn và chuyển hướng dẫn chuyên sâu sang skills hoặc tài liệu riêng.

------------------------------------------------------------------------

# 5. Thiết kế Skill System

Không nên đưa toàn bộ kiến thức Spring Boot, React, Angular hay Python
vào `GLOBAL.md`.

Global instruction chỉ cần nêu nguyên tắc và hướng dẫn agent sử dụng
skill khi phù hợp.

## 5.1. Danh sách 8 skill cốt lõi

  -----------------------------------------------------------------------
  Skill                               Khi nào sử dụng
  ----------------------------------- -----------------------------------
  `repository-analysis`               Khi bắt đầu làm việc với repository
                                      hoặc cần hiểu kiến trúc

  `feature-development`               Khi phát triển chức năng mới

  `debugging`                         Khi tìm nguyên nhân và xử lý bug

  `code-review`                       Khi review thay đổi

  `refactoring`                       Khi cải thiện code hiện tại

  `test-engineering`                  Khi viết hoặc sửa test

  `security-audit`                    Khi kiểm tra bảo mật

  `documentation`                     Khi viết tài liệu kỹ thuật
  -----------------------------------------------------------------------

## 5.2. Ví dụ skill feature-development

``` markdown
---
name: feature-development
description: Analyze, design, implement and verify software features while respecting the existing project architecture. Use when implementing a new feature or changing existing business behavior.
---

# Feature Development

## Step 1: Understand

- Read the relevant documentation.
- Inspect existing implementation.
- Identify related modules and dependencies.
- Understand current business behavior.

## Step 2: Analyze

- Define expected behavior.
- Identify edge cases.
- Identify affected components.
- Check backward compatibility.

## Step 3: Design

- Propose implementation approach.
- Identify affected files.
- Define API and data changes if needed.
- Explain architectural trade-offs.

## Step 4: Implement

- Follow existing conventions.
- Keep changes minimal.
- Avoid unnecessary dependencies.
- Preserve existing functionality.

## Step 5: Verify

- Add or update tests.
- Run relevant tests.
- Run build and static checks.
- Review the final diff.

## Step 6: Report

Explain implementation, test results,
limitations and potential risks.
```

Đây là skill mẫu tự thiết kế, không phải bản sao của skill từ repository
bên ngoài.

------------------------------------------------------------------------

# 6. Hướng dẫn cài đặt trên Windows

## Bước 1. Kiểm tra môi trường

Mở PowerShell:

``` powershell
node -v
npm -v
git --version

codex --version
claude --version
gemini --version
```

Nếu chưa có Node.js, cài phiên bản đáp ứng yêu cầu của Skills CLI. Kiểm
tra yêu cầu phiên bản hiện tại trong tài liệu của công cụ.

Kiểm tra thư mục Home:

``` powershell
$HOME
```

## Bước 2. Tạo cấu trúc thư mục

``` powershell
$Kit = "$HOME\.ai-engineering-kit"

$Dirs = @(
    "core",
    "skills",
    "workflows",
    "templates",
    "scripts",
    "config"
)

New-Item -ItemType Directory -Force -Path $Kit | Out-Null

foreach ($Dir in $Dirs) {
    New-Item -ItemType Directory -Force `
        -Path "$Kit\$Dir" | Out-Null
}

Write-Host "AI Engineering Kit created: $Kit"
```

## Bước 3. Tạo instruction chuẩn

Tạo file:

``` text
%USERPROFILE%\\.ai-engineering-kit\\core\\GLOBAL.md
```

Đưa nội dung GLOBAL.md mẫu ở phần 4 vào file này.

## Bước 4. Đồng bộ instruction sang ba agent

Sau khi đã có `GLOBAL.md`, chạy:

``` powershell
$Kit = "$HOME\.ai-engineering-kit"
$Source = "$Kit\core\GLOBAL.md"

$Targets = @(
    "$HOME\.codex\AGENTS.md",
    "$HOME\.claude\CLAUDE.md",
    "$HOME\.gemini\GEMINI.md"
)

foreach ($Target in $Targets) {
    $Parent = Split-Path $Target -Parent

    New-Item -ItemType Directory -Force `
        -Path $Parent | Out-Null

    if (Test-Path $Target) {
        $Backup = "$Target.backup-$(Get-Date -Format 'yyyyMMdd-HHmmss')"
        Copy-Item $Target $Backup
        Write-Host "Backup created: $Backup"
    }

    Copy-Item $Source $Target -Force
    Write-Host "Synced: $Target"
}
```

Nên lưu đoạn này thành:

``` text
%USERPROFILE%\\.ai-engineering-kit\\scripts\\sync-instructions.ps1
```

Sau mỗi lần chỉnh sửa `GLOBAL.md`, chạy lại script để đồng bộ.

**Lưu ý:** Đây là cơ chế sao chép. Nó không tự đồng bộ khi file nguồn
thay đổi.

## Bước 5. Cài Skills CLI

Kiểm tra CLI:

``` powershell
npx skills --help
```

Tham khảo repository:

https://github.com/vercel-labs/skills

Ví dụ cài skills từ một nguồn:

``` powershell
npx skills add vercel-labs/agent-skills -g
```

Sau đó xem các tùy chọn agent và phạm vi cài đặt được phiên bản CLI hỗ
trợ:

``` powershell
npx skills add --help
npx skills list --help
```

Khi đã xác nhận cú pháp và agent identifiers, mới chỉ định các agent cần
cài. Không giả định tên agent hoặc flag giống nhau giữa các phiên bản
CLI.

Kiểm tra và cập nhật:

``` powershell
npx skills list
npx skills check
npx skills update
```

## Bước 6. Cài Superpowers

Repository:

https://github.com/obra/superpowers

Với Claude Code, tham khảo hướng dẫn cài đặt hiện hành của Superpowers.
Ví dụ các lệnh marketplace được tài liệu dự án hướng dẫn có thể có dạng:

``` text
/plugin marketplace add obra/superpowers-marketplace
/plugin install superpowers@superpowers-marketplace
```

Với Codex và Gemini CLI, thực hiện theo hướng dẫn tương ứng trong
repository. Không coi việc cài plugin cho Claude là đã cài cho hai agent
còn lại.

------------------------------------------------------------------------

# 7. Codebase Intelligence

## 7.1. Repomix

Repository:

https://github.com/yamadashy/repomix

Kiểm tra:

``` powershell
npx repomix --version
```

Di chuyển vào project:

``` powershell
cd C:\path\to\your-project
```

Đóng gói repository theo cấu hình phù hợp:

``` powershell
npx repomix
```

Ví dụ yêu cầu phân tích:

``` text
Hãy phân tích source code được cung cấp.

Tập trung vào:

1. Package architecture.
2. Dependency giữa các module.
3. Entity và DTO.
4. Service và Repository.
5. Security configuration.
6. Các điểm có nguy cơ vi phạm SOLID.

Không sửa code.
Chỉ báo cáo hiện trạng và đề xuất cải thiện.
```

Không nên đóng gói toàn bộ repository cho task nhỏ. Cần loại trừ thư mục
build, dependency, secrets và dữ liệu cá nhân.

## 7.2. Graphify

Repository:

https://github.com/Graphify-Labs/graphify

Đây là thành phần tùy chọn. Chỉ triển khai sau khi đã có nhu cầu truy
vấn quan hệ codebase phức tạp và xác minh công cụ hoạt động phù hợp với
môi trường.

## 7.3. Context7

Website:

https://context7.com

Có thể dùng để tra cứu tài liệu framework/thư viện theo phiên bản. Chỉ
cấu hình MCP khi agent và môi trường đang dùng hỗ trợ cách tích hợp
tương ứng.

------------------------------------------------------------------------

# 8. Quality Gates -- Kiểm soát chất lượng code AI

Mọi task code nên đi qua các bước kiểm tra phù hợp với mức độ thay đổi.

1.  **Requirement Validation**
    -   Đã hiểu đúng yêu cầu chưa?
    -   Có thiếu nghiệp vụ hoặc edge case không?
2.  **Architecture Validation**
    -   Có tuân thủ kiến trúc hiện tại không?
    -   Có thêm dependency không cần thiết không?
3.  **Implementation Validation**
    -   Có sửa file ngoài phạm vi không?
    -   Có duplicate logic hoặc hardcode không?
4.  **Automated Testing**
    -   Unit test.
    -   Integration test khi phù hợp.
    -   Build và static checks.
5.  **Security Validation**
    -   Authentication.
    -   Authorization.
    -   Input validation.
    -   Secrets và quyền truy cập.
6.  **Final Review**
    -   Kiểm tra Git diff.
    -   Đọc kết quả test.
    -   Báo cáo rủi ro và giới hạn còn lại.

## Ví dụ lệnh kiểm tra theo stack

### Spring Boot / Maven

``` powershell
.\mvnw.cmd clean verify
```

### React / Node.js

``` powershell
npm run lint
npm run test
npm run build
```

### Angular

``` powershell
npm run test -- --watch=false
npm run build
```

Các lệnh thực tế phải căn cứ vào `pom.xml`, `package.json` và cấu hình
dự án. Không giả định dự án nào cũng có đầy đủ các script trên.

------------------------------------------------------------------------

# 9. Tối ưu token và context

  Thành phần            Cách tải context
  --------------------- ----------------------------------
  Global instructions   Luôn tải, giữ ngắn
  Skills                Chỉ tải khi task phù hợp
  Architecture docs     Đọc theo module liên quan
  Repomix               Chạy khi cần phân tích rộng
  Graphify              Truy vấn khi cần hiểu dependency
  Test results          Chỉ đọc kết quả liên quan

Nguyên tắc:

-   Không đưa toàn bộ kiến thức framework vào global instruction.
-   Không bắt AI viết tài liệu thiết kế cho mọi thay đổi nhỏ.
-   Không gọi MCP nếu tìm kiếm source code thông thường đã đủ.
-   Không chạy lại toàn bộ test suite nếu test mục tiêu đủ cho bước kiểm
    tra hiện tại; vẫn cần kiểm tra regression khi phạm vi thay đổi yêu
    cầu.
-   Không dùng model mạnh nhất cho mọi tác vụ.
-   Ưu tiên context có liên quan trực tiếp đến yêu cầu.

------------------------------------------------------------------------

# 10. Lộ trình triển khai

## Phase 1 -- Foundation

-   [ ] Tạo `.ai-engineering-kit`.
-   [ ] Viết `GLOBAL.md`.
-   [ ] Đồng bộ `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`.
-   [ ] Kiểm tra cả ba agent nhận instruction.

## Phase 2 -- Skills & Workflow

-   [ ] Cài Skills CLI.
-   [ ] Chọn 5--8 skill cốt lõi.
-   [ ] Cài Superpowers theo từng agent.
-   [ ] Thử feature development workflow.

## Phase 3 -- Intelligence

-   [ ] Cài và thử Repomix.
-   [ ] Đánh giá Graphify.
-   [ ] Tích hợp MCP thực sự cần thiết.

## Phase 4 -- Quality & Optimization

-   [ ] Thiết lập testing gates.
-   [ ] Thiết lập security checks.
-   [ ] Đánh giá chất lượng code.
-   [ ] Đo token và thời gian thực thi.
-   [ ] Tinh chỉnh instruction và skill dựa trên kết quả thực tế.

------------------------------------------------------------------------

# 11. Đánh giá hiệu quả bằng benchmark

Dùng cùng một repository, cùng một yêu cầu và ba agent để so sánh.

Ví dụ: Xây dựng chức năng CRUD quản lý nhân viên trong một dự án Spring
Boot có sẵn.

  Tiêu chí       Cách đánh giá
  -------------- -----------------------------------------------------
  Architecture   Có tuân thủ cấu trúc dự án không?
  Correctness    Nghiệp vụ có chính xác không?
  Code quality   Có duplicate, coupling hoặc abstraction thừa không?
  Testing        Test có bao phủ các trường hợp quan trọng không?
  Security       Có kiểm tra quyền và validation không?
  Efficiency     Token, thời gian và số lần sửa lỗi?

Chạy hai vòng:

-   **Baseline:** Agent làm việc không có AI Engineering Kit.
-   **With Kit:** Agent sử dụng instruction, skills và workflow của bộ
    kit.

Giữ nguyên yêu cầu, repository, tiêu chí đánh giá và điều kiện chạy thử
càng nhiều càng tốt. So sánh kết quả thay vì chỉ dựa vào cảm giác.

------------------------------------------------------------------------

# 12. Cấu hình khuyến nghị

  Thành phần             Lựa chọn
  ---------------------- -----------------------------------
  Global instructions    AGENTS.md / CLAUDE.md / GEMINI.md
  Skill manager          Vercel Skills CLI
  Development workflow   Superpowers
  Codebase context       Repomix
  Code graph             Graphify (tùy chọn)
  Documentation lookup   Context7 (tùy chọn)
  Quality                Tests + Static Analysis

## Nguyên tắc cốt lõi

Một nguồn instruction chuẩn, một hệ thống skill có thể tái sử dụng, ba
agent độc lập và một quy trình kiểm chứng thống nhất.

**Khuyến nghị triển khai:** Bắt đầu từ Phase 1. Chưa cần cài Graphify,
MCP hay nhiều skill trước khi xác nhận instruction global hoạt động đúng
trên cả Claude Code, Codex và Gemini CLI.

------------------------------------------------------------------------

# 13. Tài liệu tham khảo

-   Vercel Skills CLI: https://github.com/vercel-labs/skills
-   Vercel Agent Skills: https://github.com/vercel-labs/agent-skills
-   Superpowers: https://github.com/obra/superpowers
-   Repomix: https://github.com/yamadashy/repomix
-   Graphify: https://github.com/Graphify-Labs/graphify
-   Anthropic Skills: https://github.com/anthropics/skills
-   OpenAI Skills: https://github.com/openai/skills
-   Agent Skills Specification: https://agentskills.io
-   Context7: https://context7.com
