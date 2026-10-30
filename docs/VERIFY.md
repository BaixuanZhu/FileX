# 发布产物验证

本文说明如何自己验证一个 FileX 发布产物，以及**哪些性质能被自动证明、哪些不能**。
第二部分和仓库一起打包进发布归档，所以它必须与 `release.yml` 的实际行为一致：只写现存
的保证，不预告还没有的东西。

## 1. 校验归档摘要

每个平台的归档都带一个同名 `.sha256` 侧文件，Release 页面另有汇总的 `checksums.txt`。

Windows（PowerShell）：

```powershell
$archive = "filex-v0.2.0-windows-x64.zip"
Get-FileHash $archive -Algorithm SHA256 | ForEach-Object {
    "{0}  {1}" -f $_.Hash.ToLower(), $archive
}
# 与归档旁的 .sha256（或 checksums.txt 中对应行）逐字符比对
```

Linux / macOS：

```bash
sha256sum -c "filex-v0.2.0-linux-x64.tar.gz.sha256"
```

摘要不匹配就不要运行里面的二进制。

## 2. 复现构建

发布构建是可复现的：

```bash
CGO_ENABLED=0 go build -trimpath -buildvcs=false \
  -ldflags "-s -w \
    -X filex/internal/buildinfo.Version=<版本，去掉前缀 v> \
    -X filex/internal/buildinfo.Commit=<tag 指向 commit 的前 8 位> \
    -X filex/internal/buildinfo.BuildTime=<该 commit 的 committer date，RFC3339>" \
  -o filex ./cmd/filex
```

三个注入值不用猜：先运行发布出来的二进制，`filex version` 会报告它们（JSON 信封的
`data.version` / `data.commit` / `data.buildTime`）。`BuildTime` 取的是 **commit 时间
而不是构建时间**，这是同 commit 能产出同二进制的前提。

比对对象是**归档里的可执行文件**，不是归档本身：

```bash
# 展开归档，取出版本号与上面同样的构建参数复现后
sha256sum filex                 # Linux/macOS
Get-FileHash .\filex.exe        # Windows
```

归档是 zip/tar，包含目录项与文件系统时间戳，本身**不是**字节可复现的；平台构建还分
别跑在 ubuntu / windows / macOS runner 上，交叉产物不做逐字节承诺。

要求同一 Go 版本（当前固定 1.26.5）与标准库依赖不变。FileX 只有一个第三方依赖
（`golang.org/x/sys`，仅 Windows 代码路径使用），`go.mod`/`go.sum` 已锁定。

CI 里 `ci.yml` 的 `reproducible` job 会在 ubuntu 与 windows 上各做两次隔离构建（独立
HOME、TMPDIR、GOCACHE、GOMODCACHE）并比对摘要，所以"可复现"是每次 push 都被检查的，
不是一个人工承诺。

本轮已在 Windows 上按同样的隔离方式本地实跑过一次：两份不同路径下的源码树构建出的
`filex.exe` 摘要完全一致；去掉 `-trimpath` 重新构建后摘要改变（比对因此是有效的，
不是永远为真的空检查）。Linux 与 macOS 侧仍只有 CI 那一条证据。

## 3. 还没有的东西

截至本次改动，发布流水线**不产出** cosign 签名、SLSA provenance 与 SBOM，也没有
GoReleaser。这三项属于父任务的签名阶段 C–G，需要在真实 release 上下文里才能验收，因此
没有和本轮改动一起合入。

这意味着当前验证链能证明"这个二进制确实是这个 commit 编出来的"，但**不能**证明"这个
产物确实由官方流水线产出、未经 Tamper"——摘要只防损坏，不防伪造来源。要闭合这一段，
需要签名与 provenance 落地，届时本文会补上 `cosign verify-blob` /
`slsa-verifier` 的实际命令，而不是提前写在那里。

## 4. 可测性边界

自动测试能证明什么、不能证明什么，逐条写清楚，避免把"测试通过"读成"安全属性成立"。

| 性质 | 由什么证明 | 边界 |
|---|---|---|
| guard 决策表（四类风险 × 有无终端 × 配置开关） | `internal/guard` 单测穷举 `Evaluate`（纯函数，无 I/O） | 只覆盖判定本身，不覆盖调用方是否真的调它 |
| gate 在 ops 收敛点生效、决策链进审计 | `internal/ops` 单测（隔离 HOME，检查落盘 JSONL 字段） | 同上 |
| **管道喂 `y` 无法自我授权** | `cmd/filex` 的 `-tags=integration` 探针：真二进制 + 管道 stdin + 临时 HOME | 探针跑在开发机与 CI 上；结论依赖"agent 只能通过管道给 stdin"这个前提 |
| MCP 路径永远不可交互 | `internal/mcpserver` 单测（`tools/call` 走 `DenyConfirmer`） | 依赖宿主确实通过 stdio 而非终端启动 filex |
| 审计新增字段向后兼容 | `internal/journal/compat_test.go` 断言旧记录逐字节不变 | 只覆盖已知消费字段 |
| 旧格式日志仍可聚合 | `journal stats` 用固定 JSONL fixture（含损坏行、截断尾行、无 session 字段的旧行） | 真实日志形态多样，聚合的健壮性以实测为准 |
| 安装器安装/卸载/重定位 | `release.yml` 的 `test-installer` / `test-windows` job | 只在 windows-latest 上验证 |
| 写注册表的 env 集成测试 | `test-env.yml`，仅在 GitHub Actions 的隔离 Windows VM 执行 | **禁止在开发机运行**：这些测试改的是真实用户环境变量 |
| 签名、provenance、SBOM | 只能由真实 release 流水线验收 | 见上一节，尚未实现 |
| 真实终端前的人工确认 | **无法自动证明** | 需要人在真实终端跑一次：`filex rm x --purge` 提示、输 `y` 生效、直接回车拒绝、Ctrl+C 不删除 |
| 各 MCP 宿主是否会给出可用的会话键 | 无法由本仓库测试证明 | 只能靠 `filex journal stats` 的 `session key share` 与 `mcp hosts seen` 实测 |

## 5. 本地一键校验

```bash
make build            # 可复现构建（CGO_ENABLED=0、-trimpath、-buildvcs=false）
go vet ./...
go test ./...                              # 单元与表驱动测试，安全地在任何机器上跑
go test -tags=integration ./cmd/filex/...  # 确认通道探针，只操作临时 HOME 与临时二进制
```

`go test ./internal/winenv/... -tags=integration` **不要在开发机上跑**，它写真实的用户
环境变量；这条只在 `test-env.yml` 的隔离 VM 里执行。
