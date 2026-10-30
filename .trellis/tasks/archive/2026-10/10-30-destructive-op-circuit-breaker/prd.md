# 破坏性操作熔断（瘦版）

> v2 范围：3 个 P0 修复 + tty/deny 双通道 + 审计字段扩展。
> 审批队列、分类规则引擎、哈希链已移入父任务 [backlog.md](../10-30-supply-chain-exec-safety/backlog.md)。

## Goal

修掉三个"宣称安全但实际不安全"的缺陷，并在**权限通过之后、执行之前**为不可逆操作提供**独立于模型**的确认或拒绝通道。

## 现状核对（2026-10-30）

### 可复用的基础设施

- **唯一收敛点**：所有文件操作都在 `internal/ops` 调用 `checkPolicy`（10 处，`internal/ops/helpers.go:14`）；CLI（`cmd/filex`）与 MCP（`internal/mcpserver`）都经过它。
- 路径策略：`internal/policy/policy.go:10`（`allowed_dirs` / `deny_patterns`，deny 优先，`EvalSymlinks` 防逃逸），配置读取 `policy.Load()`（`policy.go:28`）。
- 路径匹配器：`policy.matchPattern`（`policy.go:136`）已支持 `**`，**复用，不新写**。
- MCP profile：ro/rw 两档（`internal/mcpserver/profile.go:10`），在 `server.go:212` 与 `tools.go:174` 两处生效。
- 审计：append-only JSONL + 全局 mutex（`internal/journal/journal.go:27`）。
- 现有依赖 `golang.org/x/sys`，可直接用于 Windows 控制台句柄操作，**无需新增依赖**。

### 缺口与缺陷

| # | 问题 | 位置 | 严重性 |
|---|---|---|---|
| 1 | **`rm --dry-run` 实际会删除**：CLI 解析到该旗标后 `continue` 丢弃，但 help 对外宣传 | `cmd/filex/file_ops.go:155` vs `cmd/filex/help.go:121` | **P0** |
| 2 | **`--yes` 是空实现**：`RmParams.Yes` 从未被 `Rm()` 读取 → **当前不存在任何确认通道** | `internal/ops/file_ops.go:312`（定义）/ `:322-376`（未使用） | **P0** |
| 3 | **MCP 让模型自填 `yes` 自我授权** | `internal/mcpserver/tools.go:383-384` | **P0** |
| 4 | `--purge` 直接 `os.RemoveAll`，无任何前置拦截 | `internal/ops/file_ops.go:339` | 高 |
| 5 | 审计缺 `actor` / `session_id` / `decision` / `confirm_source` / `bypass` / `result` | `internal/journal/journal.go:14` | 高 |
| 6 | 审计覆盖不全：backup restore/prune、upgrade、版本切换无记录 | `recordJournal` 仅 4 处 | 中 |
| 7 | `rm` 实际移到 `~/.filex/trash`，文档与 MCP tool 描述称 "Windows 回收站" | `internal/ops/file_ops.go:348` / `tools.go:119` | 中（文档） |

## Requirements

- R1 **拦截层位置**：`internal/ops`，顺序固定为
  `checkPolicy → guard.Evaluate → Confirmer → 执行 → journal`。
  不得放在 `cmd/filex` 或 `internal/mcpserver`（会被另一条入口绕过）。
- R2 决策为结构化三值 `allow` / `require_confirmation` / `deny`，附 `reason` 与 `safer_alternative`。
- R3 **确认通道必须独立于模型**：
  - CLI：**仅接受真实终端**（Windows `CONIN$` / 其它 `/dev/tty`）；打不开即视为非交互，**不回退到 stdin**。
  - MCP：stdio 已被 JSON-RPC 占用 → **永不提示**，非交互恒 deny。
  - 模型不得通过 tool 参数自我授权 → 从 MCP schema 移除 `yes`。
- R4 高危分类**硬编码**（v2 不做规则引擎），默认集合：
  1. `rm --purge`（不可逆删除）
  2. `env set/unset` 的 `machine` 作用域（影响全机器）
  3. 写/编辑 `~/.filex/**`（防自我提权：改配置即可放宽策略）
- R5 **bypass 必须存在且必须被审计**：显式冗长旗标 + 配置开关 `allow_bypass`；记录含来源。缺审计记录即视为缺陷。
- R6 审计记录决策、操作摘要、确认来源、结果。
- R7 **不重建权限系统**：guard 是"权限通过之后的执行前熔断"，ro/rw profile 与路径策略仍是第一道门。
- R8 逃生口：`guard.enabled=false` 可一键全关，**无需回滚代码**。

## Non-Goals

- 不建审批队列（→ backlog B1）。
- 不建分类规则引擎 / `guard.classes` 配置数组（→ backlog B3）。
- 不做审计哈希链（→ backlog B2，且需先解决锚点问题）。
- 不做 `warn` 灰度档：逃生口已有两个（`enabled=false` + bypass 旗标），再加一档只增加配置面。
- 不把所有写操作变成人工确认；只覆盖 R4 的三类。
- 不替代 / 不扩展 MCP profile；不引入 API Token。
- 不改 `rm` 为真 Windows 回收站（已决策：改文档）。
- 不实现 / 不预留能力令牌接口。

## Acceptance Criteria

- [ ] `rm --dry-run` 不再删除任何内容，输出结构化预览（目标路径、是否 purge、预计去向）。
- [ ] `RmParams.Yes` 与 MCP schema 的 `yes` 已移除；模型传入 `yes` 无授权效果且不报错。
- [ ] R4 三类操作在**非交互**场景被 deny，响应含 `reason` + `safer_alternative`（结构化字段，非散文）。
- [ ] R4 三类操作在**真实终端**下可经确认完成。
- [ ] **管道 stdin 喂 `y` 不能通过确认** —— 本子任务核心安全断言，必须有自动化测试。
- [ ] bypass 旗标可用，且每次 bypass 在审计中留下 `bypass=true` + 来源；**缺审计即验收失败**。
- [ ] `guard.enabled=false` 时行为完全回退到 v2 前（除 P0 修复外）。
- [ ] 审计新增字段全部 `omitempty`，老日志消费方不受影响。
- [ ] 审计覆盖补全：backup restore/prune、upgrade、版本切换。
- [ ] 测试覆盖：分类 / 决策 / 确认 / 拒绝 / bypass 审计 五类。
- [ ] 非交互路径无回归：`install.ps1`、`install.sh`、`.github/workflows/test-env.yml`、`filex upgrade`、MCP ro profile。
- [ ] 文档与行为一致：README × 2、SKILL.md、`help.go`、MCP tool description（含回收站语义更正）。
- [ ] CHANGELOG 含 **Security Fix**（dry-run / `--yes` / MCP 自授权）与 **Breaking**（非交互 purge 默认拒绝）。

## 可测性边界

| CI 可覆盖 | 只能手工 |
|---|---|
| `guard.Evaluate` 表驱动（纯函数，~100%） | 真实终端输 `y` 能通过（可用性，非安全性质） |
| `rm --dry-run` 不改文件系统 | 真实 Agent 会话中 deny 的实际表现 |
| **管道喂 `y` 被拒绝**（安全相关的那一半） | |
| MCP `yes:true` 无效 + deny 响应结构 | |
| bypass 必留审计 | |

> 这张表要落到 README，不留口头承诺。
