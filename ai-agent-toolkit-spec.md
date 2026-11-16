# Đặc tả sản phẩm: AI Agent Toolkit

> **Mục tiêu:** Xây dựng một bộ công cụ hỗ trợ các AI coding agent làm
> việc có quy trình, nhất quán và có thể mở rộng theo từng dự án. Sản
> phẩm lấy cảm hứng từ mô hình AgentKit, nhưng thiết kế độc lập, không
> sao chép mã nguồn, nội dung độc quyền hoặc thương hiệu của sản phẩm
> khác.

------------------------------------------------------------------------

## 1. Tổng quan

### 1.1. Sản phẩm là gì?

AI Agent Toolkit là một bộ công cụ gồm:

1.  **CLI (Command Line Interface):** Cài đặt, cấu hình, khởi tạo, cập
    nhật và quản lý bộ công cụ cho AI coding agent.
2.  **Kits:** Các gói cấu hình theo vai trò hoặc mục đích, ví dụ
    `engineer`, `frontend`, `backend`, `qa`, `marketing`.
3.  **Skills:** Những kỹ năng/hướng dẫn chuyên biệt để agent thực hiện
    một nhiệm vụ cụ thể.
4.  **Rules:** Các quy tắc bắt buộc hoặc khuyến nghị mà agent phải tuân
    thủ.
5.  **Workflows:** Quy trình nhiều bước để xử lý một loại công việc.
6.  **Agents/Subagents:** Các vai trò AI chuyên trách có thể được điều
    phối trong một công việc lớn.
7.  **Runtime adapters:** Lớp chuyển đổi để xuất cấu hình tương thích
    với nhiều AI coding agent khác nhau.
8.  **Cơ chế quản lý phiên bản:** Giúp cài đặt, kiểm tra và cập nhật nội
    dung toolkit an toàn.

Toolkit **không phải là một mô hình AI mới** và không tự thay thế
Claude, GPT, Gemini hay các coding agent. Nó cung cấp cấu trúc, hướng
dẫn, quy trình và tài nguyên để những công cụ đó làm việc tốt hơn.

### 1.2. Vấn đề cần giải quyết

Khi sử dụng AI coding agent, người dùng thường gặp các vấn đề:

-   Agent không hiểu đầy đủ cấu trúc và quy ước của dự án.
-   Mỗi lần giao việc phải giải thích lại cách làm.
-   Agent tự ý sửa nhiều file hoặc thay đổi kiến trúc khi chưa được yêu
    cầu.
-   Chất lượng code, kiểm thử và review không đồng đều.
-   Các bước phân tích, lập kế hoạch, triển khai và kiểm tra chưa được
    chuẩn hóa.
-   Cấu hình prompt/rules/skills bị phân tán, khó tái sử dụng.
-   Cùng một bộ hướng dẫn phải cấu hình lại cho nhiều công cụ AI khác
    nhau.

Toolkit giải quyết các vấn đề trên bằng cách đóng gói kiến thức và quy
trình thành các thành phần có thể cài đặt, tái sử dụng và cập nhật.

### 1.3. Nguyên tắc thiết kế

-   **Model-agnostic:** Không phụ thuộc vào một mô hình AI duy nhất.
-   **Runtime-aware:** Mỗi công cụ AI có cách đọc instructions, skills
    và agents khác nhau; cần adapter riêng.
-   **Project-first:** Ưu tiên hiểu cấu trúc và quy ước dự án trước khi
    sửa code.
-   **Safe by default:** Không tự ý thực hiện hành động phá hủy hoặc
    thay đổi phạm vi lớn.
-   **Composable:** Người dùng có thể chọn từng skill, rule hoặc module.
-   **Transparent:** Người dùng nhìn thấy file nào sẽ được tạo, sửa hoặc
    xóa.
-   **Reversible:** Có cơ chế backup/restore hoặc ít nhất không ghi đè
    file người dùng không sở hữu.
-   **Incremental:** Bắt đầu từ MVP nhỏ, mở rộng dần.

------------------------------------------------------------------------

## 2. Đối tượng sử dụng và tình huống sử dụng

### 2.1. Đối tượng

-   Lập trình viên cá nhân sử dụng AI coding agent.
-   Nhóm phát triển phần mềm muốn thống nhất quy trình làm việc với AI.
-   Tech lead muốn chuẩn hóa coding rules và quy trình review.
-   Người phát triển nhiều dự án, nhiều công nghệ khác nhau.

### 2.2. Các tình huống chính

**Tình huống A --- Khởi tạo toolkit cho dự án**

Người dùng mở terminal trong thư mục dự án, chạy lệnh khởi tạo. CLI hỏi
hoặc nhận tham số về runtime, kit và các skill cần cài. Sau đó CLI tạo
các file tương thích với runtime đã chọn.

