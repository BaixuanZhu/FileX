# 技术设计：会话消耗观测（瘦版）

## 1. 层次定位

```
入口层                          收敛层 (internal/ops)            基础层
─────────────────────────────  ──────────────────────────────  ─────────────────
cmd/filex/*.go                 ┌─ checkPolicy(path)          ┐
  ↓ 注入 SessionID(env|absent)  │  ① 权限（既有）              │  internal/policy
                               ├─ gate(Action)                │  internal/guard
internal/mcpserver/*.go        │  ② guard 决策（熔断子任务）   │
  ↓ 注入 SessionID(连接级)      ├─ 执行                        │
                               └─ recordJournal(… + session) ┘  internal/journal ★填值

                                        ↓ 读（离线、另一个进程）

cmd/filex/journal_stats.go  →  读 ~/.filex/journal/filex.jsonl  →  聚合输出
```

**三个"在哪一层"的明确答复：**

| 关注点 | 层 | 说明 |
|---|---|---|
| **记录** | `internal/journal`，由 `ops.recordJournal` 写入 | session 只是两个新字段，不改变写入时机 |
| **聚合** | `cmd/filex` 的新命令，**读时**完成 | 与写路径完全解耦，聚合逻辑崩了也不影响文件操作 |
| **拦截 / 降级** | **本任务不做任何拦截** | 这是刻意的；判定基础（会话身份可靠性）尚未被验证 |

> **本任务不在写路径上新增任何判定分支。** 这是 v2 最重要的设计约束：
> 观测代码的 bug 不应该有能力阻断用户的文件操作。

## 2. Session 身份

### 2.1 三种来源，不猜第四种

| 入口 | `session_id` | `session_source` | 稳定性 |
|---|---|---|---|
| MCP | `initialize` 时生成随机 ID，存入连接级 state，经 context 下传 | `mcp_connection` | 连接存活期内稳定 |
| CLI（有 env） | `FILEX_SESSION_ID` 原值（**仅做长度与字符集校验，不改写**） | `env` | 由编排器保证 |
| CLI（无 env） | 空字符串 | `absent` | — |

**为什么不做 `ppid` 派生**（v1 的退路，已删除）：

- 对**每次新起进程**的 Agent：ppid 每次不同 → 每次都算新会话 → 聚合结果全是单条会话，统计无意义。
- 对**同一终端**下的手工用户：多次正常操作被归为一个"会话" → 分布右尾被人工操作污染，正是我们最不想混进来的噪声。

两头都不准的派生键比没有键更糟：它会让数据看起来有结论。`absent` 是诚实的缺省。

### 2.2 ID 生成不需要密码学强度

`session_id` 是观测标签，**不参与任何授权判定**。伪造它的后果只是污染自己的统计 —— 没有安全后果。
因此：用 `crypto/rand` 取 16 字节 hex 即可（标准库，无新依赖），**不需要签名、不需要防伪、不需要校验归属**。

> 这一点必须写进 README：否则日后有人会误以为 `session_id` 是凭据，并基于它做授权。

### 2.3 透传路径

- MCP：`server.go:18` 已持有 clientInfo，在同处生成 session ID，与 `actor=mcp:<clientName>` 一起经 context 下传到 `ops`。
- CLI：进程启动时读一次 env，存入与 `actor="cli"` 相同的传递结构（熔断子任务阶段 D 已建立该通道）。
- `ops.recordJournal` 把两个字段原样写入 `journal.Entry`。

**不新增传递机制** —— 完全搭熔断子任务已建好的 actor 透传顺风车。

## 3. `filex journal stats`

### 3.1 命令形态

```
filex journal stats [--since=<dur|RFC3339>] [--until=<...>]
                    [--session=<id>] [--group-by=session|verb|class|source]
                    [--top=<n>] [--json]
```

默认行为（无参数）：读全量日志，输出概览。

### 3.2 输出内容

| 分区 | 字段 |
|---|---|
| 概览 | 日志行数、解析成功/跳过行数、时间跨度 |
| 按 session | session_id（截断显示）、`session_source`、操作数、高危操作数（`guard_class` 非空）、首末时间 |
| 按 verb | verb、次数、占比 |
| 按 guard_class | class、`decision` 分布（allow / require_confirmation / deny） |
| bypass | `bypass=true` 的次数与涉及的 session |
| source 构成 | `mcp_connection` / `env` / `absent` 各自占比 —— **这是决策门最关键的一项** |

