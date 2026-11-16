# DTKit — kế hoạch hoàn thiện sản phẩm và bàn giao triển khai

> Tài liệu vận hành cho AI/maintainer tiếp theo. Đọc cùng [DTKIT-IMPLEMENTATION-PLAN.md](DTKIT-IMPLEMENTATION-PLAN.md) (contract chi tiết), [ai-agent-toolkit-spec.md](ai-agent-toolkit-spec.md), [AI-Engineering-Kit-Global.md](AI-Engineering-Kit-Global.md) (ý tưởng gốc), và [docs/progress.md](docs/progress.md) (nhật ký thực thi). Nếu tài liệu và code bất đồng, kiểm tra code/test, nêu khác biệt rồi cập nhật tài liệu; không tự tuyên bố tính năng đã hoàn thành.

## 1. Đích sản phẩm và ranh giới phát hành

DTKit là CLI Go `dtkit` để phân phối bộ kỹ năng, rules, workflows, templates và về sau là hooks cho coding agents. Runtime đầu tiên là Codex. Người dùng phải cài được Engineer Kit vào **project** hoặc **global**, xem trước thay đổi, kiểm tra trạng thái, cập nhật, gỡ và khôi phục một lần ghi bị gián đoạn. Nội dung cá nhân và các skill khác phải được giữ nguyên. Binary và kit phải cài được qua terminal bằng hướng dẫn có thể lặp lại.

Các chặng phát hành:

| Chặng | Bắt buộc | Điều kiện để tuyên bố hoàn thành |
|---|---|---|
| v0.1 Codex | CLI + bundled Engineer Kit + project/global + install/status/update/remove/recover + kiểm thử trên OS hỗ trợ + kiểm chứng Codex thật + tài liệu + gói binary | Toàn bộ gate ở mục 9 đạt |
| v0.2 Hooks | Hook native có opt-in, trust review, enable/disable/status và test runtime thật | v0.1 ổn định; không chạy hook trùng giữa hai scope |
| v0.3 Distribution | Đóng gói kit, nguồn local/remote, cache theo digest, tác giả tạo kit/skill | Package không tin cậy được kiểm tra trước khi đưa vào installer |
| v0.4 Multi-runtime | Gemini CLI, OpenCode, Cursor theo từng adapter | Mỗi runtime có tài liệu chính thức, test project/global và compatibility evidence riêng |

Không gộp hooks, remote package và ba runtime mới vào điều kiện phát hành v0.1. Nhưng để gọi là **sản phẩm DTKit theo tầm nhìn đầy đủ**, phải triển khai các chặng sau v0.1; không diễn giải v0.1 là “everything”.

## 2. Snapshot chính xác tại thời điểm bàn giao

- Nhánh làm việc: `feat/dtkit-foundation`; commit quan sát: `7030acc` (`feat: report installation status and drift`). Kiểm tra lại `git status`, nhánh, remote và CI ở đầu phiên; trạng thái GitHub có thể đã thay đổi.
- Go module tạm: `dtkit.local/dtkit`, `go 1.27`; Go native Windows trên máy này bị Device Guard chặn. `scripts/verify-docker.ps1` chạy fmt/test/vet/build qua Docker. CI có ma trận Windows/Linux/macOS tại `.github/workflows/ci.yml`.
- Đã có: CLI thủ công (`version`, `list`, `info`, `validate`, `doctor`, `install`, `status`), bundled Engineer Kit, manifest validation một phần, renderer Codex, managed block parser, fresh install preview, fresh install có lock/journal/rollback cơ bản, state và read-only drift detection.
- Chưa có: `update`, `remove`, `recover`, idempotent install, prompt, status hai scope cùng lúc, exit-code contract/JSON thống nhất, runtime smoke thật, release binary, hooks, nguồn remote và adapter khác.
- `docs/progress.md` chứa vài dòng cũ nói “Next: strict manifest validation”; xác nhận lại bằng code trước khi dùng làm task list. Tài liệu này mới là bảng thứ tự công việc, còn contract chi tiết nằm trong plan cũ.
- `7030acc` đã được push theo phiên trước, nhưng kết quả CI mới **chưa được xác nhận ở snapshot này**. Kiểm tra run tương ứng trước mọi tuyên bố “CI pass”.

Điểm cần sửa ngay từ code hiện tại (không xem là đã giải quyết):

