# 执行计划：破坏性操作熔断（瘦版）

> v2 变更：删去 v1 的阶段 B（哈希链）、阶段 G（审批队列）、阶段 H7（`warn` 灰度切换）。
> 这三项移入父任务 [backlog.md](../10-30-supply-chain-exec-safety/backlog.md)（B2 / B1 / 无需重启）。
> 阶段编号重排，总量从 8 阶段降到 7 阶段、从 M→L 降到 S→M。

## 阶段 A — P0 修复（S，独立可发布，建议最先落地）

- [x] A1 `rm --dry-run` 真正生效：`RmParams` 增 `DryRun`；`internal/ops/file_ops.go:322` 的 `Rm()` 在 `checkPolicy` 通过后返回预览并 `return`（预览含目标绝对路径、`purge` 与否、预计去向）；CLI（`cmd/filex/file_ops.go:155` 当前静默丢弃）与 MCP（`tools.go:119`）透传。
- [x] A2 删除 `RmParams.Yes`（`internal/ops/file_ops.go:312`）与 MCP schema 的 `yes`（`tools.go:125,384`）。本阶段的临时保守行为：`--purge` 在**无法打开真实终端**时直接拒绝并给出 safer alternative 提示；阶段 D 由 guard 正式接管这个判定。
- [x] A3 `cmd/filex/help.go:121` 的旗标描述与实现对齐（删掉已下线的 `--yes`，补 `--dry-run` 的真实语义）。
- [x] A4 单测：`rm --dry-run` 不改变文件系统（断言文件仍存在且 mtime 未变）；`--purge` 在非交互下被拒且错误码为 `EDenied`。

**依赖**：无。
**验证**：`go test ./internal/ops/... ./cmd/filex/...`；手工 `filex rm tmp.txt --dry-run` 后文件仍在。
**回滚点**：A2 的保守拒绝可通过临时旗标放开；**A1 不建议回滚**（回滚等于恢复虚假宣传）。
**CHANGELOG**：A1/A2/A3 全部列入 **Security Fixes**。

## 阶段 B — 审计字段扩展（S，可与 A、C 并行）

- [x] B1 `internal/journal/journal.go:14` 的 `Entry` 一次性补齐 design.md §3.1 的 9 个字段，**全部 `omitempty`**。
      其中 `session_id` / `session_source` 本子任务**只加字段不填值**，由观测子任务填充 —— 刻意一次改完，避免两次动结构体。
- [x] B2 补全审计覆盖（当前 `recordJournal` 仅 6 处）：`backup.RestoreBackup`、`backup.PruneBackups`、`upgrade` 流程、版本切换。
- [x] B3 单测：新字段缺省时序列化结果与旧格式逐字节一致（向后兼容断言）；补全的 4 个调用点各产出一条审计。

**依赖**：无。
**回滚点**：全 `omitempty` → 删字段即回到旧格式，老消费方无感。
**明确不做**：`entry_hash` / `prev_hash` / 锁文件 / `filex journal verify`。见 backlog B2，重启时必须同时给出锚点方案。

## 阶段 C — `internal/guard` 决策层（S→M）

- [x] C1 新建 `internal/guard/guard.go`：`Verb` / `Action` / `Outcome` / `Decision` / `Environment` / `Config`（见 design.md §2.1）。
- [x] C2 实现 `Evaluate` 为**纯函数**，内含 design.md §2.2 的 3 个**硬编码**分类：`purge_delete`、`machine_env_write`、`self_config_write`。其余一律 `allow`。
      实际落地时补了第 4 个分类 `self_update`：`filex upgrade` 原先直接读 `os.Stdin` 取确认，管道喂 `y` 即可自我授权，是本轮唯一被点名纳入的额外口子。
- [x] C3 路径匹配复用 `policy.matchPattern`（`internal/policy/policy.go:136`）—— 从 policy 包导出或提到共享位置，**不新写第二套匹配器**。
- [x] C4 `internal/policy` 的 `Config`（`policy.go:10`）增 `Guard` 块，**只有两个字段** `enabled` / `allow_bypass`（见 design.md §6）。无 `mode`、无 `classes`。
- [x] C5 `Confirmer` 接口 + `DenyConfirmer` + `BypassConfirmer`（`TTYConfirmer` 在阶段 E）。
- [x] C6 表驱动单测：4 分类 × {interactive, non-interactive} × {enabled, disabled} → 断言 outcome / class / reason / safer_alternative 四项齐全。覆盖率目标 ~100%。
      实测 `go test -cover ./internal/guard/...` = **85.7%**，未达 ~100%。缺口很具体：`TTYConfirmer.Confirm` 是 0%（`console.OpenInput()` 要真实控制台），其余是 `describe` 的一条兜底分支。决策核心 `Evaluate` / `classify` / `withinConfigRoot` 和 `prompt`（用注入的 reader 测）都是 100% —— 也就是说没被覆盖的正好是"必须有人坐在终端前"的那一段，与 E5、`docs/VERIFY.md` 的人工验收项完全对应。`-race` 在 `internal/guard` 与 `internal/journal` 上均通过。分类已从 3 个变成 4 个（见 C2），测试矩阵按 4 个跑。