**Tình huống B --- Yêu cầu AI lập kế hoạch**

Người dùng gọi skill lập kế hoạch. Agent đọc yêu cầu, kiểm tra cấu trúc
dự án, xác định các file liên quan, phân tích rủi ro và tạo kế hoạch.
Agent chưa sửa code nếu chưa được người dùng cho phép.

**Tình huống C --- Triển khai tính năng**

Agent dựa trên kế hoạch và coding rules để thực hiện thay đổi trong phạm
vi được duyệt. Sau đó chạy kiểm tra phù hợp và báo cáo file đã sửa, kết
quả kiểm thử, vấn đề còn tồn tại.

**Tình huống D --- Debug lỗi**

Agent thu thập thông tin, tái hiện hoặc khoanh vùng lỗi, tìm nguyên nhân
gốc, đề xuất hướng sửa, triển khai bản sửa và kiểm thử hồi quy.

**Tình huống E --- Dùng cùng bộ quy tắc ở nhiều runtime**

Người dùng chọn Claude Code hoặc Codex. CLI xuất cùng một bộ nội dung
nguồn sang định dạng phù hợp với từng runtime, trong phạm vi tính năng
mà runtime hỗ trợ.

------------------------------------------------------------------------

## 3. Kiến trúc tổng thể

Thiết kế theo hướng **core + content packages + runtime adapters**.

``` text
ai-agent-toolkit/
├── apps/
│   └── cli/                    # CLI entry point
├── packages/
│   ├── core/                   # Logic nghiệp vụ dùng chung
│   ├── kit-manager/            # Cài đặt và quản lý kits
│   ├── content-engine/         # Đọc, kiểm tra và render nội dung
│   ├── runtime-adapters/       # Adapter cho từng AI runtime
│   ├── config/                 # Cấu hình và schema
│   └── utils/                  # Logging, filesystem, validation...
├── catalog/
│   ├── kits/                   # Các kit được cung cấp
│   ├── skills/                 # Skills dùng chung
│   ├── rules/                  # Rules dùng chung
│   ├── workflows/              # Workflows
│   └── agents/                 # Định nghĩa subagents
├── templates/                  # Template khởi tạo
├── docs/                       # Tài liệu
└── tests/
```

### 3.1. Luồng hoạt động

``` text
Người dùng
   |
   v
CLI (ak hoặc tên CLI do dự án tự chọn)
   |
   v
Core
   |
   +--> Authentication / Entitlement (nếu có dịch vụ tài khoản)
   |
   +--> Kit Manager
   |       |
   |       +--> Chọn kit
   |       +--> Chọn skills/rules/workflows
   |       +--> Resolve dependencies
   |
   +--> Content Engine
   |       |
   |       +--> Validate manifest
   |       +--> Resolve template variables
   |       +--> Render files
   |
   +--> Runtime Adapter
           |
           +--> Claude Code output
           +--> Codex output
           +--> Cursor output
           +--> Runtime khác trong tương lai
```

### 3.2. Phân tách nội dung và bộ máy

Không viết nội dung skill trực tiếp trong logic CLI.

-   **Core/CLI** chịu trách nhiệm cài đặt, kiểm tra, quản lý và xuất
    file.
-   **Catalog** chứa nội dung skills, rules, workflows, agents và kits.
-   **Adapter** chuyển nội dung nguồn thành cấu trúc file mà runtime mục
    tiêu hiểu được.

Nhờ vậy có thể thay đổi nội dung kit mà không phải sửa logic CLI.

------------------------------------------------------------------------

## 4. Khái niệm và cấu trúc dữ liệu

### 4.1. Kit

Kit là gói tổng hợp các thành phần phục vụ một nhóm công việc.

Ví dụ kit `engineer` có thể chứa:

-   Bộ coding rules chung.
-   Skills phân tích yêu cầu, lập kế hoạch, triển khai, debug, review.
-   Workflow phát triển tính năng.
-   Các subagent như architect, developer, tester, reviewer.
-   Hướng dẫn sử dụng và metadata.

Một kit không nhất thiết phải chứa toàn bộ mọi skill. Kit có thể khai
báo các skill bắt buộc và các skill tùy chọn.

### 4.2. Skill

Skill là một đơn vị hướng dẫn có phạm vi rõ ràng, giúp agent xử lý một
nhiệm vụ.

Ví dụ:

  Skill         Nhiệm vụ
  ------------- ------------------------------------
  `plan`        Phân tích yêu cầu và lập kế hoạch
  `implement`   Triển khai tính năng theo kế hoạch
  `debug`       Điều tra và sửa lỗi
  `review`      Review code và phát hiện rủi ro
  `test`        Xây dựng/chạy kiểm thử
  `explain`     Giải thích code hoặc kiến trúc
  `ask`         Hỏi lại khi yêu cầu chưa rõ