`--json` 输出同一结构的机器可读形式，字段名固定，可被脚本消费。

### 3.3 实现要点（刻意朴素）

- **流式逐行读取 + 单次扫描**，内存只持聚合结果，不把整个日志读进内存。
- 解析失败的行：`skipped_lines++` 后继续，**不中断、不报错退出**（退出码 0）。半截写入的尾行是正常现象。
- 不读写任何新文件、**不加锁**、不要求日志静止 —— 边写边读得到的是一个稍旧的快照，对分布统计完全够用。
- 不做缓存、不建索引。日志规模（单用户、每次操作一行）远未到需要索引的量级。
- 复用 `journal.Entry` 结构体反序列化，**不写第二份 schema**。

### 3.4 敏感信息

`journal.IsSensitiveName`（`journal.go`）已有的脱敏逻辑作用于写入侧。`stats` 只做计数与分组，**不输出路径内容**（按 verb/class 聚合，不按路径聚合）—— 天然不会泄露新信息。

> 若日后要加按路径聚合，必须先过一遍脱敏，这是已知的待注意点。

## 4. 决策门（本任务的真正交付物）

观测期结束后，用 `journal stats` 的数据回答三个问题，**按结论分三条路**：

### 4.1 判定输入

| 指标 | 怎么取 |
|---|---|
| M1 会话键可靠性 | `session_source` 中 `mcp_connection` + `env` 的合计占比 |
| M2 高危操作的会话内分布 | 按 session 分组的"高危操作数"的 p50 / p99 / max |
| M3 拒绝与 bypass 频次 | `decision=deny` 次数、`bypass=true` 次数 |

### 4.2 最小样本与观察窗

- 至少 **1 个版本周期**，或累计 **≥ 500 条带非 absent session 的审计条目**，取后达成者。
- 样本必须包含**至少 2 个不同 MCP 宿主**的数据 —— 单一宿主的数据不能代表 `session_source` 的真实可得性。

### 4.3 三种结论与后续动作

| 条件 | 结论 | 动作 |
|---|---|---|
| M1 < 50% | 会话身份在真实环境中大多不可得 | **关闭 backlog B4**，不再考虑会话级配额；若仍担心高频风险，改走"按路径/按 verb 的无状态上限"这条完全不同的路 |
| M1 ≥ 50% 且 M2 的 p99 与 max 差距不大、无长尾 | 假设不成立 —— 没有"合起来异常"的真实样本 | **关闭 backlog B4**，把结论写入 README 的安全模型小节（"我们查过，没发现这个模式"比"我们防了"更诚实） |
| M1 ≥ 50% 且 M2 存在明显长尾 / M3 的 deny 频次集中在少数 session | 假设得到支持 | **重启 backlog B4**，用实测 p99 的若干倍定标阈值 —— 而不是凭感觉拍 |

**默认结论是"关闭"。** 数据不足、样本不够、或指标不支持，都走关闭或继续观测，**不默认开建拦截**。

## 5. 与权限模型的关系（必须写进文档）

- 本任务**不授予也不收紧任何能力**，是纯旁观者。
- `session_id` 是标签，不是凭据；不参与 `checkPolicy`、不参与 `guard.Evaluate`。
- MCP `ro` profile 下照常记录（读操作也计入分布），对判定无影响。

## 6. 兼容性与回滚

| 项 | 说明 |
|---|---|
| 新增字段 | 两个，全 `omitempty`（字段本身由熔断子任务阶段 B1 加入）→ 老消费方无感 |
| 一键去除 | 不设置 `FILEX_SESSION_ID` 即全为 `absent`；MCP 侧可单条 revert |
| 回滚成本 | 删除 `journal stats` 命令 + 两处填值 = 完整回滚，**无数据迁移、无状态清理** |
| 失败模式 | 聚合命令崩溃**不影响任何文件操作**（不在写路径上）；损坏日志行跳过 |
| 并发风险 | **无** —— 无共享可变状态、无锁、无跨进程协调 |
| 依赖 | 不新增第三方依赖（`crypto/rand` + `encoding/json` 均为标准库） |

> 对比 v1：跨进程锁、state 文件损坏 fail-safe、降级事件重复触发、TTL 时序 —— 这四类风险随"不建存储"一并消失。
