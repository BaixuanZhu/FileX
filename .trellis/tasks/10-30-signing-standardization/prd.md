# 签名标准化：可复现构建 + Cosign + SLSA + SBOM

## Goal

把现有 SHA256 checksums 提升为可被第三方独立验证的供应链证据链：可复现构建 → Cosign keyless 签名（Rekor 透明日志）→ SLSA provenance → SBOM → 文档化的三步验证。

## 现状核对（2026-10-30）

| 项 | 现状 | 位置 |
|---|---|---|
| GoReleaser | **无**，手写 matrix 构建 | `.github/workflows/release.yml:56` |
| `-trimpath` | 缺失 | `release.yml:60` |
| `CGO_ENABLED=0` | 未显式设置 | `release.yml:60` |
| 构建时间戳 | `BuildTime=$(date -u ...)` → **必然不可复现** | `release.yml:60`、`Makefile:5` |
| `buildvcs` | 未设置；`fetch-depth: 0` + git 状态会被嵌入 | `release.yml:38` |
| Go 版本 | `'1.26'`（浮动小版本，破坏可复现） | `release.yml:43` |
| 归档确定性 | `Compress-Archive` / `tar -czf` → mtime、gzip 头非确定 | `release.yml:77-93` |
| checksums | 各平台 `.sha256` 合并为 `checksums.txt` | `release.yml:437` |
| 安装器 `.exe` checksum | ❌ **缺失**：合并命令不含 installer artifact，但该 exe 被发布 | `release.yml:440` vs `:509` |
| Cosign / 签名 | **无**；workflow 权限仅 `contents: write`，无 `id-token: write` | `release.yml:8` |
| SLSA provenance | **无** | — |
| SBOM | **无** | — |
| 通用 `go test` CI | ❌ **无**；仅 `test-env.yml` 限定路径 | `.github/workflows/test-env.yml:42` |
| `docs/` 目录 | **不存在**，但 release 流程会 `cp -r docs/*` → 当前归档内是空目录 | `release.yml:72` |
| 仓库身份 | 硬编码 `BaixuanZhu/FileX`，经 `git remote -v` 核实正确 | `release.yml:490,584` |

## Requirements

- R1 启用可复现构建关键项：`-trimpath`、`CGO_ENABLED=0`、`-buildvcs=false`、ldflags 用 **CommitDate** 替代构建时间、Go 版本钉到精确小版本、`GOFLAGS=-mod=readonly`。
- R2 迁移 GoReleaser，只接管 build / archive / checksums / SBOM；**NSIS 安装器 job 保留**，改为消费 `dist/`。
- R3 Cosign **keyless**（OIDC + Fulcio + Rekor）签名，产出 `.sig` + `.pem`；无长期私钥。
- R4 生成 SLSA provenance（v1.0 兼容），记录源码 commit、构建 workflow、构建环境；**provenance 生成放在独立 job**，构建 job 不持有签名凭据。
- R5 生成 SBOM（**CycloneDX JSON**），artifact 级 + source 级，随 Release 发布。
- R6 安装器 `.exe` 必须纳入 `checksums.txt`。
- R7 新增 `docs/VERIFY.md`，给出用户三步验证的可直接粘贴命令。
- R8 明确声明可复现范围，不过度承诺。
- R9 前置：新增通用 `ci.yml`（`go build` + `go vet` + `go test ./...`，linux + windows，PR 触发）。

### 可复现性声明（必须原样写入文档）

- Go 二进制 + GoReleaser 归档：**目标 bit-for-bit 可复现**，由 CI 双构建比对持续校验。
- NSIS 安装器 `.exe`：**不保证可复现**（NSIS 嵌入构建时间戳），仅由 checksum + Cosign 覆盖完整性。
- 目标等级：**SLSA Build L2～L3 之间**，不追求 L3 正式认证。

## Non-Goals

- 不追求 SLSA 最高等级，不做正式认证流程。
- 不引入长期私钥、KMS 或自建 CA。
- 不保证 NSIS 安装器可复现。
- 不签名每个独立归档（以 `checksums.txt` 作为单一信任根）。
- 不引入除 GoReleaser / cosign / syft / slsa-github-generator 之外的新发布工具链。

## Acceptance Criteria

- [ ] Release 同时附带：归档、`checksums.txt`（**含安装器**）、`checksums.txt.sig`、`checksums.txt.pem`、provenance（`*.intoto.jsonl`）、CycloneDX SBOM。
- [ ] `docs/VERIFY.md` 含三步验证命令，且在干净机器上手工走通。
- [ ] CI 存在 `reproducible-build` job：同 commit 两次构建哈希一致，PR 与 tag 均生效。
- [ ] CI 断言产物清单：缺签名 / 缺 SBOM / 缺 provenance / checksums 行数不符 → release job 失败。
- [ ] 篡改任一归档一个字节后，checksum 与 cosign 验证均失败（负例验证通过）。
- [ ] `cosign verify-blob` 使用精确 identity（`BaixuanZhu/FileX` 的 release workflow）可成功。
- [ ] 通用 `ci.yml` 对 PR 生效且 `go test ./...` 通过。
- [ ] `v*-rc*` 预发布 tag 完整跑通后，再用于正式 tag。