Mỗi skill nên có:

-   Tên và mô tả.
-   Mục tiêu.
-   Khi nào nên sử dụng.
-   Input cần có.
-   Quy trình thực hiện.
-   Quy tắc và giới hạn.
-   Định dạng output mong muốn.
-   Các skill hoặc tài nguyên phụ thuộc (nếu có).

### 4.3. Rule

Rule là chỉ dẫn có phạm vi áp dụng rộng hoặc hẹp.

Ví dụ:

-   Không tự ý thay đổi public API nếu chưa được yêu cầu.
-   Đọc file liên quan và cấu trúc dự án trước khi sửa code.
-   Không tự bịa ra thư viện, endpoint, schema hoặc kiến trúc chưa được
    xác nhận.
-   Ưu tiên thay đổi nhỏ, đúng phạm vi.
-   Không xóa dữ liệu hoặc chạy lệnh phá hủy khi chưa có xác nhận.
-   Chạy test phù hợp sau khi sửa code và báo rõ nếu không chạy được.

Phân loại rule:

-   `global`: áp dụng toàn bộ dự án.
-   `kit`: áp dụng khi dùng một kit.
-   `skill`: chỉ áp dụng trong một skill.
-   `project`: do người dùng định nghĩa riêng cho dự án.

### 4.4. Workflow

Workflow là chuỗi các bước phối hợp nhiều skill.

Ví dụ workflow phát triển tính năng:

1.  Đọc yêu cầu.
2.  Kiểm tra cấu trúc dự án và tài liệu hiện có.
3.  Xác định điểm chưa rõ và hỏi người dùng.
4.  Phân tích tác động.
5.  Lập kế hoạch thay đổi.
6.  Chờ phê duyệt nếu có thay đổi lớn hoặc rủi ro.
7.  Triển khai từng phần.
8.  Chạy test/lint/build phù hợp.
9.  Review diff.
10. Báo cáo kết quả và việc còn thiếu.

### 4.5. Agent/Subagent

Agent là một vai trò AI có nhiệm vụ và phạm vi cụ thể. Ví dụ:

-   `architect`: phân tích kiến trúc và tác động.
-   `backend-developer`: triển khai backend.
-   `frontend-developer`: triển khai frontend.
-   `qa-tester`: xây dựng và thực hiện kiểm thử.
-   `code-reviewer`: review thay đổi.

Ở phiên bản đầu, có thể chỉ tạo các định nghĩa vai trò dưới dạng file
hướng dẫn. Việc tự động điều phối nhiều agent phụ thuộc khả năng của
runtime, không được giả định mọi runtime đều hỗ trợ giống nhau.

### 4.6. Manifest

Mỗi kit cần có manifest để CLI hiểu nội dung và dependency.

Ví dụ minh họa:

``` yaml
id: engineer
name: Engineer Kit
version: 1.0.0
description: Toolkit for software engineering workflows

runtimes:
  - claude-code
  - codex

components:
  skills:
    required:
      - plan
      - implement
      - debug
      - review
    optional:
      - test
      - explain
  rules:
    - engineering-basics
  workflows:
    - feature-development
  agents:
    - architect
    - developer
    - tester
    - reviewer

dependencies: []
```

Đây là schema đề xuất ban đầu. Cần định nghĩa schema chính thức và
validate bằng JSON Schema hoặc thư viện tương đương.

------------------------------------------------------------------------

## 5. CLI --- Các chức năng và lệnh

Tên CLI tạm thời là `aikit`. Có thể đổi sau khi kiểm tra tên package,
domain và thương hiệu.

### 5.1. Cài đặt

Mục tiêu:

-   Hỗ trợ Windows, macOS và Linux.
-   Cài đặt đơn giản bằng một lệnh.
-   Có thể phân phối binary độc lập để người dùng không cần cài Node.js.
-   Có lệnh kiểm tra môi trường và phiên bản.
-   Không yêu cầu quyền Administrator nếu không cần thiết.

Các lệnh dự kiến:

``` bash
aikit --version
aikit doctor
aikit help
```

### 5.2. Đăng nhập (không bắt buộc trong MVP)

Nếu sản phẩm có tài khoản và catalog riêng:

``` bash
aikit login
aikit login --api-key <API_KEY>
aikit whoami
aikit logout
```

Yêu cầu:

-   Hỗ trợ đăng nhập tương tác.
-   Có thể hỗ trợ API key cho CI/CD.
-   Lưu credential trong thư mục cấu hình người dùng, không lưu trong
    repository.
