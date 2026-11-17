# DTKit — kế hoạch chốt RC và phát triển chất lượng agent v0.5

Ngày lập: 2026-11-17. Baseline code: `c100b867cf2e7daef348d7e50f24b3f0239cd0b8`, branch `feat/release-hardening`. CLI dev `0.4.0-dev`, Engineer Kit `0.2.0`. Đây là kế hoạch thực thi tiếp nối [DTKIT-NEXT-PHASE-PLAN.md](DTKIT-NEXT-PHASE-PLAN.md), không phải bằng chứng các công việc dưới đây đã hoàn thành.

## 1. Mục đích, thứ tự đọc và nguyên tắc quyết định

Mục tiêu sản phẩm: một CLI cài/quản lý kit an toàn, đồng thời giúp agent hiểu đúng repository, làm đúng phạm vi và tạo thay đổi vượt qua kiểm tra độc lập. Thành công phải được đo bằng task thực tế, không suy từ số skill được cài hoặc số test renderer pass.

AI nhận việc đọc: file này → [docs/progress.md](docs/progress.md) → [docs/release.md](docs/release.md) → [docs/compatibility.md](docs/compatibility.md) → [docs/evaluation/README.md](docs/evaluation/README.md). Tra contract chi tiết ở [DTKIT-IMPLEMENTATION-PLAN.md](DTKIT-IMPLEMENTATION-PLAN.md) và plan N00–N18 khi sửa module liên quan. Hai tài liệu ý tưởng gốc là [ai-agent-toolkit-spec.md](ai-agent-toolkit-spec.md), [AI-Engineering-Kit-Global.md](AI-Engineering-Kit-Global.md).

Quy tắc điều hành:

1. Kiểm tra Git/code/CI mới nhất trước mỗi milestone; SHA trong file là snapshot, không ghi đè tiến độ mới hơn.
2. Reuse planner, transaction, state validator, shared-target lock và error/JSON contract. Không làm lại core N01–N07 khi không có regression cụ thể.
3. Phân biệt `implemented`, `automated-tested`, `runtime-discovered`, `live-task-verified`, `blocked`. Mọi trạng thái phải có evidence tương ứng. Thiếu auth/tool không được ghi pass.
4. N08/N09 hiện có harness/discovery/structural checks nhưng chưa đủ live task/A-B evidence. Sửa tracker thành partial ở phần nghiệm thu còn thiếu; không xóa lịch sử test đã có.
5. Giữ branch release ổn định. Phát triển Claude, project config, quality runner trên branch v0.5 riêng. Không đưa thay đổi schema/multi-kit vào RC chỉ vì đang làm chung repo.
6. User đã cho phép làm source/feature branch; kế hoạch này chưa tự cấp quyền merge/tag/publish hay tiêu thụ ngân sách model không giới hạn. Chuẩn bị artifact/PR/report cụ thể trước khi cần quyết định cuối; tiếp tục các việc độc lập khi thiếu quyết định.

## 2. Ba mốc bàn giao và định nghĩa “xong”

| Mốc | Phạm vi | Điều kiện nghiệm thu |
|---|---|---|
| A — `0.4.0-rc.1` | Chốt docs/evidence, hướng dẫn cài an toàn, native artifact smoke, merge/tag/draft/public release theo quyền owner | Artifact đúng SHA, Windows/Linux/macOS smoke thực sự chạy, upgrade từ baseline cũ đạt; labels partial/experimental chính xác |
| B — v0.5 | Claude adapter có live task; project profile; benchmark runner; Engineer Kit cải tiến; quality checks; một integration hook Claude; preset/component selection | Ít nhất một runtime hoàn thành task thật; so sánh benchmark trước/sau có dữ liệu; checks chạy độc lập và qua hook; backward compatibility đạt |
| C — Sau v0.5 | Multi-kit, tùy chỉnh kit, team lockfile, provenance, thêm hooks runtime, re-check tương thích | Mỗi tính năng có contract/migration/test riêng; không là blocker cho A/B |

Nếu model benchmark không cho thấy lợi ích hoặc gây regression, B có thể phát hành dưới dạng experimental với kết quả trung thực, nhưng chưa được tuyên bố “kit cải thiện chất lượng code”. Không đạt score bằng cách bỏ task khó, tắt assertion hoặc đổi model giữa hai nhánh.

## 3. Baseline và các điểm đã biết cần đóng

