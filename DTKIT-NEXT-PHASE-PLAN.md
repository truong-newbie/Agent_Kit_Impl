# DTKit — kế hoạch giai đoạn tiếp theo: ổn định, kiểm chứng và phát hành

Ngày lập: 2026-11-16. Baseline mã nguồn: `be9e0bdfcbe095b74e186eeffc32f563b77644d8`, nhánh `feat/runtime-cursor-v0.4`.

> Tiếp nối ngày 2026-11-17: implementation N00–N12 đã nằm trên `feat/release-hardening` tại `c100b86`, nhưng live runtime tasks, benchmark A/B và phát hành còn gate cần đóng. Dùng [DTKIT-RC-AND-V05-PLAN.md](DTKIT-RC-AND-V05-PLAN.md) làm hướng dẫn nhận việc hiện tại. File này giữ contract và snapshot lịch sử; không làm lại N00–N12 chỉ từ checklist/prompt cũ.

Tài liệu này là đầu vào chính để giao AI tiếp tục sau khi P0–P6, H1, D1, R1–R3 đã có implementation. [Handoff cũ](DTKIT-PRODUCT-HANDOFF.md) phản ánh giai đoạn trước; không dùng các mục “chưa có update/remove/recover” ở đó để làm lại chức năng. [Plan gốc](DTKIT-IMPLEMENTATION-PLAN.md) vẫn là nguồn contract cho transaction, ownership, CLI và acceptance. Khi có khác biệt, ghi quyết định và test tương ứng thay vì âm thầm thay contract.

## 1. Mục tiêu và định nghĩa hoàn thành

Mục tiêu giai đoạn này: từ code có nhiều tính năng và CI xanh thành một bản phát hành có thể cài từ terminal, nâng cấp từ bản cũ, dùng thực tế trên runtime được công bố hỗ trợ, gỡ sạch phần DTKit và phục hồi khi gặp lỗi.

Không coi các tên v0.1/v0.2/v0.3/v0.4 trong roadmap là các release đã phát hành. Đây đang là tên các chặng phát triển; version CLI, version kit và adapter revision là ba khái niệm riêng, phải quản lý độc lập.

Kết thúc giai đoạn phải có:

- Các lỗi về phiên bản kit, dry-run nguồn remote và policy tải nguồn được xử lý và có regression test.
- Bằng chứng concurrency/recovery cho file shared, compatibility với state cũ, và native tests cho các OS được hỗ trợ.
- Runtime verification có version, scope, ngày, lệnh và kết quả thực; tính năng chưa kiểm chứng được gắn trạng thái experimental/partial rõ ràng.
- Một release candidate từ commit xác định, archive/checksum kiểm tra được, đường cài qua terminal có thể làm lại trên máy sạch.
- README, CLI help, compatibility matrix, release notes và progress cùng phản ánh đúng bản sẽ phát hành.
- Backlog sau phát hành tách khỏi các điều kiện bắt buộc; không cần làm marketplace hay thêm runtime thứ năm để hoàn thành giai đoạn này.

## 2. Hiện trạng đã xác nhận

| Vùng | Implementation hiện tại | Bằng chứng và giới hạn |
|---|---|---|
| Codex core | Install/idempotence/status/update/remove/recover, state, journal, lock, rollback, JSON/errors, project/global | Có test tự động; runtime project mới discovery một phần, global/full tool execution còn thiếu |
| Hooks | Codex SessionStart `session-context`, opt-in, merge/unmerge config, ownership, transaction integration | Shell/E2E tests có; Codex trust→event→handler thật chưa hoàn tất |
| Distribution | create kit/skill, pack, archive/digest, remote index, cache, offline | Có tests; còn những khoảng trống policy ở mục 3 |
| Gemini | Adapter, roots, instructions/skills, lifecycle, runtime state riêng | Chưa smoke trên Gemini thật theo tài liệu hiện tại |
| OpenCode | Adapter, shared AGENTS blocks, config roots, lifecycle | `1.19.5` đã discovery 7 project + 7 global skills; chưa test model làm task thật |
| Cursor | Project `.mdc` rule, skills hai scope, global instructions UI fallback | Chưa smoke trên Cursor thật theo tài liệu hiện tại |
| Release | MIT LICENSE, module GitHub chính thức, scripts build/package, CI workflow | Workflow chỉ upload Actions artifact; chưa có bước tạo GitHub Release trong workflow |
| Git | `main` tại `f81ce6e`; nhánh Cursor tại `be9e0bd` chứa chuỗi tính năng tiếp theo | Working tree sạch lúc bắt đầu lập tài liệu; kiểm tra lại khi nhận việc |