-   Không in đầy đủ secret ra terminal hoặc log.
-   Có hướng dẫn thu hồi credential.
-   Phân biệt rõ API key dùng truy cập dịch vụ và license/entitlement
    dùng xác định quyền sử dụng sản phẩm.

Không bắt buộc xây backend tài khoản ở giai đoạn đầu. Có thể dùng
catalog local trước.

### 5.3. Khởi tạo kit

Ví dụ:

``` bash
aikit kit list
aikit kit info engineer
aikit kit init engineer --target claude-code
aikit kit init engineer --target codex
```

Hỗ trợ hai phạm vi:

-   `--project`: cài vào dự án hiện tại.
-   `--global`: cài vào cấu hình người dùng để dùng ở nhiều dự án.

Nếu không truyền phạm vi, CLI phải hỏi người dùng hoặc dùng mặc định
được ghi rõ trong tài liệu.

### 5.4. Chọn thành phần

``` bash
aikit kit init engineer --target claude-code \
  --skills plan,implement,debug,review

aikit kit init engineer --target claude-code \
  --exclude-skills marketing-content
```

Có thể có chế độ tương tác:

``` bash
aikit kit init engineer --target claude-code --select-skills
```

CLI hiển thị danh sách skill, mô tả và dependency để người dùng chọn.

### 5.5. Cập nhật và kiểm tra

``` bash
aikit kit status
aikit kit refresh engineer --target claude-code
aikit update
aikit self-update
aikit self-update --check
```

Phải phân biệt:

-   Cập nhật **CLI binary**.
-   Cập nhật **nội dung kit**.
-   Cập nhật **cấu hình runtime**.

Không được gộp ba hành vi này thành một lệnh gây nhầm lẫn.

### 5.6. Gỡ cài đặt

``` bash
aikit kit remove engineer --target claude-code
```

Trước khi xóa, CLI cần:

-   Hiển thị các file dự kiến bị xóa.
-   Chỉ xóa file do toolkit quản lý và có metadata xác nhận.
-   Không xóa file người dùng tự tạo.
-   Có chế độ dry-run.
-   Có xác nhận trước thao tác có khả năng làm mất dữ liệu.

### 5.7. Dry-run và báo cáo

Các thao tác tạo, cập nhật hoặc xóa file nên hỗ trợ:

``` bash
aikit kit init engineer --target claude-code --dry-run
aikit kit refresh engineer --target claude-code --dry-run
```

Kết quả cần thể hiện:

-   File sẽ được tạo.
-   File sẽ được cập nhật.
-   File bị bỏ qua.
-   File có xung đột.
-   File sẽ bị xóa (nếu có).
-   Lý do của từng quyết định.

------------------------------------------------------------------------

## 6. Runtime adapters

### 6.1. Vì sao cần adapter?

Các AI coding agent không nhất thiết dùng chung một cấu trúc thư mục
hoặc định dạng cấu hình. Vì vậy, không nên giả định chỉ cần copy cùng
một file vào mọi công cụ là hoạt động như nhau.

Adapter chịu trách nhiệm:

1.  Xác định cấu trúc file mà runtime hỗ trợ.
2.  Chuyển đổi skill/rule/agent/workflow thành định dạng tương ứng.
3.  Ghi file vào đúng vị trí project-level hoặc global-level.
4.  Khai báo giới hạn tính năng của runtime.
5.  Kiểm tra xung đột và khả năng tương thích.

### 6.2. Runtime ưu tiên

Làm theo thứ tự:

1.  Claude Code.
2.  OpenAI Codex CLI.
3.  Cursor.
4.  Các runtime khác sau khi có tài liệu chính thức và kiểm thử thực tế.

Không tuyên bố hỗ trợ một runtime chỉ vì có thể tạo file vào thư mục của
nó. Phải kiểm chứng agent thực sự đọc và sử dụng nội dung đó.

### 6.3. Runtime capability matrix

Mỗi adapter cần khai báo khả năng:

  Capability               Ý nghĩa
  ------------------------ --------------------------------------------------
  `project_instructions`   Hỗ trợ hướng dẫn cấp dự án
  `global_instructions`    Hỗ trợ hướng dẫn cấp người dùng
  `skills`                 Có cơ chế skill
  `agents`                 Có cơ chế agent/subagent
  `hooks`                  Có cơ chế hook
  `commands`               Có cơ chế slash command hoặc command tương đương
  `mcp`                    Có tích hợp MCP nếu cần

Giá trị có thể là `supported`, `partial`, `unsupported`, `unknown`.

Không ép runtime nào cũng phải hỗ trợ tất cả capability. Khi capability
không được hỗ trợ, adapter phải thông báo rõ và đưa ra phương án
fallback nếu hợp lý.

------------------------------------------------------------------------

## 7. Nội dung mẫu của Engineer Kit

