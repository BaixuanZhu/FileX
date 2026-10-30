# 执行计划：签名标准化

## 阶段 A — 前置底座（S）

- [x] A1 新增 `.github/workflows/ci.yml`：`go build` + `go vet` + `go test ./...`，matrix = ubuntu-latest + windows-latest，触发 = PR + push main。Go 版本钉 `1.26.5`。
      另加 `workflow_dispatch` 与 `concurrency` 取消旧跑；`permissions: contents: read` 为最小权限。
- [x] A2 `Makefile` 的 `BUILD_TIME` 改为 `git show -s --format=%cI HEAD`，与 CI 对齐。
- [x] A3 核对并统一仓库 URL：`release.yml:490`、`release.yml:584`、README × 2 中的 `BaixuanZhu/FileX`（已核实正确，只需确认无遗漏的其它 org 字样）。
      复查结论：workflow + README × 2 + 两个安装脚本里指向仓库的 URL 全部是 `github.com/BaixuanZhu/FileX`（7 处），另两处 `api.github.com/repos/...` 是安装脚本按 owner/name 变量拼的 API 地址，不是硬编码 org。
- [x] A4 `LICENSE` 纳入 git 跟踪并写入 MIT 全文（原内容只有一行 `test` 且未被跟踪，发布流程却引用它）。
      `release.yml` 的 `files:` 里 LICENSE 是可选存在项（`9d47db2` 曾为此放宽），补上文件即可，无需改 workflow。

**验证**：`gh pr create` 后 CI 通过；`make build` 产物与 CI 同 commit 产物哈希一致。
**回滚点**：A1 为新增文件，删除即回滚。
**状态**：A1–A4 代码/文件侧完成；**"CI 通过"这一半必须在 GitHub 上跑**，本轮边界内不做。

## 阶段 B — 可复现构建基线（S）

- [x] B1 在现有 `release.yml` 的 build step 上先加 `-trimpath`、`CGO_ENABLED=0`、`-buildvcs=false`、`BuildTime={{commit date}}`、Go 钉 `1.26.5`（**先不迁 GoReleaser**，缩小一次变更面）。
- [x] B2 新增 `reproducible-build` job（在 `ci.yml` 内）：
      - 在 `$RUNNER_TEMP/a` 与 `$RUNNER_TEMP/b` 两处 checkout 同 commit；
      - 分别设置不同 `HOME` / `TMPDIR` / `GOCACHE`；
      - 两次构建后比对二进制 SHA256，不一致则 `exit 1`；
      - linux + windows 各跑一次。
      实现细节调整：两次 checkout 用 `build-a` / `build-b` 路径（`actions/checkout` 的 `path` 入参），隔离根目录用 `mktemp -d` 而不是手写 `$RUNNER_TEMP` 子目录 —— Windows runner 上 `$RUNNER_TEMP` 是反斜杠路径，bash step 里混用容易出错；额外隔离了 `GOMODCACHE`，并关掉 `setup-go` 的缓存复用，否则共享 GOCACHE 会把"隔离是否真的生效"这件事掩盖掉。
      **本地已实跑（Windows）**：把源码树复制到两个不同路径，分别用独立 `HOME` / `TMPDIR` / `GOCACHE` / `GOMODCACHE` 构建，两个 `filex.exe` 的 SHA-256 完全一致（`0f27171672…`，7 410 176 字节）。
      **反向验证已实跑**：同一棵树去掉 `-trimpath` 重新构建，摘要变成 `b578beba69…`，比对必然变红 —— 说明这道检查不是空转。
- [ ] B3 用 `workflow_dispatch` 跑一次确认绿灯。
      **本轮不做**（需推送到 GitHub）。Windows 侧的正反两次验证已在本地做过（见 B2），Linux 侧与 `workflow_dispatch` 仍待 GitHub 上执行。

**验证**：B2 job 稳定通过 3 次；人为去掉 `-trimpath` 后该 job 必须变红（反向验证有效性）。
**审查门**：B2 变红才证明检查有效，先做反向验证再继续。
**回滚点**：revert `release.yml` 的 build step diff。

## 阶段 C — GoReleaser 迁移（M）

- [ ] C1 新增 `.goreleaser.yaml`（见 design.md §2.1），`signs:` 暂留空。
- [ ] C2 本地 `goreleaser build --snapshot --clean` 验证：产物命名 == 现有命名，归档内部结构 == 现有结构（含外层目录 + README + LICENSE + docs）。
- [ ] C3 改 `release.yml`：`goreleaser` job 替换原 matrix build/archive/checksum 三组 step。
- [ ] C4 改 `build-installer` job：从 `dist/` 取 windows 归档（替代 `download-artifact`），其余 NSIS 逻辑不动。
- [ ] C5 保留旧 workflow 为 `release-legacy.yml`（`workflow_dispatch` only）。
- [ ] C6 **`upgrade` 回归**：用 rc tag 发布后，在 Windows 上实跑 `filex upgrade` + `filex upgrade --list`，确认资产命名与归档结构兼容（`cmd/filex/upgrade_cmd.go`）。
- [ ] C7 实跑 `install.ps1` 与 `install.sh` 对 rc tag 的安装。

