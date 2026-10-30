# 会话消耗观测：先用数据回答"问题是否存在"

> v2 策略变更（2026-10-30）：本子任务原为"执行预算与自动降级"（配额存储 + 四档降级阶梯 + TTL 恢复）。
> 现收缩为**纯观测**。拦截与降级移入父任务 [backlog.md](../10-30-supply-chain-exec-safety/backlog.md) B4，
> 并绑定一个明确的决策门 —— 不是放弃，是等证据。

## 为什么从"拦截"退到"观测"

原设计的计数键是**会话身份**，而 filex 无法可靠地知道"会话"是什么：

- MCP 连接有天然会话边界，但**真实宿主是否提供稳定标识未经验证**。
- CLI 每次调用都是独立进程，没有任何会话概念。
- v1 的退路是 `ppid + 启动时间`派生 —— 这是猜测：对每次新起进程的 Agent 会**失效**（每次算新会话，配额永不累积），对同一终端下手工操作的用户会**误伤**（连续几次正常删除被算成一个异常会话）。

键不可靠 ⇒ 配额判定的基础不成立。拿全计划最复杂的一块（跨进程锁 + 原子计数 + 降级状态机 + TTL 恢复）去换一个基础不牢的防护，不划算。

更根本的问题：**"Agent 在一轮会话中做出单个合法、合起来异常的操作"目前是一个假设，没有数据。** 本子任务的任务就是把它变成可检验的命题。

## Goal

1. 给审计日志补上**会话维度**，使"谁在一段时间内做了多少高危操作"可被事后查询。
2. 提供一条**只读**的聚合查询命令，输出按 session / verb / 分类的消耗分布。
3. 据此设立**决策门**：观测期结束后用数据判断是否值得实现 backlog B4（预算拦截）。

**零新增存储、零跨进程锁、零拦截行为** —— 既有 JSONL 本身就是计数器。

## 现状核对（2026-10-30）

### 前提校正

需求描述中的"持久化操作队列"**在本仓库不存在**（全仓 grep `queue` 无匹配）。
→ 既没有队列，也就没有"入队计数点"。v2 直接放弃建这类设施。

### 可复用的部分

| 可复用 | 位置 | 本任务如何用 |
|---|---|---|
| 审计 JSONL（append-only） | `internal/journal/journal.go:41` | **就是计数器本体**，读时聚合 |
| `Entry` 的 session 字段 | 熔断子任务阶段 B1 加好（只加字段不填值） | 本任务负责填值 |
| MCP clientInfo | `internal/mcpserver/server.go:18` | 已存在但未下传，本任务下传 |
| `internal/ops` 收敛点 | `internal/ops/helpers.go:14` | session 透传的落点；**不在此做任何判定** |
| `~/.filex/` 目录约定 | `policy.go:33`、`journal.go:34` | 不新增任何目录 |

### 缺口

- `journal.Entry` 无 `session_id` / `session_source`（由熔断子任务加字段）。
- MCP 侧有 clientInfo 但未下传到 `ops`。
- 无任何查询/聚合命令，日志只能人工 `grep`。

### 刻意不填的缺口

- **无跨进程原子计数设施**（`journal.go:27` 的 mutex 仅单进程有效）。
  → v2 不需要它：只读聚合不要求原子性，append-only 日志的少量交错不影响分布统计。

## Requirements

- R1 **session 身份来源明确且不猜测**：

  | 入口 | `session_id` 来源 | `session_source` |
  |---|---|---|
  | MCP | `initialize` 时生成，经 context 下传 | `mcp_connection` |
  | CLI | 读 `FILEX_SESSION_ID` 环境变量（由 Agent/编排器设置） | `env` |
  | CLI 未设置 | **不派生、不猜测**，字段留空 | `absent` |

- R2 `session_source=absent` 是**可接受的正常状态**，不触发任何行为变化。记录"我不知道"比记录一个猜出来的 ID 更有价值 —— 后者会污染统计。
- R3 新增只读命令 `filex journal stats`，支持按 session / verb / guard_class / 时间窗聚合，支持 `--json`。
- R4 **本任务不引入任何拦截、降级、配额、TTL、封禁行为。** 纯读 + 纯记录。
- R5 `session_id` 仅作为**观测标签**，不参与任何授权或决策判定 —— 伪造它不产生安全后果（这正是不需要加固它的原因，须写进文档）。
- R6 `filex journal stats` 对损坏/截断的日志行**跳过并计数**，不中断输出、不报错退出。
- R7 观测期设**明确的决策门**（见 design.md §4），数据不足则继续观测，**不默认开建拦截**。
- R8 日志体积不得因本任务显著增长 —— 只新增两个 `omitempty` 字段，不新增审计事件类型。

## Non-Goals

**本轮不做（已移入 backlog B4 / B5）**
- 不建配额存储（`~/.filex/sessions/`）。
- 不做拦截、不做降级阶梯、不做 TTL 自动恢复、不做永久封禁。
- 不引入跨进程锁、不引入原子计数。
- 不给 `guard.Evaluate` 增加 `Tier` 入参。
- 不实现能力令牌，**连接口都不预留**（避免订下难改的协议）。

**永久不做**
- 不做全局限流系统。
- 不替代权限模型或 MCP profile。
- 不引入外部存储、守护进程、数据库、后台 goroutine。
- 不新建操作队列。
- 不做 `ppid` 会话派生 —— 已判定为不可靠且会误伤手工用户。

## Acceptance Criteria

- [ ] MCP 连接产生的审计条目带稳定 `session_id` 与 `session_source=mcp_connection`，同一连接内多次调用 ID 一致。
- [ ] CLI 设置 `FILEX_SESSION_ID` 后，连续多次调用的审计条目共享同一 ID，`session_source=env`。
- [ ] CLI 未设置时 `session_id` 为空、`session_source=absent`，且**行为与未加本功能时完全一致**（无提示、无拦截、无降级）。
- [ ] `filex journal stats` 可输出：总条目数、按 `session_id` 分组的操作数、按 `verb` / `guard_class` 的分布、`bypass=true` 的频次、`session_source` 的构成比例。
- [ ] `filex journal stats --json` 输出结构稳定，可被脚本消费。
- [ ] 固定 JSONL fixture 的聚合结果与手工核对一致（单测）。
- [ ] 损坏行被跳过并在输出中给出 `skipped_lines` 计数，命令退出码仍为 0。
- [ ] 新增字段缺省时，审计行序列化结果与旧格式逐字节一致（向后兼容）。
- [ ] `go test ./...` 通过；**无需** `-race`、无需多进程测试（因为没有共享可变状态 —— 这正是瘦版的收益）。
- [ ] 文档写明：`session_id` 是观测标签而非安全凭据，伪造无安全后果。
- [ ] 文档写明观测期的决策门（最小样本、判定口径、三种结论的后续动作）。

## 工作量

**S**。无新存储、无并发、无拦截路径，主要成本在 `journal stats` 的聚合实现与 MCP session 下传。

## 依赖

- 熔断子任务（`10-30-destructive-op-circuit-breaker`）**阶段 B1**：`journal.Entry` 的 `session_id` / `session_source` 字段。
- 其余部分独立，不依赖 guard 决策层。
