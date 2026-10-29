# filex — 实施计划

> 按里程碑组织，每个 M 是一个可独立验证的交付单元。

## M0：仓库骨架（0.5 天）

- [ ] 初始化 Go module（`go mod init filex`），最小依赖（仅 `golang.org/x/sys`）
- [ ] 搭建目录结构：`cmd/filex/`、`internal/ops/`、`internal/errs/`、`internal/mcpserver/`
- [ ] 实现 `internal/errs/` — 错误码表 + JSON 信封（`schema_version`/`ok`/`data`/`error`）
- [ ] 实现 `cmd/filex/main.go` — 动词分发框架 + 退出码映射
- [ ] 实现 `filex help --json` — 返回完整命令表（动词、参数、描述）
- [ ] 实现 `filex version` — 构建信息
- [ ] 验收：`go build ./...` 通过；`filex help --json` 输出合法 JSON

## M1：编码内核（1.5 天）

- [ ] 实现 `internal/encoding/` — BOM 探测 + 编码判定链
  - [ ] BOM 检测：UTF-8 BOM / UTF-16LE BOM / UTF-16BE BOM / UTF-32
  - [ ] 启发式：严格 UTF-8 解码 → GB18030 尝试 → 二进制拒绝
  - [ ] 编码器：UTF-8 / GBK / GB18030 / UTF-16LE / UTF-16BE 双向转换
- [ ] 实现字节级 edit 核心
  - [ ] `encode(find, enc)` → 在原始字节中定位 → `encode(replace)` 替换区间
  - [ ] 保留原 BOM、CRLF、行尾空格
- [ ] 实现 `filex read` — 读取 + 编码报告（`encoding`/`bom`/`confidence`）
- [ ] 实现 `filex edit` — 字节级查找替换 + `--dry-run`
- [ ] 编码矩阵测试（testdata/conformance/）
  - [ ] 7 种编码 × 3 种换行 × 5 种内容 × 5 种文件名 × 5 种状态
  - [ ] 验收：`edit` 后除目标区间外字节完全一致（哈希逐段验证）
- [ ] 验收：`filex read` 对每种编码文件报告正确编码；`filex edit` 不破坏非目标区间

## M2：文件操作 + 备份（1.5 天）

- [ ] 实现 `internal/winfs/` — Windows 文件系统操作
  - [ ] 长路径规范化（`\\?\` 前缀）
  - [ ] 锁检测：`exclusive` + `deletable` 两个维度
  - [ ] 原子替换：`MoveFileEx` + `WRITE_THROUGH`
- [ ] 实现 `filex stat` — 文件元信息（大小、修改时间、编码、占用状态）
- [ ] 实现 `filex lock` — 占用诊断
- [ ] 实现 `filex ls` — 列目录（`--depth`、`--json`）
- [ ] 实现 `filex hash` — SHA-256 校验和
- [ ] 实现 `filex write` — 原子写入 + `--expect-sha256` + `--backup`
- [ ] 实现 `internal/backup/` — 写前快照
  - [ ] 备份到 `~/.filex/backups/<sha256-of-path>-<timestamp>.bak`
  - [ ] 保留策略：每个文件最近 3 个
  - [ ] `backup list` / `backup restore` / `backup prune`
- [ ] 实现 `filex rm` — 默认进回收站（`~/.filex/trash/`），`--purge` 永久删除
- [ ] 验收：原子写中途 kill 进程，原文件哈希不变；备份恢复后哈希一致

## M3：环境变量（1 天）

- [ ] 从 envward 移植 `core/registry/` → `internal/winenv/`
  - [ ] 注册表读写（`HKCU\Environment` / `HKLM\...\Environment`）
  - [ ] `REG_SZ` / `REG_EXPAND_SZ` 区分
  - [ ] `IsElevated()` 管理员检测
- [ ] 从 envward 移植 `core/broadcast/` → `internal/winenv/`
  - [ ] `WM_SETTINGCHANGE` 广播（`SendMessageTimeout` + 1s 超时）
- [ ] 从 envward 移植 `core/detect/ExpandEnv` → `internal/winenv/`
  - [ ] `%VAR%` 展开 + 不动点循环保护（max depth 5）
- [ ] 实现 `filex env get` — 读取单个变量 + 展开
- [ ] 实现 `filex env set` — 写注册表 + 广播 + 1024 字符警告
- [ ] 实现 `filex env list` — 列出变量 + 敏感值脱敏
- [ ] 实现 `filex env unset` — 删除变量
- [ ] **测试隔离**：所有写注册表的测试用 `//go:build integration` build tag 标记，本机 `go test ./...` 不执行；仅在 GitHub Actions CI（`TEST_ENV_CI=1`）中运行
- [ ] 编写 `.github/workflows/test-env.yml` — Windows VM 中运行 env 集成测试
- [ ] 验收：CI 绿色；本机 `go test ./...` 不触碰注册表