1. `internal/installer/install.go`: journal ghi `applying` sau khi artifact đã ghi, rồi state mới, nhưng `recover` chưa tồn tại. Rollback bỏ qua lỗi và vẫn đánh dấu `rollback_attempted`; sau lỗi ghi state hoặc lỗi đánh dấu `committed`, có thể còn file/state không thống nhất. Thiết kế lại trạng thái và trả lỗi tổng hợp; test crash tại từng ranh giới.
2. `atomicWrite` fallback Windows mở file đích với `O_TRUNC`. Nếu fail giữa chừng, file đích có thể dở dang; cần primitive phù hợp hoặc journal/recovery có bằng chứng, và test file locked trên Windows. Không quảng cáo atomic toàn transaction.
3. `FreshInstall` tạo `.dtkit/local` và transaction directory; kiểm tra mọi lỗi sớm/dry-run không tạo thư mục đích. `ResolveTarget` dùng `Lstat` symlink, cần chứng minh xử lý junction/reparse và đổi ancestor giữa preflight/apply. State và journal là input không tin cậy khi recover/remove.
4. Global binding: `state.CodexHome` đã lưu nhưng `Inspect` chưa so với `roots["codex-home"]`; phải báo mismatch, không âm thầm áp dụng ở profile mới.
5. `Inspect` dùng `os.ReadFile` không giới hạn trên artifact; cần giới hạn kích thước, kiểm tra file regular, xử lý lỗi đọc có phân loại. `status` text hiện trả thành công khi có drift; contract exit `3` chưa nối.
6. `previewInstall` còn comment “read-only” dù đã cài thật; CLI resolve root khác nhau giữa `install` và `status`. Tách resolver thống nhất và kiểm tra `git rev-parse` đúng cwd/path yêu cầu.

## 3. Nguyên tắc triển khai và cách ghi tiến độ

- Mỗi gói việc: đọc contract tương ứng trong plan cũ, ghi test thể hiện hành vi/nguy cơ, sửa code, chạy fmt/test/vet/build, cập nhật docs/progress, commit nhỏ. Không đánh dấu pass dựa trên `go test` của Linux khi yêu cầu Windows-specific.
- Chỉ sửa file do task sở hữu. Trước thao tác ghi/xóa: xác minh root, scope, ownership, current digest, journal và lock; conflict phải được liệt kê trước khi ghi. `--yes` chỉ bỏ prompt, không bỏ kiểm tra. Không dùng `--force` ghi đè file lạ.
- Dry-run của mọi lệnh mutation phải thật sự read-only: so snapshot cây file trước/sau, kể cả `.dtkit`, lock, cache và temp. No-op cũng không tăng generation/timestamp.
- Vùng managed trong `AGENTS.md` là sở hữu từng block; phần ngoài block là của người dùng. Whole-file chỉ quản lý path có trong state. Không recursive delete thư mục shared `.agents`, `.codex`, home hoặc repo.
- Mỗi command có JSON object nhất quán, stdout thuần JSON khi `--json`, diagnostics stderr. Exit codes theo plan cũ: `0` thành công/no-op, `2` usage/validation, `3` conflict/drift/recovery, `4` runtime thiếu, `130` hủy, `1` lỗi khác.
- Giữ project và global độc lập, namespaced skill IDs; scope thiếu ở mutation là usage error. Không tự động gọi network trong v0.1.
- Mỗi task kết thúc với: file thay đổi, test thật đã chạy và hệ điều hành, bằng chứng/giới hạn, commit, việc kế tiếp. Không cập nhật checklist “xong” nếu chỉ có unit test mà thiếu runtime smoke.

## 4. Thứ tự công việc bắt buộc: v0.1 Codex

### P0 — Chốt baseline và đóng lỗi nền

Phụ thuộc: không. Ưu tiên trước khi làm remove/update vì cùng dùng transaction engine.

1. `git status --short`, `git log -1`, kiểm tra CI cho commit hiện tại; chạy `./scripts/verify-docker.ps1`. Nếu CI Windows fail, lấy log từ UI/API được phép, sửa đúng lỗi và chạy lại. Ghi run URL/kết quả vào `docs/progress.md`.
2. Chốt một mô hình request/roots cho project và global ở `internal/cli` hoặc gói environment riêng. Tất cả lệnh dùng chung resolver, kiểm tra root không phải filesystem root/home khi project; xét `CODEX_HOME` và global binding; đường dẫn có khoảng trắng/Unicode.
3. Tạo error types (usage/conflict/runtime/recovery/IO) và mapper exit codes tại `cmd/dtkit/main.go`; gom JSON envelope/error schema, progress stderr. Chỉnh help, ví dụ, prompt TTY `[y/N]`; không prompt nếu `--yes` hoặc dry-run. Cập nhật test CLI.
4. Siết `validate`: manifest/schema và semantic, file/reference path, symlink/hardlink/device, file count/size, component graph, render cả hai scope, frontmatter/reference sau render; tách rõ validation của trusted bundled kit với package local chưa tin cậy. Thêm schema machine-readable nếu cần public format.

