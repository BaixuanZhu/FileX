# filex — 技术设计

> 完整设计方案见 `E:\Desktop\filex-plan.md`。本文档摘取实现关键决策。

## 设计原则

1. **机制在代码，流程在技能，判断在模型。**
2. **CLI 优先，MCP 是薄壳。** CLI 与 MCP 共用同一套 `internal/ops`，MCP 层只做参数映射，不 exec CLI、不解析 stdout。
3. **安全默认值。** MCP 默认只读；删除默认进回收站；越界路径默认拒绝；编辑默认保留原编码。
4. **结构化、可恢复的错误。** 每个错误带稳定错误码、`hint`（下一条该执行的命令）、`retryable`。
5. **靠发现，不靠记忆。** `filex help --json` 返回完整命令表。

## 架构

```
filex/
├── cmd/filex/
│   ├── main.go            # 动词分发、参数解析、退出码
│   └── mcp.go             # `filex mcp serve|install`
├── internal/
│   ├── ops/               # 唯一实现：Read Write Edit Grep Stat Lock Ls Hash Tail EnvRm Backup
│   ├── encoding/          # BOM 探测、GBK/GB18030、UTF-16、字节级拼接
│   ├── policy/            # root 白名单、deny 规则、dry-run 要求
│   ├── journal/           # JSONL 审计日志、脱敏
│   ├── errs/              # 错误码 + hint + retryable
│   ├── mcpserver/         # tool schema ↔ ops 映射
│   ├── winfs/             # 长路径、锁检测、原子替换、WM_SETTINGCHANGE
│   └── backup/            # 写前快照、保留策略、恢复、prune
├── testdata/conformance/  # CLI 与 MCP 共用的一致性用例表
└── docs/SKILL.md          # 给模型的使用说明（≤60 行）
```

**关键约束**：`mcpserver` 不允许 import `cmd/filex`，只能 import `ops`。

## 关键设计决策

### 编码

**读取判定顺序**：显式 `--encoding` → BOM → policy 默认 → 启发式（UTF-8 → GB18030 → 拒绝）。

**写入规则**：新建 UTF-8 无 BOM + LF；覆盖保持原编码/BOM/换行；不确定则拒绝。

**edit 实现**：字节级拼接，不解码全文。`encode(find, enc)` → 定位 → `encode(replace)` 替换区间。除被替换区间外所有字节完全不变。

### 原子写

1. 绝对路径 → policy 校验
2. `--expect-sha256` 比对
3. 同目录 `.filex-tmp-<random>`
4. 写入 → Flush → Sync
5. `MoveFileEx(temp, target, REPLACE_EXISTING | WRITE_THROUGH)`
6. 记录 journal

### 锁检测

两个独立维度：
- `exclusive`：`CreateFileW` 以 `GENERIC_READ|WRITE` + 不共享打开
- `deletable`：`CreateFileW` 以 `DELETE` 权限 + `FILE_SHARE_READ|WRITE|DELETE` 打开

### 备份策略

写前快照，不是事后审计。`write`/`edit` 覆盖已有文件时，policy 配置 `backup.before_write=true`（默认开启），自动备份到 `~/.filex/backups/<sha256-of-path>-<timestamp>.bak`。默认保留每个文件最近 3 个备份。

### 大文件处理

- >10MB 自动切换流式模式
- `tail` 从文件末尾向前扫描，编码感知换行符
- `grep --stream` 逐行解码匹配
- `edit` 对 >100MB 文件返回 `E_TOO_LARGE`
- `read --offset --limit` 行级分页

### 环境变量

- 三作用域：`process`（`os.Setenv`）、`user`（`HKCU\Environment`）、`machine`（HKLM，需管理员）
- 值含 `%VAR%` 必须写 `REG_EXPAND_SZ`
- 写入后广播 `WM_SETTINGCHANGE`
- 敏感变量默认脱敏

### 错误码与退出码

| 码 | 错误码 | 含义 |
|---|---|---|
| 0 | — | 成功 |
| 1 | `E_INTERNAL` | 内部错误 |
| 2 | `E_USAGE` | 参数错误 |
| 3 | `E_DENIED` | 策略拒绝 |
| 4 | `E_LOCKED` | 文件被占用 |
| 5 | `E_NOT_FOUND` | 文件不存在 |
| 6 | `E_CONFLICT` | 并发修改 |
| 7 | `E_ENCODING` | 编码错误 |
| 8 | `E_IO` | IO 错误 |
| 9 | `E_TOO_LARGE` | 文件过大 |
| 10 | `E_UNSUPPORTED` | 不支持 |

### MCP 层

- stdio transport，纯 stdlib JSON-RPC 2.0（不依赖第三方 MCP SDK）
- 11 个 tool：`read write edit grep stat lock ls rm tail backup(action) env(action)`
- profile：`ro`（默认只读）/ `rw`（显式开启）

## 与 envward 的关系

envward 的以下模块可直接复用：
- `core/registry/` → `internal/winenv/`（注册表读写）
- `core/broadcast/` → `internal/winenv/`（WM_SETTINGCHANGE）
- `core/detect/ExpandEnv` → `internal/winenv/`（%VAR% 展开）
- `internal/ops/commit.go` 的 safety pipeline 模式
- `internal/mcp/` 的 stdlib JSON-RPC 骨架

## 风险

| 风险 | 应对 |
|---|---|
| 编码启发式误判 | 报告 `confidence`；写操作不确定就拒绝 |
| 锁检测语义不完整 | 只回答"能否独占/能否删除"，不猜测 |
| MCP 上下文成本 | profile 机制；默认只读 6-7 个 |
| 跨卷 rename | 同目录临时文件；跨卷报 `E_UNSUPPORTED` |
| 备份磁盘膨胀 | 默认保留 3 个 + `backup prune` |
| 大文件 tail 行号漂移 | `--follow` 模式标注"近似行号" |
| **环境变量测试污染本机** | **写注册表操作不可逆；所有 env 测试必须在 GitHub Actions 隔离 VM 中运行，本机 `go test` 跳过 env 写操作测试（build tag 或 `TEST_ENV_CI` 环境变量控制）** |
