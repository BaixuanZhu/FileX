# envward 可复用模块分析

> envward 是已有的 Windows 环境变量管理工具（Go），filex 应从中吸收成熟模块。

## 可直接移植的模块

| envward 模块 | filex 目标位置 | 说明 |
|---|---|---|
| `core/registry/` (path.go, read.go, regfile.go, scope.go, values.go, write.go) | `internal/winenv/` | 注册表读写层，区分 REG_SZ/REG_EXPAND_SZ，已有完整测试 |
| `core/broadcast/` (broadcast.go) | `internal/winenv/` | WM_SETTINGCHANGE 广播 + 1s 超时策略 |
| `core/detect/expand.go` | `internal/winenv/` | %VAR% 展开 + 不动点循环保护（max depth 5） |
| `core/detect/scan.go` | `internal/winenv/` | PATH 条目扫描 + 存在性检查 |
| `core/detect/command.go` | `internal/winenv/` | 命令解析（模拟 Windows PATH 查找顺序） |
| `core/backup/` (export.go, list.go) | `internal/winenv/backup/` | .reg 格式备份（与 regedit 兼容） |
| `internal/ops/commit.go` | 设计模式参考 | "re-check → forced backup → write+verify → broadcast" 四步管道 |
| `internal/mcp/` (server.go, tools.go) | 设计模式参考 | stdlib JSON-RPC 2.0 MCP 服务器骨架 |
| `internal/upgrade/` | `internal/upgrade/`（可选） | 自更新机制 |

## envward 的 PATH 专项操作（filex 应保留）

envward 的 PATH 操作不只是"env set PATH xxx"，有独立的子命令：

- `remove_path_entries` — 字节级精确匹配移除
- `append_path_entries` — 归一化判重追加
- `reorder` — 条目重排序
- `resolve_command` — 模拟 Windows 命令查找顺序

**建议**：filex 保留这些作为 `filex path` 子命令组，不简化为普通的 `env set`。

## 依赖分析

envward 仅依赖 `golang.org/x/sys v0.49.0`（Windows registry + Win32 API）。filex 可保持同样干净的依赖树。

## 测试策略参考

envward 的测试模式：
- `internal/testreg/` — 测试用注册表隔离层
- `core/detect/fixture_test.go` — 固定测试数据
- `dirExists` 包级变量 — 测试注入点（mock 文件系统）

filex 可沿用这些模式。

## 关键设计差异

| 方面 | envward | filex |
|---|---|---|
| 范围 | 仅环境变量（主要是 PATH） | 文件操作 + 环境变量 + 备份 |
| 编码处理 | 无（注册表值固定 UTF-16LE） | 核心功能（BOM 探测、GBK/GB18030、字节级 edit） |
| 安全模型 | 写前确认（dry_run + confirmed） | profile（ro/rw）+ 策略白名单 + 回收站 |
| 审计 | 无 | JSONL journal + 写前备份 |
| 错误格式 | 中文文本 | 结构化 JSON（错误码 + hint + retryable） |