Nghiệm thu: không còn câu help/comment gây hiểu nhầm; tất cả lệnh dùng cùng root binding; invalid flag/code/JSON có test; dry-run không ghi file; CI baseline rõ ba OS.

### P1 — Transaction engine và recovery thực thi

Phụ thuộc: P0 resolver, error model. Đây là gate về dữ liệu; chia PR/commit nhỏ.

1. Định nghĩa journal versioned: operation, scope, root bindings, transaction ID, phase (`preparing`, `prepared`, `applying`, `committed`, `rolled_back`, `conflict`), state trước/sau, danh sách action có path tương đối/root ID, before/after digest, backup name, mode và create/remove intent. Validate strict và giới hạn byte. Không tin path tuyệt đối trong journal.
2. Lock độc quyền có token/owner metadata. `recover --dry-run` chỉ đọc và phân loại. Với stale lock, xác minh owner còn sống; nếu không đủ bằng chứng, báo conflict để người dùng can thiệp, không xóa theo tuổi lock. Chỉ holder đúng token mới release.
3. Chuẩn hóa preflight: so state generation, scope, Codex home và digest/identity từng target; plan tất cả action/conflict; sau lock đọc lại, trước từng action kiểm tra lại ancestor và bytes. Ghi/flush backup + journal `prepared` trước khi sửa target. Tạo temp cùng directory, bảo toàn mode, flush và phân biệt semantics Windows/POSIX.
4. Commit state sau artifacts và journal checkpoint rõ. Khi lỗi: conditional rollback chỉ nếu current == after-image hoặc trạng thái missing dự kiến; không đè file do editor vừa sửa. Trả lỗi rollback cùng lỗi gốc, lưu journal `conflict` khi không phục hồi sạch. Không đánh dấu hoàn tất nếu còn chênh lệch.
5. `recover --project|--global [--transaction id] --dry-run|--yes`: liệt kê transaction dở; nếu nhiều, yêu cầu ID. So current với before/after; hoàn tất commit metadata nếu state+artifacts đã đúng; rollback khi chắc chắn; báo conflict nếu hiện trạng khác cả hai. Test lặp recover là no-op. Giữ evidence chưa giải quyết; retention chỉ đụng transaction xác thực đã xong.
6. Tách fault injection theo các điểm: ghi backup, ghi prepared, artifact đầu/giữa/cuối, ghi state, ghi committed, release lock, rollback restore. Mỗi test xác nhận bytes người dùng, state và journal sau lỗi/crash mô phỏng. Test transaction đang chạy đồng thời; interruption và restart process nếu khả thi.

Nghiệm thu: có thể làm hỏng một bước bất kỳ trong fixture, gọi `recover` và đạt before hoặc after nhất quán; mọi conflict giữ nguyên bytes người dùng; status báo recovery cần thiết. Không có đường code xóa rộng theo directory.

### P2 — Planner chung, status, install idempotent

Phụ thuộc: P1 journal/state loader.

1. Chuyển `InstallPreview` từ fresh-only thành planner so sánh **state + disk + desired** theo bảng mục 12.5 của plan cũ; action `create/update/remove/noop/conflict`, reason code ổn định, tổng số khớp action. Mọi path trong plan phải qua validation/root confinement.
2. State loader strict: giới hạn size, duplicate JSON keys/unknown critical fields, version/generation/timestamps, artifact list unique, adapter revision, digest, root bindings. `status` phân loại installed/healthy/missing/modified/recovery-needed/profile-mismatch; đọc file có giới hạn, kiểm tra regular file, báo block marker lỗi riêng. Hỗ trợ scope rỗng: project + global; `--path` ngầm project theo contract.
3. `install` lần hai trên cùng version/digest/layout và healthy thành no-op tuyệt đối. Nếu state/version khác thì hướng dẫn `update`. Nếu drift, exit 3. Có prompt khi TTY, `--yes` cho non-interactive. Preview và apply dùng cùng plan, revalidate dưới lock.
4. Test project/global đồng thời, AGENTS có BOM/CRLF và user edits ngoài block, path override, skill unowned collision, state corruption, Codex home đổi, target missing/modified; so filesystem snapshots cho dry-run/no-op.

