# 执行计划：会话消耗观测（瘦版）

> v2 变更：删去 v1 的阶段 B（最小计数存储）、阶段 C（配额规则与降级阶梯）、阶段 E（能力令牌接口预留）。
> 这三项移入父任务 [backlog.md](../10-30-supply-chain-exec-safety/backlog.md) B4 / B5。
> 工作量从 M→L 降到 **S**；不再需要跨进程锁、`-race`、多进程测试。

> **前置依赖**：熔断子任务（`10-30-destructive-op-circuit-breaker`）**阶段 B1** —— `journal.Entry` 的
> `session_id` / `session_source` 字段（该子任务只加字段不填值，本任务负责填值）。
> 阶段 C（`journal stats`）可用手写 fixture 提前开发，不必等依赖。

## 阶段 A — Session 身份下传（S）

- [x] A1 `internal/mcpserver`：`initialize` 时用 `crypto/rand` 生成 16 字节 hex 作为 session ID，存入连接级 state，与 `actor=mcp:<clientName>` 一起经 context 下传（搭熔断子任务阶段 F4 已建好的 actor 通道，**不新建传递机制**）。
      落地形态：`identity()` 一次返回 actor / session id / source，会话 id 在同一连接的重复 `initialize` 下保持不变（有专门测试钉住）。
- [x] A2 `cmd/filex`：进程启动时读一次 `FILEX_SESSION_ID`，仅做长度与字符集校验（拒绝控制字符与超长值，防日志注入），**不改写取值**。
- [x] A3 未设置时：`session_id` 留空、`session_source="absent"`。**不派生、不猜测**（见 design.md §2.1 —— 这是 v2 刻意删除的 v1 行为）。
      **偏差（与 B1/兼容性冲突时的取舍）**：`absent` 不落盘成字面量，而是让 `session_source` **留空**（`omitempty`），这样"没有会话"的记录与加本功能之前的字节完全一致；`journal stats` 读取时把空值归入 `absent` 桶。常量 `SessionSourceAbsent` 记录了这个归一化方向，A5 的逐字节兼容断言因此成立。
- [x] A4 `ops.recordJournal` 把两个字段原样写入 `journal.Entry`。
- [x] A5 单测：
      - 三种来源各产出正确的 `session_source`；
      - 同一 MCP 连接内多次调用 `session_id` 一致；
      - `absent` 场景下序列化结果与未加本功能时**逐字节一致**（向后兼容断言）；
      - 含控制字符/超长的 `FILEX_SESSION_ID` 被拒并退化为 `absent`。
      分布在三处：`internal/journal/session_test.go`、`internal/journal/compat_test.go`、`internal/mcpserver/gate_test.go`，另加 `cmd/filex/gate_integration_test.go` 的 4 个真实二进制探针（环境变量→分组→非法值→stats 只读）。

**依赖**：熔断子任务 B1。
**关键约束**：A1–A4 **不得在写路径上新增任何判定分支** —— 只填字段。观测代码的 bug 不应有能力阻断文件操作。
**回滚点**：两处填值 revert 即完整回滚，无数据迁移。

## 阶段 B — `filex journal stats` 聚合实现（S）

- [x] B1 新增 `cmd/filex/journal_stats.go`：命令形态见 design.md §3.1（`--since` / `--until` / `--session` / `--group-by` / `--top` / `--json`）。
      实际是两个文件：聚合逻辑在 `internal/journal/stats.go`（`Collect` / `Scan` / `Report`），`cmd/filex/journal_stats.go` 只做参数解析与 `text/tabwriter` 渲染。理由：schema 归 journal 所有，把聚合放 CLI 会让"日志字段变了但忘了改统计"成为可能。**写路径仍然零调用** —— 这条核心约束没被破坏，`journal stats` 坏了也碰不到文件操作。
      另一处修正：`--since=48h` / `--until=48h` 这类时长是**从现在往前推**，不是往后；第一版按后者实现，把窗口内所有记录都滤掉了。
- [x] B2 **流式逐行扫描**，复用 `journal.Entry` 反序列化（**不写第二份 schema**）；内存只持聚合结果。
      `bufio.Scanner` + 上限 1 MiB 的行缓冲；超长行按"损坏行"跳过，不抛错。
- [x] B3 聚合六个分区：概览、按 session、按 verb、按 `guard_class`（含 `decision` 分布）、bypass 频次、`session_source` 构成比。
- [x] B4 损坏/截断行：`skipped_lines++` 后继续，**退出码仍为 0**（半截尾行是正常现象，不是错误）。
- [x] B5 `--json` 输出字段名固定，可被脚本消费。
      `journal_stats_test.go` 里有两份形状测试：顶层 + `gate` 的字段名逐个钉住，空列表序列化成 `[]` 而不是 `null`。