**依赖**：阶段 B 完成。
**审查门**：C2 的命名/结构比对必须逐项对照旧产物，不通过不得进入 C3。
**回滚点**：C5 的 legacy workflow。

## 阶段 D — 安装器哈希纳入 + Cosign 签名（S）

- [ ] D1 `build-installer` job 生成 `.exe.sha256`（格式 `<hash>  <filename>`，与 GoReleaser checksums 行一致）。
- [ ] D2 新增 `sign` job（`needs: [goreleaser, build-installer]`，`permissions: id-token: write, contents: read`）：
      - 下载 GoReleaser 的 `checksums.txt` 与安装器哈希行；
      - 追加合并，**排序稳定**；
      - `cosign sign-blob` 产出 `checksums.txt.sig` + `checksums.txt.pem`。
- [ ] D3 `create-release` job 增加产物清单断言：
      - `checksums.txt` 行数 == 预期产物数（归档数 + 安装器数）；
      - `.sig`、`.pem` 存在且非空；
      - 任一缺失 → `exit 1`。
- [ ] D4 CI 内自验一次 `cosign verify-blob`（用与文档相同的 identity regexp），失败即阻断发布。

**依赖**：阶段 C。
**关键顺序约束**：签名必须在安装器哈希合并**之后**，否则签名不覆盖安装器。
**回滚点**：移除 `sign` job + `id-token` 权限，Release 退回纯 checksums 形态。

## 阶段 E — SBOM（S）

- [ ] E1 `.goreleaser.yaml` 启用 `sboms:`（CycloneDX JSON，artifact 级）。
- [ ] E2 增加 source 级 SBOM（`artifacts: source`）。
- [ ] E3 SBOM 文件纳入 `checksums.txt`（GoReleaser 默认会计入，需确认）并纳入 D3 清单断言。

**依赖**：阶段 C。可与阶段 D 并行。

## 阶段 F — SLSA Provenance（M）

- [ ] F1 `goreleaser` job 输出 `hashes`（base64 的 subject 摘要列表）。
- [ ] F2 新增 `provenance` job，调用 `slsa-framework/slsa-github-generator` 的 generic generator reusable workflow（钉到具体 tag，不用 `@main`）。
- [ ] F3 确认 provenance 的 subject 覆盖**归档 + 安装器 + SBOM**。
- [ ] F4 `permissions` 最小化核对：`build-installer` 不得有 `id-token: write`。
- [ ] F5 `slsa-verifier verify-artifact` 实测通过。

**依赖**：阶段 C、D（需要最终产物清单）。
**回滚点**：移除 `provenance` job，其余证据链不受影响。

## 阶段 G — 文档（S）

- [x] G1 新建 `docs/VERIFY.md`：三步验证命令（见 design.md §4）+ **可复现范围声明**（二进制/归档可复现；NSIS exe 不保证）+ 精确 Go 版本与 flags。
      已写：按系统的校验和核对、精确的复现构建命令（Go `1.26.5` + 三个 `-X` 注入值 + `-trimpath -buildvcs=false` + `CGO_ENABLED=0`）、"归档不是字节级可复现"与"跨平台产物不做字节承诺"两条范围声明、可测性边界表（每条性质对应到钉住它的测试及该测试的证明上限）。
      **未写死**：cosign / provenance 的三步命令暂列为"还没有的东西"——签名链路（阶段 D–F）未在 GitHub 跑通前，文档不能承诺一个还不存在的签名身份。
- [ ] G2 `release.yml` 的 Release body 增加 Verification 段，指向 VERIFY.md 并内嵌 cosign 命令。
      **待阶段 D**：内嵌的 identity 必须等 `cosign sign-blob` 真实产出后才知道写什么。
- [x] G3 README / README.en 增加"供应链验证"小节。
      中英各一节，指向 `docs/VERIFY.md`，并写明当前只有校验和、没有签名/provenance/SBOM —— 校验和能发现损坏与不一致，不能证明来源。
- [ ] G4 CHANGELOG 单列 **Security Fix**：安装器 `.exe` 此前未被 checksums 覆盖。
      `CHANGELOG.md` 本轮已新建，这一条等 D1 落地时补进同一个 Unreleased 段。
- [ ] G5 干净机器手工三步验证，含**负例**（篡改一字节后必须失败）。
      **待阶段 D–F**：负例需要一个真实存在的签名产物。

**依赖**：阶段 D、E、F。

## 验证命令汇总

```bash
go vet ./... && go test ./...
goreleaser check
goreleaser build --snapshot --clean
goreleaser release --snapshot --clean   # 本地全链路 dry run（不含 cosign OIDC）
```

## 发布节奏

1. 阶段 A、B 合并到 main（无发布影响）。
2. 阶段 C–F 完成后打 `v0.x.y-rc1`，走完整流水线并做 C6/C7/G5 验证。
3. rc 全绿后打正式 tag。