Nghiệm thu: install lặp không tạo journal mới và không đổi timestamp; `status` phát hiện mọi drift do DTKit sở hữu nhưng bỏ qua user text; JSON và exit codes đúng.

### P3 — Remove an toàn

Phụ thuộc: P1–P2. Không viết remove độc lập với transaction engine.

1. `remove engineer --project|--global [--path] --dry-run|--yes`: đọc state, không cần bundled catalog vẫn gỡ được version từng cài; ID mismatch là lỗi. Không có state là no-op.
2. Lập action cho từng artifact; whole-file chỉ xóa khi digest khớp, missing chỉ quên metadata; managed block gọi `RemoveBlock` và giữ mọi byte ngoài block. Nếu block do DTKit tạo file và sau remove file rỗng thì có thể xóa file đó. Nếu drift/unknown file trong folder, dừng trước apply hoặc giữ unknown theo contract.
3. Backup file trước khi ghi/xóa, journal + rollback/recover như P1. Sau commit xóa installation state; giữ journal/backups theo retention. Chỉ prune thư mục do DTKit tạo và đang rỗng; không xóa root chung.
4. Test project/global độc lập, AGENTS riêng, modified file, missing file, skill directory có user file, Windows locked file, failure sau một vài action, recover.

Nghiệm thu: sau remove mọi nội dung cá nhân vẫn nguyên byte; status báo chưa cài; reinstall có thể thành công trên scope sạch.

### P4 — Update/version migration

Phụ thuộc: P1–P3.

1. `update engineer --project|--global [--from dir] --dry-run|--yes`: require installed; semantic version tăng. Cùng version + cùng digest + adapter revision + healthy là no-op; cùng version/digest khác là immutable-version conflict; downgrade không hỗ trợ ở v0.1.
2. Render desired từ bundled/local package snapshot bất biến; so state/disk/desired. Ghi mới, sửa, xóa đúng owned artifacts; xử lý managed block giữ user content, mode và CRLF. Version mới có thể thay đổi artifact set; planner phải liệt kê file bỏ đi.
3. Adapter revision migration riêng; state generation tăng chính xác một lần cho commit, installedAt giữ nguyên, updatedAt đổi; global Codex binding không tự đổi. Mọi operation dùng P1 transaction/recovery.
4. Fixture kit v0.1→v0.2, thêm/bỏ skill, edit AGENTS ngoài block, drift, downgrade, same-version mismatch, failure mid-update và recover; golden output cho preview.

Nghiệm thu: trạng thái trước hoặc sau update nhất quán khi crash; không xóa user files; update lần hai no-op.

### P5 — Kiểm thử, hardening và nội dung Engineer Kit

Phụ thuộc: P0–P4; có thể làm content review song song khi P1 đang triển khai.

1. Audit path confinement trên Windows junction/reparse, case-insensitive collisions, long path, Unicode/spaces, root symlink; POSIX symlink, permission, concurrent edits. Fuzz bounded cho marker parser, manifest/path, journal/state. Race test cho code có shared state; tránh global mutable writer seam trong test chạy parallel.
2. Test security và recovery theo matrix A01–A44 trong plan cũ; ưu tiên dữ liệu người dùng, crash, stale lock và package path traversal. CI cần fmt, test, vet, build trên Windows/Linux/macOS; báo OS-specific skipped. Ghi giới hạn TOCTOU thực tế, không quảng cáo tuyệt đối.
3. Review bảy skills (`ask`, `plan`, `implement`, `debug`, `review`, `test`, `explain`), engineering rules, ba workflows và templates. Kiểm tra chất lượng hướng dẫn, scope chỉ đọc/ghi rõ, tham chiếu tương đối hoạt động ở project/global. Test render/frontmatter/link; đánh giá bằng prompt plan-only, review-only, debug-only, implementation scope.
4. Chạy executable đã build trong temp HOME/project, không chỉ test internal API. Script integration tạo fixture, install→status→update→remove→recover; không ghi home thật.

Nghiệm thu: không có known data-loss bug; matrix có kết quả và evidence; content được Codex nạp và hữu ích trong task test.

### P6 — Kiểm chứng Codex thật và phát hành v0.1

Phụ thuộc: P5; thử smoke sớm để phát hiện adapter sai.