## M4：策略 + 审计（1 天）

- [ ] 实现 `internal/policy/`
  - [ ] 路径白名单（当前工作目录 + 显式配置）
  - [ ] 默认 deny：`C:\Windows\**`、`C:\Program Files\**`、盘符根、`.git\objects\**`
  - [ ] 软链接逃逸防护（`EvalSymlinks` 后校验）
  - [ ] `backup.before_write` 配置项
- [ ] 实现 `internal/journal/`
  - [ ] JSONL 审计日志（操作、路径、备份 ID、sha256_before、sha256_after）
  - [ ] 敏感值脱敏（env 值只存哈希）
  - [ ] 客户端信息记录（`clientInfo` from MCP `initialize`）
- [ ] 验收：越界路径返回 `E_DENIED`；journal 记录完整审计链

## M5：大文件处理（1 天）

- [ ] 实现 `filex tail`
  - [ ] 从文件末尾向前扫描，编码感知换行符
  - [ ] `--lines=N`（默认 10，最大 1000）
  - [ ] `--follow` 持续输出（context 取消可中断）
  - [ ] 输出带全局行号
- [ ] 实现 `filex read --offset --limit`
  - [ ] 行级分页，编码感知
  - [ ] 输出带全局行号前缀
- [ ] 实现 `filex grep --stream`
  - [ ] 逐行解码匹配，不加载全文
  - [ ] `--max=N` 限制返回条数
  - [ ] 二进制文件拒绝流式 grep
- [ ] `edit` 对 >100MB 文件返回 `E_TOO_LARGE` + hint
- [ ] 大文件测试
  - [ ] 100MB UTF-8 文件 `tail --lines=10` 在 1 秒内返回
  - [ ] `grep --stream` 内存峰值 <50MB
  - [ ] `read --offset=1000000 --limit=10` 返回正确内容
- [ ] 验收：所有大文件用例通过

## M6：MCP 层（1 天）

- [ ] 实现 `internal/mcpserver/` — stdlib JSON-RPC 2.0
  - [ ] `initialize` / `ping` / `tools/list` / `tools/call` / `notifications/initialized`
  - [ ] 协议版本：2024-11-05 / 2025-03-26 / 2025-06-18
- [ ] 映射 11 个 tool 到 ops
  - [ ] `read write edit grep stat lock ls rm tail backup(action) env(action)`
  - [ ] profile：`ro`（默认）/ `rw`
- [ ] 实现 `filex mcp serve` — stdio transport
- [ ] 实现 `filex mcp install --client=...` — 写入 MCP 配置
- [ ] 一致性测试
  - [ ] CLI 与 MCP 共用 conformance 表，两端结果逐字段相等
- [ ] 验收：MCP 客户端能调用所有 tool；profile 正确限制 tool 集合

## M7：打包 + 文档 + 评测（0.5-1 天）

- [ ] 编写 `docs/SKILL.md` — 给模型的使用说明（≤60 行）
- [ ] 编写 `README.md` — 安装、用法、命令速查
- [ ] 打包 `filex.exe`（`go build -ldflags` 注入版本）
- [ ] （可选）跨模型对照实验
  - [ ] 10 个典型任务 × 3 个档位模型 × 每任务 5 次
  - [ ] 指标：成功率、危险操作发生率、平均步数
- [ ] 验收：`filex.exe` 可独立运行；SKILL.md 覆盖所有命令

## 里程碑依赖

```
M0 → M1 → M2 → M3 → M4 → M5 → M6 → M7
         ↑              ↑
         |              |
    编码内核完成     策略+审计完成
    即可日常使用     MCP 层可集成
```

## 回滚点

- 每个 M 结束时 git tag，出问题可回退
- M1 编码内核是最高风险模块，失败则暂停评估方案
- M3 从 envward 移植如果接口不兼容，保留 envward 原代码做适配层

## 验证命令

```bash
# M0
go build ./... && ./filex help --json | jq .

# M1
go test ./internal/encoding/... -v
go test ./testdata/conformance/... -v

# M2
go test ./internal/winfs/... -v
go test ./internal/backup/... -v

# M3
go test ./internal/winenv/... -v

# M4
go test ./internal/policy/... -v
go test ./internal/journal/... -v

# M5
go test ./... -run TestLargeFile -v

# M6
go test ./internal/mcpserver/... -v

# M7
./filex version
```