CI của `be9e0bd` đã được kiểm tra qua GitHub checks API: Windows, Ubuntu, macOS và race đều `completed/success`. [Run 43849330462](https://github.com/truong-newbie/Agent_Kit_Impl/actions/runs/43849330462). Đây là bằng chứng cho SHA đó, không phải cho các commit sẽ tạo về sau. Lần lập tài liệu này không chạy lại test suite hoặc runtime smoke.

Module hiện tại là `github.com/truong-newbie/Agent_Kit_Impl`, license MIT đã có. Không hỏi lại owner về hai việc này nếu repo không có thay đổi mới. Native Go trên máy Windows này từng bị Device Guard chặn; có Docker verification và native CI để tiếp tục.

## 3. Những việc còn thiếu và bằng chứng từ code

Quy ước: **Quan sát** = đã thấy trong source/diff; **Cần tái hiện** = nguy cơ cần test có kiểm soát; **Thiếu bằng chứng** = chưa được runtime/test hiện có chứng minh. Tài liệu này không thay cho một security audit hoàn chỉnh.

| ID | Mức độ | Phát hiện | Nơi bắt đầu kiểm tra |
|---|---|---|---|
| F01 | Chặn upgrade | Quan sát: manifest Engineer đổi `runtimeTargets` từ Codex sang bốn runtime nhưng vẫn `version: 0.1.0`; digest package thay đổi, xung đột quy tắc immutable version khi nâng từ main | `git diff main...be9e0bd -- internal/catalog/engineer/package.json`, `internal/installer/update.go` |
| F02 | Chặn contract dry-run | Quan sát: install/update resolve source trước nhánh dry-run; resolver gọi cleanup cache, DownloadBytes tạo temp, ResolveRemote gọi Put. `--dry-run --source` có thể ghi/xóa cache | `internal/cli/{source,install,update}.go`, `internal/dist/{remote,cache}.go` |
| F03 | Chặn remote stable | Quan sát: ValidateRemoteURL chỉ kiểm URL đầu vào; DefaultDeps dùng http.DefaultClient, không có timeout tổng riêng hoặc CheckRedirect riêng. Cần test redirect downgrade và server treo | `internal/cli/deps.go`, `internal/dist/cache.go` |
| F04 | Chặn khi tái hiện mất dữ liệu | Cần tái hiện: Codex/OpenCode có operational lock riêng nhưng có thể cùng sửa project AGENTS.md. Recheck-before-write chưa chứng minh không lost update giữa hai process | `internal/adapter/adapter.go`, `internal/installer/{engine,recover,lock}.go` |
| F05 | Cần hardening | Quan sát: CleanStaleDownloads xóa mọi `.part` tìm thấy và bỏ qua lỗi, resolver gọi cả ở offline. Chưa phân biệt file đang được process khác dùng | `internal/dist/cache.go`, `internal/cli/source.go` |
| F06 | Cần hardening | Quan sát: registry index dùng json.Unmarshal; selectIndexEntry chưa validate toàn bộ entries/duplicate ID+version; cache hit/offline có thể đi trước kiểm minCliVersion của index | `internal/dist/remote.go` |
| F07 | Chặn contract capability | Quan sát: kit requires instructions+skills; Cursor global chỉ cài skills/resources, RenderKit không có vòng enforce RequiresCapabilities. Cần chốt required khác optional/fallback | `internal/catalog/engineer/package.json`, `internal/adapter/{adapter,render}.go` |
| F08 | Chặn claim runtime stable | Thiếu bằng chứng: Codex global, Codex hook trust/event, Gemini, Cursor và OpenCode task thật chưa được xác nhận đầy đủ | `docs/verification/`, `docs/compatibility.md` |
| F09 | Chặn release-ready docs | Quan sát: README vẫn nói hooks/distribution/multi-runtime là deferred; progress có pending lịch sử; release checklist vẫn v0.1; default buildinfo vẫn `0.1.0-dev` | `README.md`, `docs/{progress,release,compatibility}.md`, `internal/buildinfo/buildinfo.go` |
| F10 | Chặn release pipeline | Quan sát: workflow nhận `inputs.version` trực tiếp trong shell script, mới có Linux archive smoke, upload artifact giữ 14 ngày; chưa publish GitHub Release | `.github/workflows/release.yml`, `scripts/{build,package}.ps1` |
| F11 | Cần kiểm chứng OS | Thiếu bằng chứng: CI xanh không tự chứng minh junction/locked-file/crash subprocess được test; một số symlink/process tests có Skip | `internal/paths/paths_test.go`, `internal/installer/*test.go` |
| F12 | Cần hardening config | Quan sát: hook JSON decoder chưa reject duplicate keys; unmerge xóa tất cả entries cùng digest. Cần xác định hành vi khi config bị nhân đôi/sửa | `internal/hooks/merge.go` |

## 4. Backlog theo thứ tự ưu tiên

Mỗi task dưới đây có đầu ra, tiêu chí và test cụ thể. AI cần cập nhật trạng thái `not-started / in-progress / verified / blocked` trong progress mới. “Có code” không đồng nghĩa “verified”. Các giới hạn cần owner quyết định phải được trình bày bằng phương án cụ thể, không dừng toàn bộ các việc độc lập.

### N00 — Đồng bộ baseline và hồ sơ tiến độ

Ưu tiên: đầu tiên. Phụ thuộc: không.

1. Đọc file này, docs/cli, compatibility, release, verification; xem contract tương ứng trong plan gốc và diff từ main. Kiểm tra working tree, remote refs, tag/release hiện có trước khi sửa.
2. Tạo nhánh `feat/release-hardening` từ nhánh đầy đủ hiện tại hoặc worktree riêng nếu repo đã có công việc khác; không bắt đầu từ main cũ rồi mất các adapters mới.
3. Thêm phần “Current status” đầu docs/progress ghi SHA/CI mới; giữ log lịch sử nhưng gắn nhãn snapshot theo thời điểm. Cập nhật README phần nào implemented, experimental, runtime verified.
4. Ghi ma trận feature vs runtime vs scope; tách bốn lớp bằng chứng: rendered file, CLI lifecycle test, runtime discovery, runtime task thực. Ghi rõ Cursor global instructions có fallback và hooks hiện chỉ do DTKit quản lý trên Codex.
5. Đăng ký các task N01–N12 với owner/task branch/evidence. Chưa thay lời công bố runtime supported thành verified khi chưa có test thật.

Đầu ra: status summary nhất quán, baseline SHA/run URL, backlog thực thi. Nghiệm thu: AI mới đọc README + progress hiểu đúng nhánh cần dùng và các gate đang mở. Check: Markdown links, git diff --check; không cần chạy full suite chỉ vì sửa tài liệu.

### N01 — Version kit, upgrade từ bản cũ và compatibility state

Ưu tiên: bắt buộc trước release. Phụ thuộc: N00. Xử lý F01.

1. Lấy fixture Engineer và state từ commit main `f81ce6e` trong checkout/worktree tách biệt. Không sửa state người dùng để giả lập upgrade. Tạo installation thật trong temp project và global home bằng binary cũ khi khả thi.
2. Tăng kit version cho manifest/content đã đổi; giữ quy tắc same-version/different-digest phải fail. Chọn CLI release version riêng, adapter revision chỉ tăng khi semantics render/layout thay đổi.
3. Test binary mới update installation cũ thành công, giữ personal AGENTS bytes, timestamps theo contract và hook registrations. Test install idempotent sau upgrade. Adapter cũ/mới và Codex state v1/v2 vẫn đọc được; unknown future schema báo lỗi hữu ích.
4. Tạo fixture cho từng state runtime hiện có; test status/update/remove/recover. Giữ golden state/journal cũ để hồi quy sau này. Nếu đổi schema, thiết kế migration + rollback rõ trước khi code.
5. Cập nhật changelog và chỉ dẫn nâng cấp bản dev đã cài cùng version/digest khác. Không đưa hướng dẫn “xóa state rồi cài đè”; nếu không thể upgrade một dev snapshot, dùng remove có ownership rồi install và nêu giới hạn.

File chính: `internal/catalog/engineer/package.json`, `internal/state/`, `internal/installer/update*.go`, `internal/cli/*test.go`, docs/release.

Nghiệm thu: upgrade main→candidate qua executable thành công ở cả project/global; same-version tampering vẫn bị chặn; user files không đổi. Có fixture/test name, command, OS và output summary.

### N02 — Source resolution không ghi khi preview/read-only

Ưu tiên: bắt buộc trước release remote. Phụ thuộc: N00. Xử lý F02/F05.

1. Tách resolve/download/validate khỏi persist cache/cleanup. Truyền policy đọc/ghi rõ ràng từ CLI, không để resolver tự suy dựa vào command string.
2. Chốt theo contract hiện có: dry-run không tạo/xóa/sửa target, cache, temp, lock hoặc state; remote bytes giới hạn có thể đọc vào memory. `info`/`validate` với remote cũng đọc mà không tự cleanup cache. Network chỉ chạy nếu user chỉ rõ nguồn; offline tuyệt đối không gọi network.
3. Persist cache chỉ ở bước được phép ghi của mutation đã xác nhận; cache hit có kiểm digest. Nếu no-op thì không rewrite cùng blob hoặc timestamp. Source snapshot dùng cho plan và apply phải cùng bytes.
4. Bỏ cleanup `.part` vô điều kiện trong resolver. Cleanup chỉ đụng file do request sở hữu; GC riêng phải phân biệt active download và stale evidence. Chưa cần thêm public GC command nếu không cần cho release.
5. Test snapshot toàn temp project/home/cache trước-sau cho dry-run remote (cache miss/hit), info/validate, offline, cancel prompt, missing --yes, source invalid. Thêm trap HTTP client cho offline. Check trực tiếp số lần ghi/cleanup bằng injected boundaries nếu cần.

Nghiệm thu: không ghi/xóa bất kỳ file nào trong preview; output “Nothing was written” đúng thực tế; offline cache read không phá `.part` của process khác.

### N03 — Remote transport, registry và cache integrity

Ưu tiên: bắt buộc trước công bố remote ổn định. Phụ thuộc: N02. Xử lý F03/F05/F06.

1. Dùng HTTP client có timeout hữu hạn/cancellation, giới hạn redirect; validate scheme/URL ở mọi hop. Giữ loopback HTTP cho integration tests/local registry nếu contract có ghi rõ, chặn HTTPS→HTTP ngoài loopback. Test bằng httptest TLS và CA riêng; không vô hiệu hóa TLS verify.
2. Không đưa credential URL/userinfo/token query vào error/log. Chốt redirect cross-host và header credential policy. Không tự thêm DNS/private-network block rộng nếu sản phẩm cần registry nội bộ; ghi policy nguồn được hỗ trợ.
3. Registry index strict: reject duplicate JSON keys, invalid schema, invalid IDs/versions/digests/URLs, duplicate ID+version, giới hạn entries. Chọn highest valid semver deterministically; invalid entry phải lỗi rõ theo policy, không chọn ngẫu nhiên theo thứ tự index.
4. minCliVersion phải có semantics thống nhất cho index, direct archive, cached/offline. Nếu yêu cầu chỉ nằm trong index thì lưu metadata đã kiểm với blob hoặc đưa requirement vào manifest version mới; không để offline bypass điều kiện mà online đã áp dụng. Quy định dev/prerelease version rõ và test.
5. Cache.Put cùng digest phải idempotent trên Windows/POSIX. Hai process tải cùng digest chỉ nhận complete verified blob; không lỗi do rename target có sẵn, không xóa file active của nhau. Test interruption, corrupt cache, digest mismatch, timeout, malformed index.
6. Giữ bounded archive processing; bổ sung fuzz cho archive/index, test decompression/header-only flood và duplicate entries nếu chưa có. SHA-256 xác nhận bytes, không được mô tả như xác thực publisher. Signature/provenance nâng cao để N13.

File: internal/dist/{remote,cache,archive}.go, internal/cli/deps.go, source.go, dist tests.

Nghiệm thu: redirect không vượt policy, request không treo vô hạn, pinned digest mismatch fail trước install, offline có behavior nhất quán, parallel cache tests pass native Windows/Linux.

### N04 — Đồng thời nhiều runtime và recovery trên file shared

Ưu tiên: bắt buộc xác minh; nếu tái hiện lost update thì chặn release. Phụ thuộc: N00, N01 fixtures. Xử lý F04.

1. Viết test có barrier điều khiển hai process Codex/OpenCode cùng read trước write vào AGENTS.md. Thử install/install, update/update, remove/update, recovery/update. Sequential coexistence tests hiện có không thay cho các test này.
2. Nếu lock riêng runtime không đủ, chọn lock theo scope/root chung hoặc theo canonical shared target; quy định thứ tự acquire để tránh deadlock. State/journal runtime vẫn tách; lock chung chỉ điều phối mutation DTKit có thể đụng cùng file.
3. Mọi đường apply/recover/hooks có đụng shared config phải tuân protocol lock; không để recover restore whole-file trước-image và làm mất block runtime khác vừa commit. Khi current khác after-image, giữ evidence và báo conflict.
4. Test global roots bị alias/cùng directory, case-insensitive path Windows, PID reuse/unknown owner, stale lock sau process chết. Đừng xóa lock dựa trên tuổi. Hành vi với editor ngoài DTKit vẫn conditional rollback, nêu giới hạn TOCTOU thực tế.
5. Kiểm tra state revalidation dưới lock: hiện engine so generation + kit digest; test sửa root binding/artifact metadata mà giữ hai trường đó và quyết định strict state snapshot comparison phù hợp.

File: internal/installer/{engine,lock,recover,journal,runtime}.go, adapter profiles, planner/block.go.

Nghiệm thu: có kết quả deterministic hoặc conflict an toàn; không lost block, không commit state healthy khi artifact đã mất; concurrent recover không overwrite dữ liệu runtime khác. Ghi regression test tái hiện hoặc evidence rằng thiết kế hiện tại đã an toàn.

### N05 — Capability contract và status chính xác

Ưu tiên: bắt buộc trước công bố bốn runtime. Phụ thuộc: N00; trước final runtime verification. Xử lý F07.

1. Định nghĩa required capabilities phải được DTKit cung cấp trong scope chọn. Runtime tự có UI capability không đồng nghĩa installer cung cấp được capability đó.
2. Với Cursor global, thiết kế explicit optional/fallback trong package schema hoặc command contract; kit thật sự require instructions phải fail trước ghi khi DTKit không quản lý được. Engineer Kit có thể hỗ trợ skill-only fallback nếu contract khai báo rõ. Không dùng warning để bỏ qua một required capability.
3. Warnings/capabilities phải có trong dry-run, apply, no-op, info, status/doctor và JSON; hiện warning global fallback có thể chỉ xuất hiện muộn ở apply. Cùng request phải cho cùng thông tin trước/sau cài.
4. Tách trạng thái file `healthy` với integration `unverified`/`partial`/`verified`. Đây là metadata evidence, không biến every status thành network request. Doctor phát hiện executable/config không được tự suy rằng runtime load đúng skills.
5. Test package required instructions/skills ở mọi runtime/scope; unknown capability, unsupported hook/plugin, absent binary và global fallback. Cập nhật schema, docs và migrations nếu cần.

Nghiệm thu: không có install success bị thiếu capability bắt buộc mà chỉ cảnh báo; người dùng biết ngay preview thực sự cài gì.

### N06 — Hook ownership, parser và executable lifecycle

Ưu tiên: bắt buộc cho Codex hook stable. Phụ thuộc: N04 nếu sửa lock, N05 contract. Xử lý F12 và chuẩn bị runtime smoke.

1. Parse hooks JSON strict với duplicate-key detection và size limit; test numeric values/unrelated events được giữ đúng semantic qua merge. Unknown schema fail trước ghi.
2. Xác định trường hợp nhiều entries cùng digest: mặc định báo conflict/ambiguity; không xóa tất cả các bản sao chỉ vì digest giống nhau. Disable/remove chỉ gỡ ownership xác định.
3. Test đường executable chứa spaces, Unicode, dấu nháy, `$`, backtick; command chạy thực ở shell được runtime dùng. Xác minh binary nằm nơi ổn định, permission execute POSIX, binary missing/moved và upgrade executable khi hooks đã bật.
4. Test timeout stdin, malformed payload, stale/missing state, stderr/stdout contract; hook không gây session treo hoặc leaked goroutine trong test process. Giữ limit stdin/context output và không dùng payload cwd làm đường truy cập tin cậy.
5. Test update/remove khi hook enabled, modified entry, unrelated user hooks, project/global duplicate event guard. Configured/trust-pending/invoked phải có evidence khác nhau.

File: internal/hooks/, internal/installer/hooks.go, internal/cli/hooks.go và tests.

Nghiệm thu: không mất user hooks trong mọi fixture; lifecycle giữ stable executable và error behavior được tài liệu hóa; trust/event proof bổ sung tại N08.

### N07 — Kiểm thử OS và crash thực tế

Ưu tiên: bắt buộc theo OS sẽ phát hành. Phụ thuộc: N01–N06 liên quan đã ổn định. Xử lý F11.

1. Lập danh sách test capability đang Skip trên từng CI runner; không coi một test đã bị skip là pass. Tạo Windows-specific fixtures cho junction/reparse, sharing violation/file lock và case collision; test rename giữ original khi fail.
2. Test Unicode/spaces, CRLF/BOM, Unix permissions, long path theo giới hạn công bố, home/config root khác volume. Parent confinement kiểm tra cả operational dirs, state, journal, backup, cache.
3. Ngoài fault injection trong cùng process, dùng child process dừng sau prepared/action/state commit, kill, khởi động process mới recover. Chỉ kill child của fixture. Test chạy lại recovery là no-op và giữ external editor edits.
4. Retention giữ unresolved transactions, chỉ prune completed đã validate. Test malicious journal backup path và corrupted state không thể dẫn đến đọc/xóa ngoài allowed root. So file bytes ngoài vùng sở hữu trước/sau.
5. CI: giữ fmt/test/vet/build ba OS và race Linux; thêm bounded fuzz cho parser/state/path/archive/index theo gate hợp lý. Full suite mỗi PR; không lặp tests khi không có thay đổi liên quan. Lưu kết quả native tests và skip reasons.

Nghiệm thu: target OS có native evidence cho những primitive filesystem quan trọng; chỗ chưa làm được được đánh dấu unsupported/experimental cụ thể trong release, không chỉ ghi “CI xanh”.

### N08 — Runtime verification thực tế, một checklist cho từng runtime

Ưu tiên: bắt buộc cho runtime tuyên bố stable. Phụ thuộc: N01, N05, N06; có thể chuẩn bị môi trường song song với N02–N04. Xử lý F08.

Các bước chung:

1. Kiểm tra bản runtime và tài liệu chính thức vào ngày test; lưu URL/version/date vào report. Đừng coi path trong adapter là bằng chứng runtime hỗ trợ. Tạo fixture repository với lỗi nhỏ và unit test có expected result.
2. Chọn môi trường cô lập thực sự: test user/VM/container được runtime hỗ trợ. Fake HOME chỉ đủ nếu đã xác nhận runtime lấy home từ biến đó; đừng chạm global configuration thật để làm smoke.
3. Test project-only, global-only, cả hai scope và coexistence với runtime khác. Xác nhận đúng skill names/path, instructions precedence, resource references, reload/new-session behavior.
4. Có tối thiểu task `plan` và `review` đọc code không đổi file; task `implement` sửa lỗi fixture và test pass; task `debug` nêu cause bằng evidence. Trước/sau so filesystem. Model outcome có tính biến thiên: lưu prompt/model/version, đánh giá tiêu chí, không chỉ kiểm phrase “I used skill”.
5. Update kit rồi mở session mới xác minh nội dung mới; remove rồi xác minh entry DTKit mất, user entry còn. Chụp log cần thiết, lọc token/secret, không lưu auth vào repo.

| Runtime | Khoảng trống cụ thể | Điều kiện nghiệm thu |
|---|---|---|
| Codex | Project đã discovery nhưng tool read bị host policy chặn; global chưa test; hook trust chưa test | Workspace đọc/sửa fixture được trong sandbox cho phép; global-only + combined; trust review→SessionStart invokes exactly once→disable không invoke |
| Gemini | Chưa có executable/runtime smoke theo snapshot | Pin executable/version; xác minh home resolution, settings context.fileName, instructions+skills cả hai scope; thực hiện task thật |
| OpenCode | Discovery đã có, chưa model call | Session có model đọc đúng skills/resources, dùng đúng AGENTS block, chạy task/test; coexistence không load trùng gây side effect |
| Cursor | Chưa có runtime smoke; global User Rules qua UI | Kiểm chứng phiên bản Cursor thực tế (IDE/Agent CLI đúng sản phẩm), `.mdc` application và skills discovery; global fallback rõ; personal rules vẫn áp dụng |

Đầu ra: `docs/verification/<runtime>-<version>-<date>.md` cùng fixture/script có thể lặp lại. Report gồm dtkit SHA, binary hash, runtime/model/OS, scope, setup, command, observed output, pass/fail/skip và cleanup.

Nếu thiếu executable/auth/access: hoàn thiện harness và mọi kiểm thử local độc lập, báo blocker cụ thể; không gọi model có chi phí chưa được cho phép. Runtime đó vẫn experimental/partial. Không tắt Device Guard hoặc bypass sandbox để biến một test thành pass.

### N09 — Đánh giá chất lượng Engineer Kit

Ưu tiên: bắt buộc có baseline nhỏ trước stable; mở rộng sau. Phụ thuộc: N08 có môi trường thực chạy.

1. Tạo bộ 8–12 task nhỏ gồm ask/plan/implement/debug/review/test/explain; ghi expected behavior và scope được phép ghi. Task có lỗi thật, thông tin thiếu, user yêu cầu review-only, và fixture có instruction xung đột để test giữ phạm vi.
2. Chạy baseline không kit và with-kit cùng model/settings/task. Ghi sample size, số lần chạy, latency/token khi runtime cung cấp, task pass, scope violation, lỗi bịa file và chất lượng output. Có thể chọn vài task lặp 3 lần để quan sát biến thiên; không kết luận tỷ lệ cải thiện từ một mẫu.
3. Chấm theo rubric: đúng yêu cầu, evidence từ file thật, plan có test, implement scope vừa đủ, review findings có vị trí/tác động, debug cause được tái hiện, test thực chạy. Report scope violations phải bằng 0 trên bộ acceptance trước stable.
4. Sửa skills/rules/workflows/templates dựa trên case fail; tránh nhồi cùng instruction vào mọi skill. Nếu đổi nội dung kit thì tăng version và chạy N01 upgrade fixtures.

Đầu ra: `docs/evaluation/` chứa task specs/rubric/results, content revision và limitation. Nghiệm thu: chứng minh được kit hoạt động đúng scope và tài nguyên đọc được; không hứa “nâng chất lượng” nếu chưa có kết quả so sánh.

### N10 — Release pipeline và trải nghiệm cài qua terminal

Ưu tiên: bắt buộc phát hành thật. Phụ thuộc: core gates và version từ N01; chuẩn bị song song với N08. Xử lý F09/F10.

1. Đề xuất một candidate version sau khi xem tag/release thực tế; đề xuất mặc định `0.4.0-rc.1` cho snapshot đa runtime nếu version này chưa dùng, nhưng không coi tên milestone là quyết định release đã được owner chốt. CLI version/kit version/adapter revision xuất hiện đúng trong metadata.
2. Validate version input trước shell/path/ldflags; truyền workflow_dispatch input qua environment, không nội suy trực tiếp vào shell source. Scripts PowerShell kiểm output path nằm đúng thư mục build, version không chứa path separator/shell payload. Build từ commit sạch, provenance ghi SHA.
3. Build tối thiểu target hiện có: Windows amd64, Linux amd64, macOS arm64. Mỗi archive được extract/run smoke trên native OS tương ứng. Thêm macOS amd64/Linux arm64 chỉ khi có nhu cầu và test runner; đừng công bố cả arch chưa chạy.
4. Archive kèm README/license/version, SHA256SUMS tạo từ đúng assets. Verify binary `version --json`, offline bundled install/status/remove bằng binary đã extract. Build RC mới từ current source, không dùng lại archive `0.1.0-rc1` cũ.
5. Workflow hiện tại chỉ upload Actions artifacts: bổ sung draft GitHub Release hoặc quy trình publish riêng, permissions tối thiểu, xác minh tests cho đúng SHA. Release notes có supported/experimental matrix, breaking changes, upgrade guide và known issues. Tag/merge/publish chỉ thực hiện theo quyền đã có; có thể chuẩn bị đầy đủ draft/PR trước.
6. Hướng dẫn terminal cho Windows/Unix: tải pinned version archive, kiểm SHA256SUMS, chọn user bin, cập nhật PATH, chạy doctor/install preview. Nếu thêm installer `.ps1`/`.sh`, default user-level, không admin, có verify-before-replace, temp cleanup scope rõ; CI dùng fixture local server để test download fail/checksum mismatch/upgrade interrupted.
7. Chọn checksum verification failure phải dừng, không bypass OS policy. Phân biệt gỡ kit (`dtkit remove`) và gỡ binary CLI; nếu hooks trỏ binary sắp gỡ phải hướng dẫn disable trước.

Nghiệm thu: người khác theo README từ máy sạch lấy được binary candidate thật và hoàn tất lifecycle. SHA binary, release SHA và reported version khớp. Không gọi workflow upload artifact là đã publish release.

### N11 — Tài liệu cuối, UX và chuẩn bị merge

Ưu tiên: bắt buộc trước publish. Phụ thuộc: N01–N10.

1. README không còn liệt kê tính năng đã implement là deferred. Tách quick-start ổn định và runtime experimental; links download dùng version tồn tại. `go install @latest` chỉ giới thiệu theo branch/tag thực tế, không mặc định lấy code mới chưa merge.
2. Cập nhật docs/cli từ command tests: flags source/offline/runtime, JSON schema, exit code, hooks, create/pack, giới hạn one kit/runtime/scope, instructions fallback, upgrade vs binary upgrade. So behavior thật với mọi ví dụ.
3. docs/release ghi current RC/gates đúng version; docs/compatibility dẫn evidence từng runtime/scope. Thêm changelog và troubleshooting cho drift, immutable version, stale lock, blocked executable, cache corrupt, auth chưa có và profile mismatch. Hướng dẫn ưu tiên preview/recover có ownership.
4. Reduce CLI parsing duplication bằng common helpers nếu cần cho lỗi thực đã thấy; giữ compatibility flag/output. Không rewrite toàn CLI chỉ để đổi framework. Test command contract cần thiết cho lỗi đã sửa.
5. Tạo PR từ chuỗi nhánh đã tích hợp hoặc từ release-hardening branch vào main; kiểm main remote mới nhất và diff review. Không cherry-pick lặp các commit vốn đã nằm trong ancestry. Merge conflict phải resolve và chạy checks cho merge result. Việc merge/publish cần nằm trong quyền owner đã cấp.

Nghiệm thu: docs và help khớp code; reviewer có thể theo release checklist mà không cần lịch sử hội thoại; CI cho merge SHA/candidate SHA có link rõ.

### N12 — Theo dõi sau phát hành và xử lý lỗi

Ưu tiên: chuẩn bị trước stable, thực hiện khi có release. Phụ thuộc: N10–N11.

1. Thêm issue templates cho bug/compatibility: version/OS/runtime/scope/command/exit code và log đã che secret; hướng dẫn không gửi state/backup/auth nguyên bản.
2. Cung cấp diagnostic output read-only (mở rộng doctor nếu phù hợp) với runtime/version/root summary/transaction status; không dump file content/credential URL. Nếu thêm `--redact`, test homedir/token được che.
3. Viết support policy: runtime version nào đã test, lịch kiểm tra lại adapter khi runtime đổi, quy trình patch/bump version. Check compatibility theo lịch chỉ là optional automation, không cần model credentials trong CI công khai.
4. Quy trình sự cố: dừng rollout asset lỗi, ghi known issue, giữ backup/journal, phát hành bản sửa có regression test. Không tự động downgrade kit/state khi chưa có migration contract.

Nghiệm thu: có cách tái hiện issue, thu thập bằng chứng an toàn và xuất bản patch minh bạch; không cần telemetry mặc định.

## 5. Nâng cấp sản phẩm sau khi các gate trên đạt

Đây là backlog mới, không phải tính năng đã có hoặc yêu cầu chặn candidate. Chỉ triển khai sau khi chốt contract cụ thể và nhu cầu.

| ID | Nâng cấp | Công việc cụ thể | Tiêu chí đạt |
|---|---|---|---|
| N13 | Nguồn kit đáng tin và reproducible team config | Trust policy cho registry, publisher signatures/provenance, lockfile pin kit/archive/runtime; kiểm integrity lẫn identity; schema/migration | Clean checkout có thể dựng cùng kit bytes; signature/key rotation/revocation có test; không gọi checksum là publisher authentication |
| N14 | Nhiều kit trong cùng runtime/scope | Hiện state giữ một Installation/kit và CLI chặn khác kit; thiết kế installation registry, kit-aware skill namespace, per-kit ownership, migrate names, overlap resolution | Hai kit có skill/plan cùng tồn tại hoặc conflict minh bạch; gỡ kit A không ảnh hưởng B; recover state cũ vẫn chạy |
| N15 | Chọn components/presets | `minimal`, `engineering`, `review` hoặc selector; dependency closure, update khi selection đổi, state lưu selection | Bỏ một skill không làm mất resource đang được component khác dùng; preview rõ add/remove và no-op |
| N16 | Hooks cho Gemini/Cursor, plugins OpenCode | Xác minh native API/trust hiện hành, capability riêng, ownership config, event dedupe, stable handler lifetime | Mỗi integration có trust/event/disable runtime smoke; không ánh xạ tùy tiện hook Codex vào runtime khác |
| N17 | Team/project experience | Export/import cấu hình có pin version, migration project roots/profile, diagnostics all runtimes, completion scripts | Onboarding team lặp lại được, không copy absolute home/state/journal giữa máy |
| N18 | Nâng chất lượng và chi phí agent | Mở rộng benchmark theo N09, profile token/latency, chia resources theo nhu cầu, tối ưu description/trigger | Báo cáo baseline vs changed kit cùng model/task/settings, phạm vi/số mẫu rõ; content update có version |

Giữ native hooks của package bên thứ ba là opt-in có trust rõ. Không đưa thực thi arbitrary scripts vào archive installer chỉ để đạt feature parity. Chưa cần marketplace/server/account system trước khi team config và multi-kit có use case cụ thể.

## 6. Dependency, chia việc cho nhiều AI và thứ tự PR

Thứ tự ưu tiên: N00 → N01/N02/N04 → N03/N05/N06 → N07/N08 → N09/N10 → N11 → N12. N13–N18 làm sau. Các bước có thể chạy song song khi input đã rõ:

- AI A: N01 và N04, sở hữu state/planner/installer; đây là nhóm file dễ xung đột nhất.
- AI B: N02–N03, sở hữu dist/source/deps và network tests; phối hợp CLI policy với AI A trước merge.
- AI C: N05–N06, adapter capability/hooks và tests; không tự sửa engine lock khi AI A đang giữ task.
- AI D: chuẩn bị runtime fixtures/docs N08–N09 và release draft N10; chạy final smoke sau core fixes.

Mỗi AI dùng branch/worktree riêng, không cùng ghi working directory; integration owner kiểm test trên combined result. Dừng mở rộng feature khi còn data-loss/upgrade/read-only/remote policy regression chưa giải quyết.

PR gợi ý:

1. `docs/current-status`: N00, chỉ trạng thái/evidence.
2. `fix/kit-version-upgrade`: N01 + fixtures binary cũ→mới.
3. `fix/source-preview-side-effects`: N02.
4. `fix/remote-policy-cache`: N03.
5. `fix/shared-target-transactions`: N04, chỉ sửa nếu test/evidence yêu cầu.
6. `fix/capability-contract` và `fix/hook-ownership`: N05–N06.
7. `test/native-crash-runtime`: N07–N09, evidence phải chỉ đúng SHA được test.
8. `release/candidate`: N10–N11, đủ archive/notes/checksum để owner review.

## 7. Ma trận kiểm chứng tối thiểu

| Nhóm test | Tình huống | Kết quả cần thấy |
|---|---|---|
| Upgrade | Main Engineer 0.1.0 → kit version mới, có personal AGENTS và hooks | Upgrade commit đúng; phần user nguyên byte; status healthy |
| Read-only | Remote dry-run/info/validate, offline cache hit, cancelled confirmation | Không đổi project/home/cache/temp; offline không network |
| Transport | HTTPS redirect downgrade, loop, timeout, invalid digest, token URL | Fail có mã/hint, không lộ secret, không ghi installation |
| Registry/cache | Duplicate index entries/keys, incompatible CLI offline, concurrent Put, corrupt blob | Reject deterministic; không nhận partial blob hoặc xóa download khác |
| Shared files | Hai runtime mutate AGENTS đồng thời, recover trong lúc runtime khác update | Không lost block/state; conflict giữ evidence khi cần |
| Capability | Cursor global instructions required vs optional; dry-run/no-op | Fail required hoặc fallback khai báo trước; output nhất quán |
| Hooks | Duplicate matching entry, binary moved, quote path, malformed stdin, disable | Không xóa user config; trust/invocation statuses chính xác |
| Native | Windows lock/junction, Unix mode/link, Unicode/path alias, subprocess kill | Fail an toàn hoặc phục hồi; skip được báo và xử lý gate |
| Runtime | Bốn runtimes × project/global/combined × discovery/task | Chỉ đánh verified nơi có bằng chứng; thiếu quyền/auth ghi pending |
| Release | Native extract/run, checksum mismatch, version metadata, clean-machine install | Dùng đúng binary release; verify fail dừng trước replace |

Lệnh baseline khi nhận task trên máy hiện tại:

```powershell
git status --short
git branch --show-current
git log -1 --oneline
.\scripts\verify-docker.ps1
```

Trên native runner có Go được phép: `gofmt -l cmd internal schemas`, `go test ./...`, `go vet ./...`, `go build ./cmd/dtkit`; `go test -race ./...` ở platform hỗ trợ. Chạy thêm tests target theo task; không gọi runtime/model task thật chỉ để verify Markdown. Không thay hệ thống bảo mật máy để chạy Go.

## 8. Release gate và quyết định cần owner

Candidate có thể chuẩn bị khi code fixes/test suite đạt; stable chỉ được công bố cho capability/runtime/scope đã có bằng chứng tương ứng.

- [ ] N01 upgrade, N02 read-only và N03 remote policy đạt regression tests.
- [ ] N04 concurrency đã được tái hiện/kiểm chứng; không còn known lost-update/data-loss.
- [ ] N05 capability contract và N06 hook ownership rõ, test pass.
- [ ] N07 native tests có evidence; tất cả required checks pass đúng candidate SHA.
- [ ] N08 runtime matrix có kết quả; unverified integrations gắn experimental, không phát hành với lời hứa stable rộng hơn evidence.
- [ ] N09 evaluation baseline có kết quả thực, content/version đã đồng bộ.
- [ ] N10 artifacts/checksums/native smoke và terminal instructions dùng được.
- [ ] N11 docs/release notes/help đúng code; PR/merge result đã kiểm tra.
- [ ] N12 support/incident procedure đủ để nhận lỗi người dùng.

Owner chỉ cần chốt các việc chưa có quyết định trong repo: candidate/stable version và kênh phát hành, runtime nào được công bố stable nếu còn missing smoke, quyền merge/tag/publish, môi trường/auth và ngân sách cho task model thực tế. Chuẩn bị candidate, PR, evidence và các lựa chọn trước khi hỏi; không hỏi lại module path/MIT hoặc dừng các task code độc lập vì thiếu Cursor/Gemini login.

Không tự coi yêu cầu viết kế hoạch này là lệnh merge/publish hoặc chạy model có chi phí. Quyền push feature branch đã có trong lịch sử; quyền tag/release cần kiểm tra ở phiên thực thi.

## 9. Mẫu bàn giao mỗi task và prompt cho AI khác

Mẫu ghi vào docs/progress:

```text
Task: Nxx — tên
Status: not-started | in-progress | verified | blocked
Baseline/branch/SHA:
Observed problem and reproduction:
Changed files and behavior:
Tests actually run: command, OS/runtime, result, CI URL
Remaining limitation / external blocker:
Acceptance checklist result:
Next task:
```

Prompt giao việc:

> Tiếp tục DTKit từ nhánh chứa đầy đủ bốn runtime. Đọc toàn bộ DTKIT-NEXT-PHASE-PLAN.md, docs/progress.md, docs/compatibility.md và contract liên quan trong DTKIT-IMPLEMENTATION-PLAN.md; xem Git/CI/code hiện tại trước khi sửa. P0–P6/H1/D1/R1–R3 đã có implementation, không xây lại. Thực hiện N00 rồi lần lượt các task bắt buộc N01–N12 theo dependencies, ưu tiên upgrade kit, remote dry-run, transport/cache policy và shared-target concurrency. Với điểm “cần tái hiện”, viết test chứng minh trước khi đổi thiết kế. Dùng worktree/branch riêng nếu có AI khác đang làm; bảo toàn user edits. Mỗi task cập nhật progress với test và evidence thực. Tách automated pass, runtime discovery và runtime task verified. Chuẩn bị release candidate có thể review, kiểm tra quyền owner trước merge/tag/publish; không mặc định triển khai backlog N13–N18 trong cùng đợt. Nếu thiếu runtime/auth, hoàn thành harness và các việc độc lập, ghi rõ blocker thay vì đánh dấu verified. Bàn giao cuối với commit, CI đúng SHA, gate còn mở và việc tiếp theo.