1. Dùng bản Codex CLI thực tế trong môi trường cô lập; kiểm tra tài liệu chính thức tại thời điểm test (đường dẫn skills, AGENTS precedence, global/project, cách gọi). Ghi version, OS, ngày, auth/test setup và các lệnh tái hiện tại `docs/verification/codex-<version>-<date>.md`.
2. Cài project, global và cùng lúc; xác nhận Codex liệt kê/thực sự tải đúng namespaced skills, đọc instruction, theo workflow; kiểm tra override/precedence, restart/reload nếu cần. Có test không kích hoạt skill sai. Nếu runtime thiếu khả năng, ghi limitation và sửa adapter/content trước release.
3. Thay module path `dtkit.local/dtkit` thành `github.com/truong-newbie/Agent_Kit_Impl` (hoặc path canonical được repo owner xác nhận), thống nhất license/version metadata; build metadata version/commit/date.
4. README + docs: lấy binary qua terminal, kiểm checksum, cài CLI vào PATH, cài kit project/global, dry-run, status, update, remove, recover, troubleshooting Device Guard/CODEX_HOME, limitations và cách build từ source. Tạo release workflow tạo archives/checksums cho target OS/arch, extract chạy smoke, có provenance/artifact retention; không tuyên bố link download khi chưa có release.
5. Chuẩn bị release candidate và checklist gate mục 9; chỉ tag/upload/publish khi phạm vi phát hành đã được owner chốt. Sau phát hành, cùng version kit là immutable; content change phải tăng version.

Nghiệm thu: người dùng khác từ clean machine có thể theo README để lấy và dùng binary, và Codex nhận kit ở cả hai scope; checksum/archives kiểm tra được.

## 5. Sau v0.1: đầy đủ tầm nhìn sản phẩm

### H1 — Hooks opt-in (v0.2)

- Kiểm chứng API hook Codex theo version hiện hành trước thiết kế. Contract event, stdin/stdout, timeout, trust/approval, failure policy; tối thiểu `session-context` handler với input giới hạn. Không chạy lệnh tùy ý từ package.
- State schema v2 + migration có test; merge cấu trúc vào native config, ghi ownership/digest của entry DTKit, giữ user hooks. Binary path ổn định và Windows quoting được test.
- `hooks list/status/enable/disable`; enable có preview, explicit opt-in và trust-pending report. Global/project cùng bật không gây duplicate event. Update/remove cũng quản lý hook DTKit; real Codex smoke configured→approved→invoked→disabled.
- Gate: hook thật chạy một lần, vô hiệu hóa/gỡ bảo toàn config người dùng; fail mode được ghi rõ.

### D1 — Kit độc lập, local source và remote (v0.3)

- Thiết kế package schema/version, dependency và content digest ổn định; `create kit`, `create skill`, `validate`, `pack`. Archive deterministic có checksums; test reproducibility.
- `install <id>@<version> --from <dir>` ở local trước; chỉ mở `--source <url>` sau khi tải/verify/extract an toàn: size, file count, ratio, traversal, symlink/hardlink/device, duplicate/case names. Cache theo digest, pin lock, offline reinstall. Network explicit; không tự tải trong status/doctor.
- Chính sách nguồn, update version resolution, signature/provenance nếu hứa hẹn tính tin cậy. Test hostile archives, partial download, cache corruption, source đổi giữa preview/apply.
- Gate: tác giả khác tạo kit, pack, cài offline/remote và gỡ an toàn mà không sửa source CLI.

### R1–R3 — Gemini CLI, OpenCode, Cursor (v0.4+)

Thực hiện lần lượt từng runtime, mỗi runtime thành một milestone riêng: (1) xem tài liệu chính thức/version hiện hành; (2) lập capability matrix instructions/skills/hooks/project/global/trust/precedence; (3) thiết kế path và namespace không va Codex/adapter khác; (4) renderer + golden tests; (5) reuse planner/transaction engine, state binding và remove; (6) smoke project/global/coexistence trên runtime thật; (7) mở runtime ID trong CLI khi gate đạt. Đừng suy file layout runtime khác từ Codex. Capability thiếu phải có fallback rõ và không quảng cáo tương đương.

## 6. Lệnh kiểm chứng và thứ tự PR/commit gợi ý

Trên máy hiện tại (PowerShell, Docker Desktop):

```powershell
git status --short
git branch --show-current
.\scripts\verify-docker.ps1
```

Trên môi trường có Go được phép:

```text
gofmt -l cmd internal
go test ./...
go vet ./...
go build ./cmd/dtkit
```

Thứ tự commit/PR có thể giao cho nhiều AI nhưng không merge song song cùng file installer: `baseline/CLI` → `transaction journal` → `recover` → `planner/status/idempotence` → `remove` → `update` → `hardening/content` → `Codex smoke` → `release`. Các test content/renderer có thể làm song song với transaction khi không sửa cùng file. Sau mỗi commit kiểm tra CI cho chính SHA đó, cập nhật `docs/progress.md` theo mẫu: việc, commit, tests/OS, known limitation, bước tiếp.

## 7. Các tình huống phải có fixture test

| Fixture | Kỳ vọng quan trọng |
|---|---|
| Repo trống / global home cô lập | Install project/global và status healthy |
| `AGENTS.md` có text cá nhân, BOM, CRLF | Append/replace/remove block giữ bytes ngoài block |
| Project + global cùng tồn tại | Skill IDs riêng; thao tác scope này không đổi scope kia |
| Skill path có file chưa sở hữu | Preview conflict; không ghi file nào |
| Managed file bị sửa/xóa | Status drift; update/remove quyết định theo contract, không đè |
| Home/CODEX_HOME đổi | Báo binding mismatch; không ghi vào profile mới |
| Crash trước/sau mỗi journal/state action | Recover đạt before/after, hoặc báo conflict giữ evidence |
| File bị sửa sau action trước rollback | Không restore đè lên edit mới |
| Windows locked file/junction/case collision | Conflict/error rõ, không truncate hay escape root |
| Unicode/spaces/long path, symlink POSIX | Install/remove chỉ trong root hợp lệ |
| Manifest/state/journal corrupted/oversize | Reject trước mutation, lỗi hữu ích |
| Dry-run/no-op | Snapshot filesystem không đổi kể cả operational files |

## 8. Những quyết định cần người sở hữu xác nhận trước thời điểm phát hành

- License chính thức, canonical Go module/repository URL và tên release artifact.
- Có phát hành public hay chỉ internal; target OS/arch được cam kết hỗ trợ dựa trên test thật.
- Kênh phân phối binary (GitHub Releases/package manager) và security update policy.
- Nếu feature hooks/remote bắt buộc trong **bản đầu tiên**, phải đổi release gate; mặc định v0.1 tập trung Codex core như plan đã chốt.

Các quyết định này không chặn lập trình P0–P5. AI tiếp theo chỉ hỏi owner khi cần chốt release metadata/publication; trình bày release candidate có thể review trước khi hỏi.

## 9. Gate hoàn thành và prompt bàn giao cho AI tiếp theo

v0.1 chỉ đạt khi toàn bộ điều sau đúng: clean checkout build được; tất cả command v0.1 và exit/JSON contract hoạt động; project/global coexistence; dry-run/no-op không ghi; install/update/remove/recover chống mất dữ liệu theo fixture; CI Windows/Linux/macOS cho đúng release SHA pass hoặc limitation được đánh giá rõ; Codex thật tải kit cả hai scope; README giúp người khác cài binary qua terminal; archives/checksums và smoke extraction pass; docs phản ánh tính năng thực có. Nếu một gate chưa đạt, ghi trạng thái **release candidate chưa sẵn sàng**.

Prompt có thể đưa nguyên văn cho AI khác:

> Bạn đang tiếp tục DTKit trong repository này. Hãy đọc toàn bộ `DTKIT-PRODUCT-HANDOFF.md`, `DTKIT-IMPLEMENTATION-PLAN.md`, `docs/progress.md` và hai tài liệu ý tưởng gốc; kiểm tra Git/CI/code thật. Triển khai gói việc chưa hoàn thành sớm nhất theo thứ tự P0→P6, ưu tiên test bảo vệ dữ liệu người dùng. Không đánh dấu hoàn thành chỉ vì có skeleton hoặc unit test. Dùng Docker verification khi Go Windows bị Device Guard chặn; kiểm tra CI đúng commit trên Windows/Linux/macOS. Mỗi gói cập nhật docs/progress với thay đổi, bằng chứng test, limitation và bước kế. Giữ dữ liệu người dùng và tránh ghi vào home thật khi smoke test. Khi v0.1 đạt gate, tiếp tục H1→D1→R1–R3 theo cùng tiêu chuẩn. Báo rõ những quyết định phát hành cần owner chốt sau khi đã chuẩn bị release candidate.