**依赖**：无。
**审查门**：`Evaluate` 不得读文件、读环境变量、判断终端。若实现中发现需要 I/O，说明职责划错了，退回重构 —— 这一条直接决定本子任务的可测性。

## 阶段 D — 接入 ops 收敛点（M）

- [x] D1 `internal/ops/helpers.go` 新增 `gate(a guard.Action) (*ruling, *errs.Error)`，顺序固定为 `checkPolicy → guard.Evaluate → Confirmer.Confirm`。
      返回值是实现内部的 `ruling`（决策 + 确认来源）而不是裸 `*guard.Decision`，否则调用点拿不到写审计所需的来源信息。
- [x] D2 在高危 op 接入：`Rm`（`file_ops.go:322`）、`Write`（`:229`）、`Edit`（`edit.go:33`）、`EnvSet` / `EnvUnset`、`upgrade` 与版本切换。
      **偏差**：`backup prune` 未接入门 —— 它删的是备份而非用户数据，分类表里没有对应类别，硬塞一个类别会让"分类=不可逆的用户可见后果"这条判据失效。已记入 backlog。
- [x] D3 `recordJournal` 扩展为记录完整决策链：`guard_class` / `decision` / `decision_reason` / `confirm_source` / `bypass` / `result`。**被拒的操作也必须留痕**，否则无法审计"拦住了什么"。
- [x] D4 `internal/errs` 复用 `EDenied`，错误体携带结构化 `reason` + `safer_alternative`（不是把两者拼成一句散文）。
- [x] D5 集成测试：各高危 op 在 `DenyConfirmer` 下被拒、错误体含两个结构化字段、审计留痕完整。
- [x] D6 grep 核对无遗漏：确认不存在绕过 `gate` 的高危写路径。
      核对结论：`internal/ops` 内共 6 处 `gate(...)` 调用（rm / write / edit / env set / env unset / `GateUpgrade`），`GateUpgrade` 这一个门被升级、版本切换、旧版本清理三条流程共用。`mcp install` 的配置写入**未被门控** —— 它写的是 `~/.filex` 之外的客户端配置文件，不属于 self_config_write 的判据。

**依赖**：阶段 B（审计字段）、阶段 C（决策层）。
**关键约束**：`gate` 必须在 `internal/ops`，**不得**在 `cmd` 或 `internal/mcpserver` —— 否则另一条入口直接绕过。

## 阶段 E — CLI 确认通道与 bypass（S）

- [x] E1 实现 `TTYConfirmer`：Windows 用现有 `golang.org/x/sys/windows` 打开 `CONIN$`，非 Windows 打开 `/dev/tty`；**打不开即判定非交互，绝不回退 `os.Stdin`**。
      新建 `internal/console` 承载这一判定。实测发现的必要修正：仅打开 `CONIN$` 不够，还要求 stdin 本身是字符设备 —— 否则 agent 拉起、stdin 是管道的进程会在 `ReadFile` 上永久挂起。判定为非交互时返回 `console.ErrNoTerminal`，绝不回退 stdin。
- [x] E2 `cmd/filex` 入口绑定 `TTYConfirmer` + `Environment{Interactive: <能否打开真实终端>}`。
      `bindRuntime(verb, args)` 在任何 verb 执行前完成绑定；confirmer 绑定在 `ops.Runtime` 上，MCP 侧另行安装 `DenyConfirmer`。
- [x] E3 bypass 旗标（`--i-understand-this-is-destructive`）+ config `allow_bypass`；包装为 `BypassConfirmer`，**强制审计 `bypass=true` + 来源**。
      `upgrade --yes` / `-y` 保留为同一个 bypass 的别名（它历史上读的就是 `os.Stdin`，正是 agent 可控的通道），语义从"静默跳过"改成"可审计的 bypass"。
- [x] E4 集成测试（`integration` tag）：
      - **管道 stdin 喂 `y` 必须不通过** —— 本子任务的核心安全断言；
      - bypass 执行成功**且**审计含 `bypass=true`，缺记录即测试失败；
      - `allow_bypass=false` 时 bypass 旗标无效。
- [ ] E5 手工验证（CI 覆盖不到，须记录于 PR）：Windows 真实终端下 y/N 提示可用，确认后操作执行、拒绝后不执行。
      **未执行**：本轮交付边界是"只做本地改动、不提交"，真实终端交互无法由 agent 自证。`docs/VERIFY.md` 已把这一条标成需人工验收，review 时请在真实 cmd/Windows Terminal 跑一次 `filex rm x.txt --purge`。

**依赖**：阶段 D。
**审查门**：E4 的管道负例不通过不得合并。E5 是可用性验证，不可用 CI 绿替代。

## 阶段 F — MCP 侧加固（S，可与 E 并行）