Engineer Kit là kit ưu tiên để chứng minh sản phẩm.

### 7.1. Các skill cơ bản

**`plan` --- Phân tích và lập kế hoạch**

Agent cần:

1.  Đọc yêu cầu của người dùng.
2.  Đọc tài liệu và cấu trúc dự án liên quan.
3.  Xác định công nghệ, kiến trúc và convention đang được sử dụng.
4.  Phân tích các file/module có khả năng bị ảnh hưởng.
5.  Nêu điểm chưa rõ và hỏi lại.
6.  Đề xuất kế hoạch theo từng bước.
7.  Nêu rủi ro, test cần có và tiêu chí hoàn thành.

Skill này mặc định không sửa code.

**`implement` --- Triển khai**

Agent cần:

1.  Kiểm tra kế hoạch đã được duyệt hoặc xác nhận phạm vi.
2.  Đọc code hiện tại trước khi sửa.
3.  Tuân thủ kiến trúc và convention hiện hữu.
4.  Chỉ thay đổi những phần cần thiết.
5.  Không tự ý thêm dependency hoặc đổi kiến trúc nếu chưa có lý do và
    sự đồng ý.
6.  Chạy kiểm tra phù hợp.
7.  Tóm tắt thay đổi.

**`debug` --- Điều tra lỗi**

Agent cần:

1.  Thu thập thông báo lỗi, log và bước tái hiện.
2.  Khoanh vùng thành phần liên quan.
3.  Phân biệt triệu chứng và nguyên nhân gốc.
4.  Đưa ra giả thuyết có bằng chứng.
5.  Đề xuất cách sửa ít rủi ro.
6.  Viết hoặc cập nhật test để tránh lỗi tái diễn.
7.  Báo rõ điều gì đã xác minh và điều gì mới chỉ là giả thuyết.

**`review` --- Review code**

Agent kiểm tra:

-   Bug và logic sai.
-   Security issue.
-   Validation và xử lý lỗi.
-   Hiệu năng.
-   Khả năng bảo trì.
-   Test bị thiếu.
-   Vi phạm coding rules.

Mỗi finding cần có mức độ nghiêm trọng, file/vị trí, lý do và hướng khắc
phục. Không tạo finding giả hoặc chỉ nêu nhận xét chung chung.

**`test` --- Kiểm thử**

Agent cần xác định test framework hiện có, ưu tiên cách chạy test của dự
án, bổ sung test phù hợp và báo kết quả thật. Không được nói test pass
nếu chưa chạy hoặc không có bằng chứng.

**`explain` --- Giải thích**

Giải thích theo mức độ người dùng yêu cầu, ưu tiên bám sát code thực tế,
có ví dụ và luồng thực thi. Phân biệt nội dung đọc được từ source với
suy luận.

**`ask` --- Làm rõ yêu cầu**

Khi thiếu thông tin quan trọng, agent phải hỏi câu hỏi ngắn gọn, cụ thể.
Không tự suy đoán những quyết định ảnh hưởng lớn đến kiến trúc, dữ liệu,
bảo mật hoặc hành vi sản phẩm.

### 7.2. Quy trình phát triển tính năng chuẩn

``` text
Requirement
    |
    v
Understand project
    |
    v
Clarify unknowns
    |
    v
Impact analysis
    |
    v
Plan
    |
    v
User approval (when needed)
    |
    v
Implement
    |
    v
Test
    |
    v
Review diff
    |
    v
Report
```

### 7.3. Quy tắc kỹ thuật ví dụ

Đối với dự án backend Java/Spring Boot:

-   Đọc `pom.xml` hoặc `build.gradle` để xác định phiên bản và
    dependency thực tế.
-   Đọc package structure và các module tương tự trước khi thêm code.
-   Tôn trọng cách phân chia Controller, Service, Repository, DTO,
    Mapper hiện tại.
-   Không tự ý thay đổi entity/database schema khi yêu cầu không đề cập.
-   Không đưa business logic vào mapper nếu dự án đang quy định service
    quản lý logic đó.
-   Không tự tạo endpoint hoặc field không có trong yêu cầu.
-   Ưu tiên dùng exception handling, validation, logging và response
    format hiện có.
-   Chạy test/build tương ứng và báo chính xác kết quả.

Các rule theo framework phải nằm trong skill/rule chuyên biệt, không áp
dụng cứng cho mọi loại dự án.

------------------------------------------------------------------------

## 8. Quản lý file và tránh ghi đè dữ liệu

Đây là phần quan trọng của CLI.

### 8.1. Metadata quản lý

CLI cần lưu metadata về những file do nó tạo, ví dụ:

``` json
{
  "managedBy": "aikit",
  "kitId": "engineer",
  "kitVersion": "1.0.0",
  "runtime": "claude-code",
  "files": [
    {
      "path": "example/path/in/project",
      "templateId": "engineering-instructions",
      "checksum": "sha256:..."
    }
  ]
}
```

Đây chỉ là ví dụ minh họa; cần thiết kế schema chính thức.

### 8.2. Quy tắc ghi file

-   Nếu file chưa tồn tại: có thể tạo.
-   Nếu file tồn tại nhưng không do toolkit quản lý: không ghi đè mặc
    định.
-   Nếu file do toolkit quản lý và chưa bị người dùng sửa: có thể cập
    nhật.
-   Nếu file do toolkit quản lý nhưng đã bị người dùng sửa: phát hiện
    xung đột và hỏi người dùng.
-   Không xóa file không thuộc quyền quản lý của toolkit.

### 8.3. Hỗ trợ backup và restore

Trước thao tác cập nhật lớn, có thể tạo backup. Cần có thông tin về thời
điểm, phiên bản cũ và file liên quan. Việc restore phải có xác nhận.

------------------------------------------------------------------------

## 9. Catalog và hệ thống phân phối kit

Có hai chế độ:

### 9.1. Local catalog

-   Kit được đóng gói cùng CLI hoặc nằm trong repository.
-   Không cần tài khoản.
-   Phù hợp phát triển MVP và dùng cá nhân.
-   Dễ kiểm thử, dễ chạy offline.

### 9.2. Remote catalog (giai đoạn sau)

Một dịch vụ catalog có thể cung cấp:

-   Danh sách kit.
-   Metadata và phiên bản.
-   Nội dung kit.
-   Changelog.
-   Tương thích runtime.
-   Dependency.
-   Chữ ký hoặc checksum để kiểm tra tính toàn vẹn.

API dự kiến (chỉ là định hướng, chưa phải contract cuối cùng):

``` text
GET /api/v1/kits
GET /api/v1/kits/{kitId}
GET /api/v1/kits/{kitId}/versions
GET /api/v1/kits/{kitId}/versions/{version}
```

Không cần xây marketplace ngay từ đầu. Trước tiên cần chứng minh CLI cài
và quản lý kit local hoạt động ổn định.

------------------------------------------------------------------------

## 10. Tài khoản, license và bảo mật

Phần này chỉ triển khai khi sản phẩm cần phân phối thương mại hoặc quản
lý quyền truy cập.

### 10.1. Phân biệt khái niệm

-   **Account:** Danh tính người dùng.
-   **API key/token:** Credential để CLI gọi dịch vụ.
-   **License/entitlement:** Quyền sử dụng một sản phẩm hoặc kit.
-   **Subscription:** Quyền sử dụng có thời hạn nếu mô hình kinh doanh
    yêu cầu.

Không dùng chung một khái niệm cho tất cả.

### 10.2. Yêu cầu bảo mật

-   Không commit secret vào Git.
-   Không log API key/token.
-   Hạn chế quyền truy cập file credential.
-   Hỗ trợ logout và thu hồi credential.
-   Có thời hạn hoặc cơ chế xoay vòng token phù hợp.
-   Kiểm tra quyền ở server đối với nội dung trả phí.
-   Truyền dữ liệu qua HTTPS.
-   Không tải và thực thi script tùy ý từ catalog.
-   Kiểm tra checksum/chữ ký của package nếu có remote distribution.
-   Hiển thị nguồn gốc và phiên bản nội dung được cài.

### 10.3. Không được giả định

Không tự quyết định rằng một license được chia sẻ cho nhiều người hoặc
nhiều thiết bị. Quy tắc này phải được xác định rõ trong chính sách sản
phẩm và được backend thực thi nếu có.

------------------------------------------------------------------------

## 11. Trải nghiệm người dùng (UX)

### 11.1. Nguyên tắc

-   Lệnh ngắn, dễ nhớ.
-   Có `--help` và ví dụ.
-   Lỗi phải nói rõ nguyên nhân và cách xử lý.
-   Các thao tác có rủi ro phải có xác nhận.
-   Có chế độ tương tác cho người mới và chế độ flags cho người dùng
    nâng cao.
-   Không yêu cầu người dùng hiểu toàn bộ kiến trúc nội bộ mới cài được
    kit.

### 11.2. Ví dụ trải nghiệm

``` text
$ aikit kit init engineer --target claude-code

AI Agent Toolkit

Kit: Engineer
Version: 1.0.0
Target: Claude Code
Install scope: Project

Available skills:
  [x] plan
  [x] implement
  [x] debug
  [x] review
  [ ] test
  [ ] explain
  [ ] ask

The following files will be created:
  - <runtime-specific project instruction file>
  - <runtime-specific skills directory>
  - <runtime-specific agent definitions, if supported>
  - .aikit/manifest.json

Continue? [y/N]

Installation completed.
Created: ...
Skipped: ...
Warnings: ...
```