- [x] B6 **不加锁、不要求日志静止**；不做缓存、不建索引。
- [x] B7 单测（固定 JSONL fixture，含正常行 + 损坏行 + 旧格式无 session 字段的行）：
      - 各分区聚合结果与手工核对一致；
      - 损坏行被跳过且计数准确；
      - 旧格式行能正常参与聚合（归入 `absent`）；
      - 时间窗过滤边界正确。
      `internal/journal/stats_test.go` 用 9 行 fixture（含一条无时间戳、一条损坏、一条截断尾行）钉住 `lines = parsed + skipped` 不变式、构成比之和≈1、决策链计数、p50/p99/max、`--top` 截断与"省略 N 条"计数；无日志时 `Collect` 返回全零且**不创建 journal 目录**。

**依赖**：无（fixture 可手写，不必等阶段 A）。
**审查门**：B6 一旦需要加锁，说明设计走偏了 —— 只读聚合不需要原子性。

## 阶段 C — 文档与观测期启动（S）

- [x] C1 `cmd/filex/help.go` 增 `journal stats` 条目。
      `journal` 条目含 `stats` / `path` 两个动作与 `--since` / `--until` / `--session` / `--group-by` / `--top` / `--json`；`main.go` 的 usage 行同步加了一行。
- [x] C2 `README.md` / `README.en.md` `## 安全模型`增"会话消耗观测"小节，必须写明：
      - `session_id` 是**观测标签而非安全凭据**，伪造无安全后果、不参与任何授权判定；
      - `session_source=absent` 是正常状态，不触发任何行为变化；
      - 本功能**不拦截、不降级、不限流**。
- [x] C3 `skills/filex/SKILL.md`：说明编排器可通过设置 `FILEX_SESSION_ID` 让一轮任务的操作可被归组（**可选，不设置无任何影响**）。
- [x] C4 把 design.md §4 的决策门（最小样本、三项指标、三种结论）落到 `docs/` 或父任务，**不留口头承诺**。
      已把判定输入、最小样本与三种结论的完整表复制到父任务 `10-30-supply-chain-exec-safety/prd.md` 的"观测期与决策门"小节，README 的"会话消耗观测"小节不再只指向任务内部文档。
- [x] C5 观测期开始计时：记录起始日期与当前版本号。
      **起始 2026-10-30**，本轮改动落地前的最新已发布 tag 为 `v0.3.0`（2026-10-29）；观测窗从下一个发布版本起算，样本要求 ≥1 个版本周期或 ≥500 条非 absent 条目（取后达成者）且覆盖 ≥2 个 MCP 宿主。起始日期与版本号已写入父任务 PRD。

**依赖**：阶段 A、B。

## 阶段 D — 决策门结算（观测期后，不在本轮实施范围内）

- [ ] D1 达到最小样本（≥1 个版本周期 **或** ≥500 条非 absent 条目，取后达成者；且覆盖 ≥2 个 MCP 宿主）后，跑 `journal stats --json` 取 M1/M2/M3。
- [ ] D2 按 design.md §4.3 三条路之一下结论。
- [ ] D3 **结论为"关闭"时**：把"我们查过，没发现这个模式"写入 README 安全模型小节，并关闭 backlog B4。
- [ ] D4 **结论为"重启"时**：以实测 p99 的若干倍定标阈值，重开 backlog B4 的规划（阈值不得凭感觉拍）。

**默认结论是"关闭"。** 数据不足、样本不够、指标不支持 → 继续观测或关闭，**不默认开建拦截**。

## 验证命令汇总

```bash
go vet ./... && go test ./...
go test ./cmd/filex/... ./internal/journal/...
```

**明确不需要**：`-race`（无共享可变状态）、多进程集成测试（无跨进程协调）、锁竞态测试。
这正是放弃计数存储换来的直接收益。

## 本子任务明确不做

- 配额存储 `~/.filex/sessions/`、跨进程锁、原子计数（backlog B4）。
- 降级阶梯、TTL 自动恢复、`guard.Evaluate` 的 `Tier` 入参（backlog B4）。
- 能力令牌 —— **连接口都不预留**（backlog B5）。
- `ppid` 会话派生（已判定不可靠，永久不做）。
- 按路径聚合（需先过脱敏，见 design.md §3.4；当前只按 verb/class 聚合）。
- 全局限流、分布式配额、守护进程、后台 goroutine。