- [x] F1 `internal/mcpserver` 绑定 `DenyConfirmer`，`Environment.Interactive` **恒 `false`**（stdio 已被 JSON-RPC 占用，永不提示）。
- [x] F2 移除 tool schema 的 `yes`（`tools.go:125`）与其读取（`:384`）；客户端传入时**忽略，不报错**。
- [x] F3 给 `rm` tool 增 `dry_run`，与 CLI 对齐 —— 给模型一个安全的探查手段。
- [x] F4 把 `server.go:18` 已有的 clientInfo 下传为 `actor=mcp:<clientName>`。
      实现为 `identity()` 一次返回 `actor` / `session_id` / `session_source`（观测子任务的连接级会话落在这里）；未初始化或未命名的连接拿到诚实的 `mcp` + 空会话，不借用别人的身份。
- [x] F5 deny 响应体结构化携带 `reason` + `safer_alternative`，便于 Agent 自主改用安全替代而非盲目重试。
- [x] F6 集成测试：模型传 `yes:true` 无任何授权效果；高危请求返回结构化 deny；`purge:true` 在 MCP 下恒被拒（请求 ≠ 授权）。
      **偏差**：测试放在新建的 `internal/mcpserver/gate_test.go`（包内测试，能把 HOME 指向临时目录、直接读回审计日志），而不是扩展 `cmd/filex/mcp_cmd_test.go`。CLI 侧的 MCP 测试只覆盖 `mcp install` 的配置写入，够不到这条链路。

**依赖**：阶段 D。

## 阶段 G — 文档与回归核对（S）

- [x] G1 `README.md` / `README.en.md` 的 `## 安全模型`（当前 250–283 行）：
      - **更正回收站语义**为 `~/.filex/trash`（当前文档称 Windows 回收站，与 `file_ops.go:348` 不符）；
      - 新增"确认通道与 bypass"小节；
      - 写明**可测性边界**（哪些性质由 CI 保证、哪些只手工验过）。
      可测性边界实际落在新建的 `docs/VERIFY.md`（含"真实终端 y/N 无法自动证明"这条），中英文 README 各加"供应链验证"小节指向它；README 的 `### 回收站` 已改写为 `### 恢复目录` 并写明"不是 Windows 回收站"。
- [x] G2 `skills/filex/SKILL.md`：旗标语义更新；新增"收到 deny 时 Agent 该怎么做"——**不要重试**，读 `safer_alternative`，或报告用户等人工确认。
- [x] G3 `cmd/filex/help.go`：旗标描述对齐（阶段 A3 已改一半，此处收尾）。
      收尾内容：`rm` / `env` / `write` / `edit` 四个入口补 bypass 旗标说明，`upgrade --yes` 从"Skip confirmation prompt"改成"可审计的 bypass"，并新增 `journal` 条目（观测子任务的 C1）。
- [x] G4 MCP tool description 更正 "recycle bin" 措辞（`tools.go:119`）。
- [x] G5 配置文档：`guard` 块两个字段与默认值，并说明两个逃生口（`enabled=false` 与 bypass 旗标）。
- [ ] G6 回归核对（非交互高频写路径，最可能被误伤）：`install.ps1`、`install.sh`、`.github/workflows/test-env.yml`、`filex upgrade`、MCP `ro` profile —— 逐一实测不受熔断影响。
      **静态核对已完成，动态实测未做**：`test-env.yml` 确认新集成探针已被现有命令行覆盖（流水线未改）；MCP `ro` 由 `internal/mcpserver` 包内测试覆盖。`install.ps1` / `install.sh` / `filex upgrade` 的真实执行会改动本机安装目录，属人工验收项 —— 尤其 `upgrade --yes` 语义变了，脚本调用方需要实跑一次确认。
- [x] G7 新建 `CHANGELOG.md`（仓库此前没有）：Unreleased 段按 Security / Added / Changed / Breaking 归类，历史段按 tag 日期补齐；四个 P0 修复进 Security，`rm --yes` 下线与退出码 3 进 Breaking。

**依赖**：全部阶段。
**风险点**：G6 的 `upgrade` 涉及大量文件写与版本目录操作，必须实测而非推断。

## 验证命令汇总

```bash
go vet ./... && go test ./...
go test -race ./internal/guard/... ./internal/journal/...
go test -tags=integration ./cmd/filex/... ./internal/ops/...
```

## 并行与关键路径

```
A（P0 修复，可独立发布）────────────────┐
B（审计字段）──────┐                    ├─→ G（文档与回归）
C（guard 决策层）──┴─→ D（ops 接入）──┬─→ E（CLI tty + bypass）
                                      └─→ F（MCP 加固）
```

关键路径为 `C → D → E`。阶段 A 与 B 可由任意顺序先行；E 与 F 并行。

## 本子任务明确不做

- 审批队列与 `filex approve/deny`（backlog B1 —— 重启前需先有真实的"离线审批"需求场景）。
- 审计哈希链与 `journal verify`（backlog B2 —— 重启须同时解决锚点）。
- 分类规则引擎 / 可配置分类（backlog B3 —— 扩到第 6–7 个分类时再引入）。
- `warn` 灰度档位：逃生口已有两个，第三个只增加配置面与测试组合。
- `filex trash list/restore`（backlog B6，建议作为独立 S 任务）。
- `policy.Config.BackupBeforeWrite` 死配置处置（backlog B7）。