- Nhánh đầy đủ hiện tại là `feat/release-hardening`; snapshot `main` vẫn `f81ce6e`. CI cho `c100b86` đã được kiểm tra trong phiên review trước: Windows/Linux/macOS/race/fuzz success, run [43912929680](https://github.com/truong-newbie/Agent_Kit_Impl/actions/runs/43912929680). Kiểm tra lại tag/release/remote refs khi thực thi.
- Release workflow có `native-smoke` và draft-release, nhưng định nghĩa job và Go CI pass chưa phải bằng chứng workflow release đã chạy trọn trên candidate cuối.
- Codex Windows: báo cáo N08 xác nhận isolated CODEX_HOME/AGENTS.md, project skills; override HOME/USERPROFILE không chứng minh global `.agents/skills` của OS user thật. Probe CODEX_HOME/skills được discover. Chưa đổi layout đang cài dựa riêng vào probe này.
- Codex task bị sandbox chặn PowerShell; OpenCode discovery có nhưng isolated profile thiếu credentials; Gemini/Cursor live task chưa được xác minh. Claude local đã kiểm tra version `2.1.319`; executable có mặt chưa chứng minh auth, tool permissions hoặc task thành công.
- N09 hiện có 10 scenarios, structural invariant tests và context delta; chưa có benchmark A/B nhiều lần. Một số scenarios và ví dụ skill còn gắn với nội bộ DTKit.
- Đoạn Unix trong docs/release.md chưa dừng rõ khi checksum check fail; architecture selection còn giả định Linux amd64/macOS arm64. Cần kiểm chứng chính hướng dẫn copy/paste.
- Một runtime/scope hiện chỉ sở hữu một kit; tên skill chưa kit-aware. Multi-kit cần migration names/state/ownership, không chỉ cho phép nhiều dòng install.

## 4. Mốc A — Chốt RC trước khi mở rộng tính năng

### A01 — Chuẩn hóa trạng thái release và proof

**Phụ thuộc:** không. **File:** docs/progress.md, docs/release.md, docs/compatibility.md, README.md.

1. Ghi baseline SHA, current remote main, tags/releases, checks của đúng HEAD. Không suy ra “không có release” từ tag local trống.
2. Chỉnh N08/N09: discovery/harness đạt nhưng live task/A-B còn partial/blocked. N10 tách workflow implemented, local packaging passed và native release workflow passed; mỗi claim có link/job/log.
3. Đặt support boundary cho RC: code lifecycle đã test; runtime nào partial/experimental; hook nào configured nhưng chưa observed invocation. Giữ limitation trong release notes, không chỉ trong progress.
4. Tạo release checklist evidence: version, full SHA, checksum assets, upgrade test, CI URL, release workflow URL, runtime report URLs, owner publication decision.

**Done:** mọi checkbox gắn đúng loại evidence; người đọc không hiểu N08/N09 là full live verified. Docs-only kiểm links và diff, không chạy model.

### A02 — Kiểm thử cài và nâng binary qua terminal

**Phụ thuộc:** A01. **File:** docs/release.md, scripts/package.ps1, scripts/build.ps1, .github/workflows/release.yml; thêm installer script chỉ khi cần dùng chung logic.

1. Sửa snippet/script thành fail-fast: download fail, missing/duplicate checksum entry, digest mismatch, extraction fail phải dừng trước replace binary. Chọn checksum tool theo availability, không fallback sang tool khác chỉ vì checksum báo mismatch.
2. Nhận diện OS và architecture thật; unsupported target trả lỗi có hướng dẫn. Không tải arm64 cho Intel Mac hoặc amd64 cho ARM Linux chỉ từ uname OS.
3. Dùng temp directory duy nhất/request; validate destination user bin; verify archive trước extract, kiểm version/commit sau extract; temp cleanup đúng scope. Windows locked binary phải fail giữ binary cũ.
4. Test upgrade CLI khi hook đang bật: stable executable path, failed replace giữ cũ, update xong hook vẫn chạy. Không xóa binary đang được hook tham chiếu mà không hướng dẫn disable/rebind.
5. Thêm test dùng fixture HTTP server và binary/archive nhỏ cho good/bad checksum, network fail, wrong arch, previous binary preservation. Script thực và ví dụ docs dùng cùng quy trình đã kiểm.

**Done:** từ máy/test home sạch có thể install/upgrade/uninstall binary có kiểm checksum; failure paths không làm mất binary dùng được. Kit removal và CLI uninstall được giải thích riêng.

### A03 — Chốt Codex global Windows bằng evidence

**Phụ thuộc:** A01. **File:** adapter/codex, paths, state/planner nếu đổi layout, docs/verification/codex-*.

1. Mặc định ưu tiên test layout hiện tại trên OS account/VM cô lập có profile thật. Nếu môi trường không có, báo global discovery partial trong RC và tiếp tục các gate độc lập.
2. Nghiên cứu hỗ trợ CODEX_HOME/skills như một layout tùy chọn riêng; verify official docs + version thực, project/global precedence, duplicate discovery và cách resource links resolve.
3. Chỉ đổi layout nếu có use case và evidence. Khi đổi phải lưu layout/binding vào state, tăng adapter revision, migrate owned paths qua transaction, giữ user skills, test upgrade/remove/recover cả old/new layout. Không copy cùng skill vào hai global locations để che lỗi.
4. Thử plan/review read-only và implement có test trong môi trường sandbox cho phép; không vô hiệu hóa chính sách bảo mật để tạo pass. Trust hooks được test riêng.

**Done:** có quyết định bằng văn bản: giữ layout với test thật, hoặc giữ partial RC, hoặc thêm optional layout có migration. Không chặn toàn RC vô thời hạn chỉ vì máy kiểm thử hạn chế nếu RC công bố đúng giới hạn.

### A04 — Chạy pipeline và hoàn tất release candidate

**Phụ thuộc:** A01–A02; quyết định A03 được ghi rõ.

1. Review diff từ main, chuẩn bị PR release-hardening; giữ bằng chứng tests trên candidate. Kiểm tra quyền merge/tag/publish trong hội thoại/repo hiện tại; nếu chưa có, trình PR + checklist + version cụ thể để owner chốt.
2. Có thể dispatch workflow trên branch phù hợp để chạy build/native-smoke trước public release nếu workflow/default branch và quyền cho phép. Không coi việc chỉ thêm YAML là đã test workflow.
3. Sau merge được phép, kiểm CI của merge SHA. Tạo tag version chưa dùng trên đúng SHA; không di chuyển tag đã phát hành. Nếu RC candidate lỗi trước public, sửa và build lại; nếu đã public thì tăng rc number.
4. Native-smoke bắt buộc extract và chạy Windows amd64/Linux amd64/macOS arm64 bằng runner đúng architecture; checksum, version/full SHA, install/status/remove và upgrade fixture phải đúng.
5. Tạo draft Release với assets + SHA256SUMS + known issues; public prerelease theo quyền owner. Smoke đường tải actual assets; link README dùng URL đã tồn tại. Ghi lại URL/SHA vào docs.

**Done:** RC thật có thể tải/cài, hoặc draft candidate hoàn chỉnh đang chờ quyền cuối được ghi rõ. Không đánh milestone published khi mới upload Actions artifact.

## 5. Mốc B — v0.5 tập trung bằng chứng chất lượng code

### B01 — Thêm Claude adapter tối thiểu và một live task thành công

**Phụ thuộc:** tách branch từ code A ổn định; không cần chờ public RC để chuẩn bị. **Ưu tiên:** đầu tiên trong v0.5.

**Nguồn tham khảo:** [Claude skills](https://code.claude.com/docs/en/skills), [settings và precedence](https://code.claude.com/docs/en/settings), [hooks](https://code.claude.com/docs/en/hooks). Đã tham khảo ngày 2026-11-17; đọc lại bản hiện hành, pin phiên bản runtime thực test. Không suy API từ Codex.

1. Xác minh `claude --version`, cấu hình và auth availability bằng kiểm tra không lộ credential. Chạy một task fixture đọc file/chạy test trong quyền hợp lệ trước khi hứa benchmark khả thi. Model run phải có giới hạn chi phí/thời gian đã được owner cho phép.
2. Profile dự kiến: runtime ID `claude`; project CLAUDE.md managed block; `.claude/skills/dtkit-claude-project-*`; personal skills ở Claude config home đã xác minh; state `.dtkit/runtimes/claude/state.json`; operational dirs tương tự runtime khác. Validate CLAUDE_CONFIG_DIR semantics bằng native test, không hardcode giả định nó luôn bằng user home/.claude.
3. Nối profile vào renderer, catalog allowed runtimes, schemas, CLI runtime parser/help, roots, state allowlist, doctor, install/update/status/remove/recover; templates gọi skill đúng cú pháp Claude. Frontmatter/resources phải render đúng ở cả scope.
4. Test personal instructions/skills tồn tại, custom config dir, project+global, five-runtime coexistence, monorepo/subdirectory và Git worktree inheritance. Root/path mapping và command source phải có evidence đúng version.
5. CLI integration label bắt đầu experimental. Chỉ nâng evidence khi session thật discover skill, đọc resource, làm task, chạy test; ghi phiên bản/model/SHA/output đã redact. Global scope không verified theo suy luận từ project.
6. Thêm `claude` vào manifest làm đổi digest kit: tăng kit version, tuyệt đối không sửa `engineer@0.2.0` đã đóng băng. Gợi ý baseline kit `0.3.0` chỉ thêm runtime mapping, giữ nguyên nội dung để làm mốc đo; phiên bản tiếp theo dành cho content cải tiến. Package parser hiện cần semver ba số, không đưa prerelease kit vào khi chưa hỗ trợ.

**File:** internal/adapter/{adapter,render}.go, adapter/claude/* mới, internal/cli/runtime.go, paths, catalog, state, schemas, tests theo runtime hiện có.

**Done:** full CLI lifecycle tự động + ít nhất một implement task thật sửa fixture và test độc lập pass; read-only task không sửa file. Nếu auth bị chặn, adapter có thể automated-tested nhưng milestone live còn blocked; không thay bằng một câu agent tự nhận đã dùng skill.

### B02 — N09/N18: benchmark runner có thể lặp lại và baseline đóng băng

**Phụ thuộc:** B01 có runtime chạy task; harness/fixtures có thể làm trước. **Đầu ra:** internal/eval hoặc công cụ dev tương đương, testdata/evaluation, docs/evaluation; không bắt buộc thêm public CLI command.

1. Tách scenarios khỏi code nội bộ DTKit. Có ít nhất 10 tasks đánh giá trên fixture Go service, TypeScript/React, Python, gồm read-only plan/review/explain, implement, debug, regression tests, thiếu dependency và instruction không đáng tin. Chuẩn bị thêm development variants cho pilot/tuning; không lấy chính các task đã tune làm held-out. Để DTKit-specific cases thành suite riêng.
2. Mỗi task pin initial repo snapshot/dependency lock, allowed write paths, expected behavior và private evaluator tests. Fixture chạy thật, không chỉ JSON prompt mô tả một file chưa tồn tại. Người chấm giữ tests ngoài vùng agent sửa; reject agent sửa/xóa/disabling evaluator.
3. Mỗi run tạo checkout/home/session độc lập, workspace sạch và không có global kit lẫn vào baseline. Hai arms cùng model ID/settings/runtime version/permissions/toolchain/network policy/time/token cap. Ghi mọi khác biệt không kiểm soát được.
4. Trước tiên chạy pilot 2–3 development tasks để kiểm auth/tool access/collector và lưu baseline trước khi sửa kit. Kế hoạch đánh giá cuối là N>=3 × 10 tasks × 2 arms = 60 runs (30 cặp) cho một cấu hình runtime/model. Chốt budget tổng và cap mỗi run trước khi chạy; thiếu ngân sách thì làm dry harness và báo, không coi đã có kết quả. Bộ held-out đầy đủ chạy khi candidate content đã đóng băng ở B08; baseline không-kit phải dùng cùng môi trường/model với candidate tại thời điểm đó.
5. Chốt đánh giá explicit skill invocation vs automatic discovery. Có thể dùng hai suite riêng; không đổi prompt để cho with-kit thêm đáp án/hướng dẫn mà baseline không có. Randomize thứ tự arms, session mới, reset memory để tránh learning/carry-over.
6. Run record tối thiểu: task/fixture hash, arm, repeat, runtime/model version, kit/version/digest, dtkit SHA, config hash, duration, token/cost nếu runtime cung cấp, exit codes, changed paths, evaluator results, scope violation, evidence artifact paths.
7. Phân loại `passed`, `task_failed`, `environment_blocked`, `timeout`, `cancelled`, `not_run`. Không loại failed trials khỏi mẫu mà không báo; environment-blocked riêng với tỷ lệ pass của runs hợp lệ và tổng số blocked.
8. Đo false-completion (agent nói xong nhưng evaluator fail), test tampering, regression ở phần không liên quan, fabricated execution claims bằng trace đối chiếu. Những điểm cần human adjudication phải ghi rõ, không dùng một model chấm khác làm ground truth duy nhất.
9. Report raw counts và paired results, N, variance/uncertainty; N=3 là pilot nhỏ. Không công bố % improvement khi chưa đủ runs hoặc môi trường bị chặn. Token/cost unavailable là null/unknown, không ghi 0.
10. Evaluator owner giữ bộ held-out và hash trước content tuning; AI chỉnh skill không dùng đáp án/case này để tối ưu. Sau khi đã xem kết quả và sửa theo case, coi bộ đó là regression suite, không tiếp tục gọi là held-out độc lập. Report rõ ai tạo/chấm fixture và giới hạn nếu chưa thể tách người viết kit với người chấm.

**Done B02:** runner test bằng fake runtime + một pilot live chạy được; baseline kit/runtime/config và rubric được đóng băng trước content tuning; có manifest bộ đánh giá, cách cô lập và dự toán runs. Full report 60 runs là gate B08, không được tick đã có trong B02 khi mới chạy pilot. Scope violations mục tiêu là 0 trên suite, không phải bảo đảm tuyệt đối ngoài suite.

### B03 — Project profile: agent biết dùng lệnh và phạm vi của repo

**Phụ thuộc:** B02 đã đóng băng baseline; có thể thiết kế schema sớm. **Command dưới đây là contract đề xuất mới, chưa tồn tại.**

```text
dtkit project inspect --path <repo> --json
dtkit project init --path <repo> --dry-run
dtkit project init --path <repo> --yes
```

1. File đề xuất `.dtkit/project.json`, versioned, git-friendly, độc lập với installation state; chứa workspace roots tương đối, stack hints, check commands, include/exclude, generated dirs, behavior defaults. Không ghi auth, absolute home hoặc kết quả test transient vào file team config.
2. `inspect` chỉ đọc manifests/lockfiles/Makefile/CI để đề xuất và chỉ nguồn bằng chứng; không chạy discovered script để đoán. `init` preview config trước ghi, không overwrite config người dùng. Khác biệt đề xuất→được user xác nhận phải rõ.
3. Check command khai báo executable + argv + cwd tương đối + timeout + allowed environment names; mặc định không `shell -c`/string interpolation. Repo script có thể thực thi tùy ý nên config có mặt không tự tạo quyền chạy. Runner require trust/explicit command execution policy; --yes cài kit không đồng nghĩa cho phép chạy repo scripts.
4. Monorepo: map changed path→workspace→checks, explicit fallback full check nếu không xác định được; multi-language scopes giữ riêng. Ưu tiên explicit project config, rồi conventions được xác nhận; inferred hints chỉ là đề xuất. Không dò filesystem ngoài project để thu context.
5. Quy định phân biệt human-owned project config với generated context. Skills tham chiếu profile qua tài nguyên phù hợp, tránh nhét toàn bộ manifest vào always-loaded instructions. Nguồn context và quyết định fallback phải được explainable.
6. Test malformed/unknown schema, path traversal, symlink, workspace ambiguity, Windows argv, monorepo, project config thay đổi kể từ trust lần trước. Test read-only inspect/dry-run không ghi cache/temp/target.

**Done:** từ repo mới có bản đề xuất review được; agent nhận đúng test/lint commands và phạm vi workspace; chưa có command nào bị thực thi chỉ vì manifest/config được đọc.

### B04 — Nâng nội dung Engineer Kit dựa trên kết quả

**Phụ thuộc:** B02 baseline; B03 nếu nội dung dùng project profile.

1. Thay ví dụ `internal/apperr`, `AppendBlock` và yêu cầu hiểu DTKit bằng ví dụ repository-neutral; giữ các trường template hợp lệ. Chuẩn hóa ví dụ Go/TS/Python ở resources, không dồn vào description skill.
2. Rules/skills yêu cầu đọc project profile và repo docs phù hợp; report chính xác command chạy/chưa chạy, exit code, file đã thay đổi, limitation. Khi không có profile, khám phá conventions và ghi uncertainty thay vì tự bịa test command.
3. Review checklist theo case: validation/auth/path/input handling, concurrency/race/cancellation, edge cases/null/empty/overflow, backward compatibility, migration, tests. Skill chỉ nạp checklist liên quan; không ép mọi task nhỏ qua full questionnaire.
4. Tách instruction dùng chung ngắn và resources on-demand. Đo context footprint, trigger accuracy/over-trigger, conflict với AGENTS/CLAUDE cá nhân. Prompt injection fixture không được thay đổi scope/quyền chỉ vì xuất hiện trong file repo.
5. Freeze nội dung baseline trước chỉnh. Dùng development tasks để tune, giữ held-out tasks chưa dùng để viết skill; compare baseline kit với revised kit cùng runtime/model/settings. Khi muốn đo cả project profile/hooks, chạy ablation riêng để biết cải thiện đến từ đâu.
   So sánh no-kit vs candidate cần 60 runs; nếu muốn tuyên bố candidate tốt hơn kit cũ, thêm arm kit cũ cùng cấu hình (tổng 90 runs cho ba arms), hoặc chạy thí nghiệm riêng. Không dùng số liệu khác model/ngày/cấu hình làm đối chứng mà không công bố giới hạn. Trong phép đo content-only, giữ quyền và công cụ kiểm tra giống nhau; trong phép đo full-product, công bố rõ profile/checks/hooks là phần can thiệp.
6. Mọi content change tăng kit version; test rendered links, frontmatter, output golden, upgrade/remove/recover từ bản cũ. Nếu baseline là kit 0.3.0 adapter-only, content candidate có thể 0.4.0; CLI v0.5 không buộc kit phải mang version 0.5.

**Done B04:** có diff nội dung, lý do gắn development case fail, structural tests và development comparison; đóng băng candidate để B08 chạy held-out. Kết quả không cải thiện thì ghi thật và điều chỉnh trước claim; không ghi pass dựa trên các câu “không bịa” có mặt trong template.

### B05 — Quality runner dùng được cả bằng tay và trong CI

**Phụ thuộc:** B03. **Public command đề xuất:**

```text
dtkit check --path <repo> --dry-run --json
dtkit check --path <repo> --changed --json
dtkit check --path <repo> --all --json
```

1. Runner dùng project profile đã xác nhận: executable/argv/cwd, timeout/cancel, bounded stdout/stderr; không truyền credential vào report. Chạy check thay đổi theo workspace; final/full checks độc lập với agent.
2. `--changed` phải có semantics rõ cho staged/unstaged/untracked/base ref, đường rename/delete, ignored/generated; nếu không suy ra phạm vi an toàn thì báo hoặc full check theo policy đã chọn. `--all` chạy full configured checks. Dry-run chỉ show plan.
3. Trạng thái per check: pass/fail/skipped/missing_tool/timeout/cancelled; missing formatter/linter không được tính pass. Giữ exit code existing; thêm stable error codes cho quality checks để JSON consumer phân biệt lỗi test và lỗi setup. Quyết định mapping bằng ADR trước publish CLI contract.
4. Default chạy checks không sửa file. Auto-fix opt-in rõ, phạm vi files allowlist, snapshot diff sau chạy; formatter không được silently thay code ngoài task. Chặn overlapping runs trong một workspace khi checks có mutation; kill đúng child process tree khi timeout.
5. Sắp checks nhanh→chậm; không cần cache kết quả trong bản đầu. Nếu thêm cache sau, key phải gồm source/config/toolchain, không reuse pass sau file đổi.
6. Report chỉ kết luận dựa trên checks thực chạy; output file vào ignored operational dir khi người dùng yêu cầu, không chứa source/secret mặc định. CI có thể upload report đã redact.

**Done:** check runner chạy được trên fixture ba stacks từ CLI và CI; tests cho timeout, missing tool, output lớn, failed command, path/argv quoting, changed workspace và auto-fix scope. Kết quả độc lập agent nên phát hiện false completion.

### B06 — Claude quality hooks đầu tiên, sau đó mới mở rộng runtime

**Phụ thuộc:** B01, B05. **File:** internal/hooks, installer hooks/transaction/schema, adapter/claude, CLI hooks commands.

1. Kiểm tra hook API thật trên Claude version pin. PreToolUse dùng cho quyết định trước chạy; PostToolUse chạy sau tool nên không dùng để tuyên bố đã ngăn tác động. Luôn tuân native permission/sandbox; hook rule không là sandbox tổng quát.
2. Start nhỏ: PostToolUse với sự kiện sửa file → lint/check nhanh chỉ file/workspace liên quan; optional format đã explicit opt-in. Final gate/Stop event chỉ thêm sau khi test cách runtime retry/stop để tránh chặn phiên vô hạn.
3. PreToolUse policies ban đầu giới hạn mục tiêu cụ thể (ví dụ paths không cho ghi hoặc command đã biết); chốt behavior unknown shell syntax. Không dựng regex blacklist rồi quảng cáo ngăn mọi lệnh nguy hiểm. Dùng structured input/native permissions khi khả thi.
4. Hệ thống cần debounce, recursion guard, per-workspace concurrency, timeout, output cap, failure policy warning/block rõ. Không tự gọi model trong hook làm tăng chi phí hoặc tạo vòng lặp.
5. Structural merge `.claude/settings.json` theo scope đã xác minh; giữ unrelated settings/user hooks, digest ownership, duplicate guard, trust review. State schema mở rộng cần backward compatibility cho Codex hooks cũ và transaction rollback/recover.
6. Live sequence: configured→native trust→tool edit event→check invocation→result model nhận→disable→không còn invoke. Test format gây edit không kích hoạt vòng lặp; project/global không double-run; uninstall hook giữ personal config.

**Done:** ít nhất một quality hook Claude chạy thật, dùng chung B05 runner với CLI/CI, không gây loop hoặc overwrite user config. Advanced dangerous-command policies và runtime khác là C05, không chặn MVP quality hook.

### B07 — N15: preset/component selection trước multi-kit

**Phụ thuộc:** B04 ổn định content, state migration plan; có thể song song B06 nếu không chạm cùng schema. **Command đề xuất:** install/update nhận `--preset` hoặc `--components`; selector conflict là usage error.

1. Manifest khai báo preset IDs và component roots; deterministic dependency closure. Presets ban đầu `minimal`, `review-only`, `engineering`; định nghĩa chính xác skills/resources của từng preset trong schema/examples.
2. State lưu user selection + resolved component set + package digest/adapter revision. Omit selector khi update giữ lựa chọn hiện tại; user explicit đổi preset tạo plan add/remove. Không biến install có state thành implicit update trái contract.
3. Render từ selection closure: resource còn được selected component dùng phải giữ; drop resource khi không còn reference và DTKit sở hữu. Component removed upstream là conflict/explicit plan theo contract, không silently mở lại full kit.
4. `validate`/`info`/`pack` validate mọi preset; tests cycle/missing ref, overlapping presets, changes upstream, drift và no-op. Missing skill reference sau filter phải fail render.

**Done:** review-only thực sự ít skills/context hơn full kit, update không quên lựa chọn; migration từ state full-kit cũ mặc định full selection và có rollback. Benchmark so preset trên task phù hợp để kiểm giảm nhiễu có lợi thật.

### B08 — Release v0.5 và gate chất lượng

**Phụ thuộc:** B01–B07; giữ scope RC/hook runtime khác rõ.

- Full lifecycle tests cho năm runtime, schema/package backwards compatibility, shared locks, source read-only, native filesystems và hooks vẫn pass.
- Ít nhất Claude hoặc một runtime khác có live task/evaluator pass trong môi trường hợp lệ, project và global claims đúng evidence riêng.
- Benchmark hoàn tất tối thiểu 10 tasks × 3 repeats × 2 arms trên bộ held-out: baseline + with-kit, blocked count, scope violations/test tampering/false completion; báo liệu đã đạt mục tiêu hay chưa. Nếu thiếu runs, candidate vẫn partial; nêu số đã chạy và gate còn mở.
- Project profile/quality runner hoạt động ngoài agent; hooks có invocation + disable evidence. Presets giữ selection qua update.
- Upgrade RC→v0.5, archives/checksums/native smoke/cài terminal test như A; docs không hứa stable cho runtime chưa kiểm chứng.

**Done:** candidate v0.5 có report code correctness và agent task quality riêng; owner có thể review để publish. Nếu benchmark/tool blockers chưa giải quyết, đánh partial/experimental thay vì tick mọi task verified.

## 6. Mốc C — Mở rộng sau khi có một vòng chất lượng chạy được

### C01 — N14: multi-kit, namespace và sở hữu dữ liệu

Thiết kế ADR trước code: state registry theo (runtime, scope, kit), logical artifact owner, dependency/shared resources, lock ordering và migration old names. Gợi ý namespace mới chứa kit ID nhưng không rename skill cũ tùy tiện nếu user đang gọi; định nghĩa compatibility alias hoặc migration explicit.

Tasks: cài engineer + go/react/security-review trong cùng scope; resolve duplicate skill names/rules/capability conflict; update/remove riêng từng kit; resource reference counts hoặc ownership không share; transaction multi-artifact giữ atomic-recovery semantics. Test hai kits cùng `skill/review`, update kit A trong lúc B chạy, conflict rule không silently chọn winner, remove A giữ B và user config. Mọi state/schema cũ phải migrate và recover được. Nghiệm thu bằng hai kit thật + fixture crash, không chỉ list nhiều kit.

### C02 — Xem diff và tùy chỉnh kit có kiểm soát

Thêm preview content diff cho install/update, bounded output/redaction, explain source + version + selector + ownership. Hỗ trợ export/fork sang package local để user chỉnh và tăng version; giữ separation managed vs project overrides. Chưa thêm auto-adopt file drift nếu không có contract ownership rõ. Test update sau fork, duplicate IDs, user edits, local references và source privacy. Mục tiêu: người dùng không buộc sửa file managed rồi mắc kẹt ở update.

### C03 — N13: lockfile, nguồn tin cậy, reproducible installation

`dtkit.lock` là portable project declaration khác installation state/journal; pin kit/version/package digest/archive digest/selection/runtime adapter compatibility. Không chứa absolute home, tokens hoặc runtime auth. Chốt semantics source unavailable/offline/platform-specific mapping trước implement. Tách schema declaration vs resolved manifest; fresh clone hai máy cài cùng bytes từ verified cache.

Publisher signatures/provenance: chọn trust root/key management, verify before mutation, key rotation/revocation và offline policy có test; không tự thiết kế cryptography. Hash bảo vệ integrity, signature xác thực identity theo trust policy, cả hai không chứng minh nội dung skill/hook an toàn. Lockfile review phải hiển thị source/version/content change; public registry/marketplace chưa cần cho milestone này.

### C04 — N17: team onboarding và completion

Export/import config portable, inspect tất cả runtime trong một report, completion bash/zsh/PowerShell, template onboarding cho repo. Export không chứa state/binding/backup/auth riêng máy; import có preview/capability checks và không tự chạy project commands. Test hai workspace/home khác nhau và CI clean checkout. Có thể ship completion trước multi-kit nếu không đổi contract; team sync dựa trên C03 lockfile.

### C05 — N16: quality integrations cho các runtime khác

Lần lượt Codex/Gemini/Cursor/OpenCode theo nhu cầu: ghi official event/protocol/trust/path per version; OpenCode plugin khác shell hook nên thiết kế adapter riêng. Reuse B05 quality runner, giữ runtime-specific setup/ownership. Acceptance mỗi integration: event thật, output hiểu được, giới hạn time/loop, project/global dedupe, update/remove/recover giữ user settings. Không cần hook parity tuyệt đối; capability matrix công bố phần thiếu.

### C06 — N12/N18: compatibility định kỳ và performance

Public CI có thể chạy discovery/layout/config/native tests không cần model key; version pin + scheduled/manual test bản mới ở branch riêng, tạo report/diff để maintainer xem, không auto-upgrade adapter hoặc publish. Live model eval chạy trong môi trường riêng có budget/credentials permission, logs đã redact. Theo dõi token/latency/task pass theo model+runtime+kit version; set regression budget có baseline trước, không ngưỡng tùy ý. Giữ last-known-good compatibility reports và chính sách deprecation/runtime version support.

## 7. Thứ tự thực thi, ranh giới schema và giao việc cho nhiều AI

Luồng chính: **A01–A04 → B01 → B02 baseline → B03/B04 → B05 → B06/B07 → B08 → C01–C06 theo dependency**.

Một số việc song song được phép: chuẩn bị fixtures B02 cùng adapter B01; thiết kế schema B03 khi chưa có live budget; content audit B04 trước benchmark nhưng không overwrite frozen baseline; B07 và B06 dùng branch riêng, một integration owner điều phối schema.

| Lane | Sở hữu file/task | Điểm đồng bộ |
|---|---|---|
| Release | A, docs, scripts/workflows | Không kéo feature schema mới vào RC |
| Runtime | B01 + native Claude reports | Freeze adapter semantics trước live benchmark |
| Evaluation/content | B02/B04, test fixtures, reports | Tách evaluator tests khỏi agent workspace; freeze baseline |
| Project/checks | B03/B05, profile schema/process runner | Approve command contract và trust policy trước hook integration |
| Hook/preset | B06/B07 | Shared state/schema migrations review cùng installer owner |

Mỗi AI có branch/worktree riêng; không cùng ghi một checkout. Mỗi PR nhỏ có problem→behavior→test evidence. Không rename/reformat toàn core khi chỉ thêm profile. Mỗi schema change cần fixtures cũ, forward-unknown error rõ, no-op/dry-run invariant, transaction recovery tests.

### 7.1. Bản đồ code để bắt đầu đúng chỗ

| Phần | Điểm đọc/sửa hiện có | Đầu ra mới hoặc contract cần chốt |
|---|---|---|
| Runtime Claude | `internal/adapter/adapter.go`, `render.go`, các adapter con; `internal/cli/runtime.go`, `internal/paths/paths.go`, `internal/installer/runtime.go` | Profile Claude, renderer theo pattern hiện có; không tạo engine cài đặt thứ hai |
| Kit và presets | `internal/catalog/engineer/package.json`, `skills/*/SKILL.md.tmpl`, rules/workflows/resources; `internal/catalog/validation.go`, `digest.go` | Preset/dependency schema, kit version mới, selection renderer; dùng resource layout thật của package |
| State/migration | `internal/state/{state,load}.go`, `schemas/{state,package,cli-output}.schema.json`, `internal/installer/{engine,install,update,remove,recover}.go` | Version/migration fixtures; không chỉnh state bằng thao tác JSON rời khỏi transaction |
| Hooks | `internal/hooks/{merge,command,runner}.go`, `internal/installer/hooks.go`, `internal/cli/hooks.go` | Runtime-specific event/config adapter gọi chung quality runner |
| Evaluation | `docs/evaluation/`, `internal/adapter/engineer_eval_test.go` | Công cụ runner dev, fixture repositories và run/report schema; structural test giữ riêng live evaluator |
| Project profile/checks | `internal/cli/` cùng JSON/error conventions hiện có | Có thể thêm `internal/project`, `internal/check`, project schema và CLI handlers; đây là đề xuất, không phải thư mục đã tồn tại |
| Release | `.github/workflows/{ci,release}.yml`, `scripts/{build,package,verify-docker}.ps1`, `docs/release.md` | Safe install snippets/scripts và tests, candidate evidence |
| Team/multi-kit | Catalog/source resolver, state/installer, CLI export/import mới | Registry ownership, lockfile schema, portable declarations; không dùng installation state làm team config |

Trước mỗi schema/API change, ghi một quyết định ngắn trong docs: input/output, default, error codes, compatibility, migration và failure recovery. Review hợp đồng trước khi cho nhiều AI sửa cùng nhóm file. Nếu phát hiện tên/path trong bảng đã đổi, đọc code hiện tại rồi cập nhật bảng; không tạo file trùng chỉ để khớp plan.

### 7.2. Cách chia PR và điểm dừng có thể bàn giao

1. PR release: A01/A02 và quyết định A03; sau đó A04 có artifact/report của đúng SHA.
2. PR Claude: adapter/lifecycle/schema/package version cùng fixtures; native verification report riêng khi có môi trường.
3. PR evaluator: fake-runtime tests + fixture manifest + pilot + baseline frozen. Chưa thay nội dung kit trong PR này.
4. PR project profile → PR content → PR quality runner: theo dependencies; mỗi PR dùng được độc lập và nêu phần chưa nối.
5. PR Claude hooks và PR presets: integration owner xử lý migration chung, không merge hai schema version trùng số với semantics khác nhau.
6. PR v0.5 readiness: kết quả held-out, docs/compatibility, upgrade/native artifact evidence. C01–C06 lập PR riêng sau mốc này.

Không ước lượng “xong” chỉ theo số PR. Một task thiếu native/live evidence có thể đã merge code nhưng phải để trạng thái partial. Nếu input ngoài môi trường đang thiếu, bàn giao chính xác lệnh cần chạy, prerequisite và expected artifact; tiếp tục các task không phụ thuộc input đó.

## 8. Ma trận acceptance trọng yếu

| Case | Expected result |
|---|---|
| RC checksum sai, thiếu entry, wrong arch | Dừng trước replace; binary cũ nguyên vẹn |
| Codex HOME override khác OS profile | Report đúng đường thực discover, không giả global verified |
| Claude project/global/custom config/worktree | Skill và resources đọc đúng; scope/state không va runtime khác |
| 60 runs / 30 cặp từ fixture frozen | Results đủ task/arm/repeat hoặc explicit blocked; không carry-over global kit |
| Agent sửa evaluator tests hoặc báo xong giả | Run fail/tampering/false-completion được nhận diện |
| Project manifest có script chưa trust | Inspect chỉ đề xuất, không thực thi |
| Monorepo change một package | Checks đúng workspace; ambiguous mapping có fallback rõ |
| Check thiếu tool/timeout/output lớn | Trạng thái đúng, không pass giả, child processes được xử lý |
| Formatter hook tạo edit | Không loop; chỉ sửa allowed paths |
| Duplicate/user hook và uninstall | Không xóa config không sở hữu |
| Preset update/no-op/resource shared | Selection được giữ, resource cần thiết không bị xóa |
| Hai kits trùng review/rules | Namespace/conflict có contract; remove/update một kit không phá kit còn lại |
| Lockfile trên máy mới/offline | Cùng verified bytes, không dùng home/auth máy cũ |

Lệnh dev baseline hiện có: `git status --short`, `git log -1 --oneline`, `.\scripts\verify-docker.ps1`. Native CI vẫn cần fmt/test/vet/build ba OS, race và bounded fuzz phù hợp. Không gọi full model benchmark cho docs-only change. Mỗi lần run tool/test lưu completion exit status; container biến mất không tự chứng minh toàn verification script đã pass.

## 9. Tiêu chí chấm benchmark và cách ra quyết định

Primary metric là independent task acceptance, kèm điều kiện không tamper tests và không scope violation. Report thêm false-completion/fabricated execution và regression. Secondary metrics là latency, token/cost nếu đo được, số lần intervention/retry và độ rộng diff.

Với cùng 10 task × 3 repeats, trình bày pass/total mỗi arm và paired win/tie/loss theo task; ghi rõ n nhỏ, model/environment version và blocked runs. Tune trên development set, dùng held-out set cho quyết định. Không gộp điểm nhiều model/runtime khác nhau thành một tỷ lệ duy nhất.

Quyết định sau mỗi candidate kit:

- Nếu có scope violation/test tampering do kit hướng sai: sửa trước release stable, thêm regression case.
- Nếu pass tăng nhưng latency/cost tăng: trình trade-off bằng số liệu, không tự coi là thắng toàn diện.
- Nếu không cải thiện hoặc worse: giữ previous known-good content, kiểm trigger/context/config, rerun các case liên quan khi có thay đổi có cơ sở; không lặp vô hạn chỉ để tìm một run đẹp.
- Nếu blocked do environment/auth: sửa setup hoặc đổi môi trường được phép; không bỏ checks để vượt blocker. Tiếp tục harness/docs/code độc lập.

## 10. Mẫu tiến độ và prompt giao cho AI triển khai

Mỗi task ghi ở docs/progress:

```text
ID: Axx / Bxx / Cxx
Status: not-started | implemented | automated-tested | partial | verified | blocked
Baseline SHA / branch:
User-visible behavior and schema decisions:
Files changed:
Tests completed: commands, exit status, OS/runtime/model, CI URL
Live evidence: task, allowed scope, evaluator output, budget used (if applicable)
Acceptance still missing / external blocker:
Next independent work:
```

Prompt khởi động có thể dùng nguyên văn:

> Bạn tiếp tục DTKit từ code N00–N12 đã implement. Đọc DTKIT-RC-AND-V05-PLAN.md đầy đủ, docs/progress.md và các contract liên quan; kiểm Git/CI hiện tại trước khi sửa. Thực hiện A01–A04 để chốt RC với support labels đúng evidence, rồi B01–B08 theo dependency để có Claude live task, benchmark tái lập, project profile, Engineer Kit cải tiến, quality runner/hooks và presets. Không đánh N08/N09 full verified khi chỉ có discovery/structural tests. Các command/config/schema mới trong plan là đề xuất cần chốt contract và test trước khi public; không giả rằng chúng đã tồn tại. Reuse core installer/ownership/recovery, không rewrite không cần thiết. Freeze baseline trước content tuning, report đủ blocked/failed runs và không sửa evaluator để tăng score. C01–C06 là roadmap tiếp theo, chỉ làm khi mốc B và quyết định sản phẩm cho phép. Dùng branch/worktree riêng, cập nhật progress từng task với exit status/evidence thực; chuẩn bị PR/RC cụ thể trước khi cần owner chốt merge/tag/publish hoặc ngân sách model. Nếu runtime/auth bị chặn, hoàn thành công việc độc lập và ghi blocker, không bypass policy hoặc claim pass. Bàn giao với commit, checks đúng SHA, features thực dùng được, gate còn mở và bước kế.