Các đường dẫn cụ thể phải do adapter xác định theo tài liệu chính thức
của runtime, không hard-code dùng chung cho mọi runtime.

------------------------------------------------------------------------

## 12. Công nghệ đề xuất

Đây là lựa chọn ban đầu để đội phát triển đánh giá, không phải yêu cầu
bắt buộc.

### 12.1. CLI

Có thể chọn một trong các hướng:

-   **Go:** thuận lợi để build binary đa nền tảng, phù hợp CLI độc lập.
-   **Rust:** hiệu năng tốt, binary độc lập, nhưng tốc độ phát triển có
    thể chậm hơn với người chưa quen.
-   **Node.js/TypeScript:** phát triển nhanh, phù hợp nếu đội ngũ quen
    hệ sinh thái TypeScript; cần quyết định cách phân phối runtime.

Ưu tiên chọn **một** công nghệ, không triển khai nhiều CLI song song.

### 12.2. Backend (giai đoạn sau)

Nếu cần account, license và remote catalog:

-   Backend API.
-   Database lưu account, product, entitlement, package metadata và
    version.
-   Object storage/CDN để phân phối package.
-   Admin interface để quản lý catalog.
-   Audit log cho các hoạt động quan trọng.

Không cần backend nếu MVP chỉ dùng local catalog.

### 12.3. Kiểm thử

-   Unit test cho parser, manifest validator, dependency resolver.
-   Integration test cho kit installation.
-   Golden/snapshot test cho output của từng adapter.
-   Test trên Windows, macOS, Linux.
-   Test xung đột file và nội dung người dùng chỉnh sửa.
-   Test upgrade từ phiên bản kit cũ.
-   Test lỗi mạng và lỗi xác thực nếu có remote catalog.

------------------------------------------------------------------------

## 13. Phạm vi MVP

Không xây tất cả tính năng ngay từ đầu.

### Phase 1 --- MVP cốt lõi

Mục tiêu: Người dùng có thể cài một Engineer Kit vào một runtime và dùng
được các hướng dẫn.

Bao gồm:

-   CLI chạy được trên Windows trước.
-   `--version`, `help`, `doctor`.
-   Local catalog.
-   Một kit `engineer`.
-   Skills: `plan`, `implement`, `debug`, `review`, `test`, `explain`,
    `ask`.
-   Rules cơ bản.
-   Một adapter đầu tiên cho Claude Code.
-   Lệnh `kit list`, `kit info`, `kit init`, `kit status`, `kit remove`.
-   Project-level installation.
-   Dry-run.
-   Metadata để nhận biết file do toolkit quản lý.
-   Không ghi đè file người dùng không sở hữu.
-   Tài liệu cài đặt và sử dụng.

**Tiêu chí hoàn thành Phase 1:**

1.  Người dùng cài CLI theo tài liệu.
2.  `doctor` kiểm tra môi trường và báo lỗi hữu ích.
3.  Có thể xem kit và skill có sẵn.
4.  Có thể cài Engineer Kit vào một dự án.
5.  Runtime mục tiêu thực sự nhận diện và sử dụng các hướng dẫn đã tạo.
6.  Có thể xem trạng thái cài đặt.
7.  Có thể gỡ toolkit mà không xóa file người dùng tự tạo.
8.  Test tự động chạy thành công trên môi trường CI mục tiêu.

### Phase 2 --- Mở rộng runtime

-   Adapter cho Codex.
-   Adapter cho Cursor.
-   Global installation.
-   Chọn skill khi cài.
-   Refresh/update kit.
-   Compatibility matrix.
-   Test thực tế trên từng runtime và phiên bản.

### Phase 3 --- Hệ thống phân phối

-   Remote catalog.
-   Versioning và changelog.
-   Account và API authentication.
-   License/entitlement nếu có nội dung trả phí.
-   Package integrity verification.
-   Caching và offline behavior.

### Phase 4 --- Quản trị và hệ sinh thái

-   Web dashboard.
-   Quản lý kit và phiên bản.
-   Tạo kit riêng.
-   Import/export kit.
-   Team-shared configuration.
-   Marketplace nếu thực sự cần.
-   Analytics có kiểm soát quyền riêng tư.

------------------------------------------------------------------------

## 14. Những điều không làm trong MVP

-   Không tự xây mô hình LLM.
-   Không tự xây một IDE hoặc coding agent mới.
-   Không tự động điều phối multi-agent nếu runtime chưa hỗ trợ.
-   Không xây marketplace ngay.
-   Không xây hệ thống thanh toán ngay.
-   Không yêu cầu tất cả runtime phải hỗ trợ cùng một tính năng.
-   Không chạy lệnh terminal nguy hiểm mà không có xác nhận.
-   Không tự sửa kiến trúc dự án của người dùng.
-   Không ghi đè các file cấu hình sẵn có mà không có cơ chế phát hiện
    xung đột.

