# 技术设计：签名标准化

## 1. 边界与职责划分

```
┌─ ci.yml (新增, PR+push) ──────────────────────────────┐
│  build / vet / test ./...  (linux + windows)          │
│  reproducible-build: 双构建比对 SHA256                 │
└───────────────────────────────────────────────────────┘

┌─ release.yml (tag v*) ────────────────────────────────┐
│ job: goreleaser                                       │
│   permissions: contents:write, id-token:write         │
│   GoReleaser → dist/ : 二进制 + 归档 + checksums       │
│                        + SBOM + cosign .sig/.pem      │
│   outputs: hashes (base64 of subject digests)         │
│                                                       │
│ job: build-installer  (needs: goreleaser)             │
│   消费 dist/ 的 windows 归档 → NSIS → .exe             │
│   生成 .exe.sha256 并追加进 checksums.txt              │
│   permissions: contents:read  ← 不持有签名凭据          │
│                                                       │
│ job: test-installer   (needs: build-installer)  [保留] │
│                                                       │
│ job: provenance  (needs: goreleaser, build-installer)  │
│   uses: slsa-github-generator (reusable workflow)      │
│   permissions: id-token:write, contents:write,         │
│                actions:read                            │
│   仅接收 subject 摘要 → 产出 *.intoto.jsonl            │
│                                                       │
│ job: create-release (needs: 全部)                      │
│   断言产物清单完整 → 发布                               │
└───────────────────────────────────────────────────────┘
```

**隔离要点**：构建 job 与 provenance job 分离，provenance 由 GitHub 托管的 reusable workflow 签发，构建 job 无法伪造；安装器 job 不授予 `id-token: write`。

## 2. 可复现构建契约

### 2.1 构建参数（`.goreleaser.yaml`）

```yaml
version: 2

before:
  hooks:
    - go mod download

builds:
  - id: filex
    main: ./cmd/filex
    binary: filex
    env:
      - CGO_ENABLED=0
      - GOFLAGS=-mod=readonly
    flags:
      - -trimpath
      - -buildvcs=false
    ldflags:
      - -s -w
      - -X filex/internal/buildinfo.Version={{ .Version }}
      - -X filex/internal/buildinfo.Commit={{ .ShortCommit }}
      - -X filex/internal/buildinfo.BuildTime={{ .CommitDate }}
    mod_timestamp: "{{ .CommitTimestamp }}"
    goos: [windows, linux, darwin]
    goarch: [amd64]

archives:
  - id: default
    name_template: "filex-v{{ .Version }}-{{ .Os }}-x64"
    format_overrides:
      - goos: windows
        formats: [zip]
    files:
      - README.md
      - LICENSE
      - src: docs/**/*
        dst: docs

checksum:
  name_template: checksums.txt
  algorithm: sha256

sboms:
  - id: archive
    artifacts: archive
    documents: ["{{ .ArtifactName }}.cyclonedx.json"]
    args: ["$artifact", "--output", "cyclonedx-json=$document"]

signs:
  - id: cosign-checksums
    cmd: cosign
    artifacts: checksum
    certificate: "${artifact}.pem"
    output: true
    args:
      - sign-blob
      - --yes
      - --oidc-provider=github-actions
      - --output-signature=${signature}
      - --output-certificate=${certificate}
      - ${artifact}
```

### 2.2 关键决策

| 决策 | 理由 |
|---|---|
| `BuildTime` 用 `.CommitDate` | 构建时间是唯一无法复现的输入；提交时间对同一 commit 恒定 |
| `-buildvcs=false` | `go build` 默认嵌入 VCS 状态，dirty/clean 与路径差异会改变二进制；commit 已由 ldflags 注入 |
| `mod_timestamp = CommitTimestamp` | 消除归档内 mtime 与二进制 mod time 差异 |
| Go 版本钉 `1.26.5` | `'1.26'` 会随 setup-go 浮动到新小版本，编译器变更即破坏复现 |
| **只签 `checksums.txt`** | 单一信任根；安装器哈希已追加进该文件，签名即覆盖全部产物。避免 N 个 `.sig` 的管理成本 |
| SBOM 格式选 CycloneDX | 单一格式避免双份产物；syft 原生支持 |