------------------------------------------------------------------------

## 15. Yêu cầu dành cho AI coding agent triển khai sản phẩm

AI coding agent nhận tài liệu này phải làm việc theo các nguyên tắc sau.

### 15.1. Trước khi viết code

1.  Kiểm tra repository hiện tại.
2.  Đọc toàn bộ tài liệu hiện có liên quan.
3.  Xác định ngôn ngữ, framework, cấu trúc thư mục và cách chạy test.
4.  Đề xuất kiến trúc triển khai phù hợp với repository.
5.  Liệt kê các điểm chưa rõ hoặc quyết định cần người dùng xác nhận.
6.  Chia công việc thành các phase nhỏ.
7.  Không tự ý chọn công nghệ hoặc thêm dependency lớn nếu chưa giải
    thích lý do.

### 15.2. Trong khi triển khai

-   Bám sát phạm vi MVP Phase 1.
-   Triển khai theo từng module nhỏ.
-   Ưu tiên code dễ đọc, dễ test và dễ mở rộng.
-   Không tạo các abstraction phức tạp khi chưa có nhu cầu.
-   Viết test cho logic quan trọng.
-   Không giả lập tính năng chưa triển khai.
-   Không tuyên bố hỗ trợ runtime nếu chưa có kiểm thử xác nhận.
-   Giữ nguyên dữ liệu và file hiện có của repository.

### 15.3. Sau mỗi phần việc

Báo cáo:

1.  Đã triển khai những gì.
2.  Những file chính đã tạo/sửa.
3.  Lệnh test/build đã chạy.
4.  Kết quả thực tế.
5.  Vấn đề còn tồn tại.
6.  Bước tiếp theo đề xuất.

### 15.4. Cách bắt đầu

Không bắt đầu bằng việc tạo toàn bộ sản phẩm trong một lần.

Hãy thực hiện theo thứ tự:

**Bước 1:** Audit repository và báo cáo hiện trạng.

**Bước 2:** Đề xuất tech stack, kiến trúc MVP và cấu trúc thư mục dựa
trên hiện trạng thực tế.

**Bước 3:** Chốt schema manifest, cấu trúc catalog và contract của
runtime adapter đầu tiên.

**Bước 4:** Xây CLI skeleton cùng local catalog và manifest validator.

**Bước 5:** Xây adapter đầu tiên và Engineer Kit.

**Bước 6:** Hoàn thiện cài đặt, dry-run, status, remove và kiểm thử.

Chỉ chuyển sang bước tiếp theo khi bước hiện tại có kết quả kiểm chứng
được.

------------------------------------------------------------------------

## 16. Các quyết định cần chốt trước khi phát triển

AI coding agent cần đưa ra đề xuất và hỏi người dùng xác nhận các điểm
sau, không tự suy đoán:

1.  Tên sản phẩm và tên CLI chính thức.
2.  Ngôn ngữ triển khai CLI: Go, Rust hay TypeScript.
3.  Runtime đầu tiên: Claude Code hay Codex.
4.  Chỉ cài ở cấp project hay hỗ trợ cả global ngay trong MVP.
5.  Catalog ban đầu chỉ local hay cần remote.
6.  Kit đầu tiên chỉ là Engineer hay cần thêm kit khác.
7.  Có cần tài khoản/license trong phiên bản đầu không.
8.  Sản phẩm dùng cá nhân, nội bộ team hay phân phối thương mại.

------------------------------------------------------------------------

## 17. Định nghĩa thành công của sản phẩm

Sản phẩm được xem là có giá trị khi:

-   Người dùng cài một lần và tái sử dụng được bộ quy tắc/skills ở nhiều
    dự án.
-   AI agent hiểu quy trình làm việc và convention rõ hơn.
-   Người dùng không phải lặp lại các hướng dẫn giống nhau trong mỗi
    prompt.
-   Có thể chọn và cập nhật từng thành phần.
-   Không làm hỏng cấu hình hoặc file có sẵn của người dùng.
-   Có thể mở rộng sang runtime mới mà không phải viết lại toàn bộ core.
-   Các tuyên bố về tính năng đều có kiểm thử hoặc bằng chứng thực tế.

**Ưu tiên cao nhất:** Làm một CLI nhỏ, một Engineer Kit tốt, một runtime
adapter hoạt động thực sự và cơ chế quản lý file an toàn. Chất lượng của
luồng này quan trọng hơn số lượng kit hoặc số runtime được quảng cáo hỗ
trợ.