### 2.3 双构建比对（`reproducible-build` job）

同一 commit，在**不同工作目录 + 不同 `HOME`/`TMPDIR`/`GOCACHE`** 下各构建一次，比对二进制 SHA256。
- `-trimpath` 消除路径差异 → 两次必须一致；若不一致说明仍有非确定输入。
- 只比对**二进制**，不比对 NSIS exe。
- linux 与 windows 两个 runner 各自内部双构建（不跨 OS 比对）。

## 3. 安装器哈希纳入链路

当前缺口：`cat artifacts/*/*.sha256 > checksums.txt`（`release.yml:440`）不含 installer。

新流程：
1. `build-installer` job 生成 `.exe` 后计算 SHA256，输出 `filex-v<ver>-windows-x64.exe.sha256`（格式与 GoReleaser checksums 行一致：`<hash>  <filename>`）。
2. 该行**追加**到 GoReleaser 产出的 `checksums.txt`。
3. **顺序约束**：追加必须发生在 cosign 签名**之前**，否则签名不覆盖安装器哈希。
   → 因此 cosign 签名步骤从 GoReleaser 内置 `signs:` 移到 `build-installer` 之后的独立步骤，或 installer 改为在 goreleaser job 内构建。
   **方案选择**：保持 GoReleaser 不签名（`signs` 置空），在 `sign` job（needs: goreleaser + build-installer）中统一：合并 checksums → cosign sign-blob → 输出 `.sig`/`.pem`。这样签名时机唯一且覆盖完整。

修正后的 job 链：
```
goreleaser → build-installer → sign(合并checksums+cosign) → provenance → create-release
                            ↘ test-installer ↗
```

## 4. 验证契约（`docs/VERIFY.md`）

```bash
# 步骤 1：验签 + Rekor 透明日志
cosign verify-blob \
  --certificate checksums.txt.pem \
  --signature checksums.txt.sig \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp '^https://github.com/BaixuanZhu/FileX/\.github/workflows/release\.yml@refs/tags/v' \
  checksums.txt
# 再校验自己下载的产物
sha256sum -c checksums.txt --ignore-missing

# 步骤 2：核对 provenance 的源码引用
slsa-verifier verify-artifact <归档> \
  --provenance-path multiple.intoto.jsonl \
  --source-uri github.com/BaixuanZhu/FileX \
  --source-tag <tag>

# 步骤 3（可选）：自行可复现构建比对
git clone --depth 1 --branch <tag> https://github.com/BaixuanZhu/FileX
# 用 docs/VERIFY.md 给出的精确 go 版本与 flags 构建，比对二进制 SHA256
```

**identity 精确性**：`--certificate-identity-regexp` 必须锚定 `BaixuanZhu/FileX` 与 `release.yml`，否则任意仓库的 keyless 签名都能通过验证 —— 这是 keyless 最常见的误用。

## 5. 兼容性与影响面

| 影响 | 说明 |
|---|---|
| 归档文件名 | GoReleaser 命名模板需与现有 `filex-v<ver>-<os>-x64` **保持一致**，否则 `install.ps1` / `install.sh` / `upgrade` 命令下载失败 |
| 归档内部结构 | 现有归档含一层外层目录（`release.yml:65`），`install.ps1` 与 NSIS 依赖该结构 → GoReleaser 的 `wrap_in_directory` 需对齐 |
| `docs/` 新建 | 会被打进归档（原先是空目录），归档体积与内容变化；`upgrade` 流程需确认不受影响 |
| `Makefile` | 本地构建的 `BUILD_TIME` 也应改为 commit date，否则本地与 CI 产物不可比对 |

> **`upgrade` 命令是回归高风险点**：`cmd/filex/upgrade_cmd.go`（769 行）依赖 Release 资产命名与归档结构，任何命名变更必须同步验证。

## 6. 回滚形态

- 保留旧 `release.yml` 为 `release-legacy.yml`（`workflow_dispatch` 触发），回滚 = revert 单个 workflow 文件。
- 先用 `v0.x.y-rc1` 预发布 tag 全链路验证，再用于正式 tag。
- cosign identity 写错时：修正后补发 `.sig`/`.pem` 资产即可，**不需要重打 tag**。
