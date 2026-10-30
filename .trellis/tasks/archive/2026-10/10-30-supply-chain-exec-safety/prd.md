# 供应链与执行安全增强（父任务 · 最终版 v2）

> v2 策略调整（2026-10-30）：熔断收缩为瘦版，预算从"拦截系统"降级为"观测先行"。
> 被移出本轮范围的条目记录在 [backlog.md](./backlog.md)，不是丢弃，是等证据。

## 策略变更摘要

| 增强项 | v1 计划 | v2 最终范围 | 理由 |
|---|---|---|---|
| 签名标准化 | GoReleaser + Cosign + SLSA + SBOM + 可复现 | **不变**，且提升为首位 | 收益明确、可独立验证、与代码逻辑零耦合 |
| 破坏性操作熔断 | guard 规则引擎 + 审批队列 + 哈希链 + 能力令牌 | **瘦版**：3 个 P0 修复 + tty/deny 双通道 + 审计字段 | 真实风险集中在 3 个具体缺陷；审批队列为不存在的场景建状态机 |
| 执行预算/降级 | 跨进程配额存储 + 四档降级阶梯 + TTL | **观测模式**：session 身份入审计 + 离线聚合查询 | 计数键（会话身份）不可靠时，配额判定基础不成立；先用数据回答"问题是否存在" |

### 两个关键的诚实判断（决定了 v2 范围）

1. **无外部锚点的审计哈希链对本地攻击者是安全剧场** —— 能改日志的人也能重算整条链。它的真实价值是检测意外损坏与半截写入，不是防篡改。按这个价值定位，它是可选小项，移入 backlog。
2. **filex 无法可靠知道"会话"是什么**。`ppid` 派生是猜测：对每次新进程的 Agent 会失效，对同终端的手工用户会误伤。键不可靠 → 用全计划最复杂的跨进程锁 + 阶梯状态机换一个判定基础不牢的防护。因此 v2 **不建计数存储**，改为从既有 JSONL 读时聚合。

## Goal

1. **签名标准化** — 把 SHA256 checksums 升级为可被第三方独立验证的供应链证据链。
2. **破坏性操作熔断（瘦版）** — 修掉三个"宣称安全但实际不安全"的缺陷，并为不可逆操作提供独立于模型的确认/拒绝通道。
3. **会话消耗观测** — 让"单动作合法、合起来异常"这个假设可被数据检验，再决定是否值得建拦截。

父任务不承载实现，只负责需求源、任务映射、跨子任务验收与最终集成评审。

## 现状调研结论（2026-10-30，基于实际代码）

### 前提校正

需求提出时描述的若干能力在本仓库**不存在**，计划按实际代码编写：

| 描述的能力 | 实际情况 |
|---|---|
| 自托管 Web 文件管理器 | Windows 优先的单体 CLI，verb 分发在 `cmd/filex/main.go:38`，无 HTTP server |
| 29 种命名权限模型 | 只有 2 档 MCP profile（ro/rw）`internal/mcpserver/profile.go:10` + 路径策略 `internal/policy/policy.go:10` |
| API Token verb scopes | 不存在；`scope` 仅指环境变量作用域（process/user/machine） |
| 持久化操作队列 | 不存在（全仓 grep `queue` 无匹配） |
| `filex client` 瘦客户端 | 不存在 |
| MCP 端点 | stdio JSON-RPC（`internal/mcpserver/server.go:52`），非 HTTP |
| Windows 回收站 | 实际移动到 `~/.filex/trash`（`internal/ops/file_ops.go:348`），无 restore 命令 |

### 架构事实（三项共用）

- **唯一收敛点是 `internal/ops`**：所有文件操作都在该层调用 `checkPolicy`（10 处，`internal/ops/helpers.go:14`）；CLI 与 MCP 两条入口都经过它。
  → **熔断必须落在 `internal/ops`。放在 `cmd/filex` 或 `internal/mcpserver` 必然被另一条入口绕过。**
- 审计是 append-only JSONL + 全局 mutex（`internal/journal/journal.go:27`），字段固定、无 actor/session。
- `recordJournal` 仅 4 处（write/edit/rm×2）+ env 2 处；**backup restore/prune、upgrade、版本切换无审计**。
- Go 1.26.5，约 7.5k 行，唯一依赖 `golang.org/x/sys`。

### 已发现的现成缺陷

| # | 问题 | 位置 | 处置 |
|---|---|---|---|
| 1 | **`rm --dry-run` 实际会删除**，而 help 对外宣传该旗标 | `cmd/filex/file_ops.go:155` vs `cmd/filex/help.go:121` | 熔断 P0 |
| 2 | **`--yes` 是空实现**，`RmParams.Yes` 从未被读取 → 当前**不存在任何确认通道** | `internal/ops/file_ops.go:312` / `:322-376` | 熔断 P0 |
| 3 | **MCP 让模型自填 `yes` 自我授权** | `internal/mcpserver/tools.go:383-384` | 熔断 P0 |
| 4 | **安装器 `.exe` 未纳入 `checksums.txt`**，但该 exe 被发布 | `release.yml:440` vs `:509` | 签名 P0 |
| 5 | **CI 无通用 `go test ./...`**（仅 `test-env.yml` 限定路径） | `.github/workflows/test-env.yml:42` | 签名 阶段 A（**收益最高的单项**） |
| 6 | `policy.Config.BackupBeforeWrite` 默认 true 但无调用点（死配置） | `policy.go:13` / `file_ops.go:255` | backlog |
| 7 | trash 无 list/restore 命令 | `internal/ops/file_ops.go:348` | backlog |
| 8 | `LICENSE` 未被 git 跟踪，而发布流程引用它 | `release.yml:69` | 签名 阶段 A |

## 已拍板决策

| 决策项 | 结论 |
|---|---|
| 任务结构 | 父任务 + 3 子任务 |
| 发布流水线 | **迁移 GoReleaser**（只接管 build/archive/checksums/SBOM，NSIS job 保留并消费 `dist/`） |
| 仓库身份 | **BaixuanZhu/FileX**（已由 `git remote -v` 核实） |
| 回收站语义 | **改文档对齐代码**（`~/.filex/trash`），不改为真 Windows 回收站 |
| 熔断范围 | **瘦版**：无审批队列、无规则引擎、无 `warn` 灰度档；逃生口 = `guard.enabled=false` + 显式 bypass 旗标 |
| 预算范围 | **观测先行**：不建状态存储、不做拦截与降级；从 JSONL 读时聚合 |
| 哈希链 | 移入 backlog，并按"损坏检测"而非"防篡改"定位 |

## 范围与优先级（最终）

| 顺序 | 子任务 | 范围 | 优先级 | 工作量 |
|---|---|---|---|---|
| 1 | `10-30-signing-standardization` | 增强项 1 + 通用 CI + 安装器 checksum + LICENSE 跟踪 | **P1** | M→L |
| 2 | `10-30-destructive-op-circuit-breaker` | 3 个 P0 修复 + tty/deny 通道 + 审计字段扩展 | **P0**（内含 P0 修复） | S→M |
| 3 | `10-30-session-usage-observability` | session 身份入审计 + `journal stats` 聚合 + 决策门 | **P3** | S |

### 依赖与并行

- 子任务 1 **与** 2/3 完全并行：前者只动 `.github/`、`.goreleaser.yaml`、`docs/`；后者只动 `internal/`、`cmd/`。
- 子任务 3 依赖子任务 2 的 journal 字段扩展（`actor` / `session_id`），其余独立。
- 子任务 2 的 P0 修复（阶段 A）**不依赖任何其它工作**，建议最先落地。

### 建议实施顺序

1. **签名 阶段 A**（通用 CI + LICENSE）—— 先建测试网，收益最高。
2. **熔断 阶段 A**（3 个 P0 修复）—— 一天量级，消掉绝大部分实际风险。
3. 签名 阶段 B–G（可与下一步并行）。
4. 熔断 阶段 B–E。
5. 观测子任务（数据收集期开始计时）。

## 可测性契约（CI 覆盖边界）

明确声明哪些性质由 CI 保证、哪些只能手工验证 —— 避免把"CI 绿"误读为"安全性质成立"。

### CI 可自动覆盖（目标：逻辑层 85–90%）

| 性质 | 测试形态 |
|---|---|
| `guard.Evaluate` 决策正确性 | 表驱动单测，纯函数无 I/O，可达 ~100% |
| `rm --dry-run` 不改变文件系统 | 单测 + 集成 |
| **管道 stdin 喂 `y` 必须被拒绝** | 集成测试（**安全相关的那一半，可稳定自动化**） |
| MCP 侧 `yes:true` 无授权效果、deny 响应结构 | 扩展 `cmd/filex/mcp_cmd_test.go` |
| bypass 必留审计（缺记录即失败） | 集成断言 |
| `journal stats` 聚合正确性 | 固定 JSONL fixture 单测 |
| 二进制 bit-for-bit 可复现 | 双构建比对 job |
| 产物清单完整性（签名/SBOM/provenance/checksums 行数） | release job 断言 |
| 篡改产物后验证必败 | 负例 job |

### CI 覆盖不到（必须手工 / 必须在文档中承认）

| 性质 | 为什么 | 替代验证 |
|---|---|---|
| **真实终端确认路径可用** | runner 无交互控制台 | 手工在 Windows 终端验一次（可用性问题，非安全问题） |
| 真实 MCP 宿主是否传 session ID | 取决于宿主实现 | 观测期数据中看 `session_source` 分布 |
| "Agent 真的被拦住了"的端到端行为 | 依赖具体 Agent 实现 | 人工跑一次真实 Agent 会话 |
| NSIS 安装器可复现 | NSIS 嵌入时间戳，**明确声明不保证** | 仅 checksum + 签名覆盖 |
| 跨进程锁真实竞态时序 | 易 flaky | v2 已通过"不建状态存储"规避该风险 |

## Acceptance Criteria（跨子任务）

- [ ] 三个子任务各自验收通过并归档。
      代码侧全部完成，归档待人工 review；本轮交付边界是"只改本地、不提交"。
- [ ] `go test ./...` 在 CI（linux + windows）对 PR 生效且通过。
      `.github/workflows/ci.yml` 已建；**未在 GitHub 上跑过**（无提交）。本地 `go vet ./... && go test ./...` 全绿。
- [ ] 一次 `v*-rc*` 预发布 tag 完整跑通：归档 + checksums（**含安装器**）+ `.sig`/`.pem` + provenance + SBOM 全部就位。
      **本轮不做**：签名阶段 C–G 只能在 GitHub 真实发布时验收。
- [ ] 干净机器按 `docs/VERIFY.md` 三步验证成功，且篡改任一产物后必然失败。
      文档已写；cosign/provenance 尚未产出，VERIFY.md 里明确标注了"还没有的东西"。
- [x] `rm --dry-run` 不再删除任何内容。（单测 + 真实二进制探针：文件仍在、trash 目录未创建、日志文件大小不变）
- [x] 不可逆操作在非交互场景被 deny 并返回结构化 reason + safer_alternative。
      **后半句未证**：交互场景的真实终端确认为人工验收项（熔断 E5）。
- [x] **管道喂 `y` 不能通过确认**（本轮核心安全断言）。
- [x] MCP schema 不再暴露 `yes`，模型传入无授权效果。
- [x] bypass 路径可用且 100% 留痕（探针断言审计里必须有 `bypass=true` + 来源，缺记录即失败）。
- [x] `filex journal stats` 可输出按 session / verb 的消耗分布；`session_source` 可区分真实会话与缺失。
- [ ] 现有非交互路径无回归：`install.ps1`、`install.sh`、`test-env.yml`、`filex upgrade`、MCP ro profile。
      静态核对完成（`test-env.yml` 未改即已覆盖新探针；MCP ro 有包内测试）；`install.*` 与 `filex upgrade` 的实跑属人工验收 —— `upgrade --yes` 语义已变，脚本调用方必须实跑一次。
- [x] 文档与行为一致：README × 2、SKILL.md、`help.go`、MCP tool description（含回收站语义更正）、`CHANGELOG.md`（新建）、`docs/VERIFY.md`（新建）。
- [x] 本文件的"可测性契约"已落到 README 或 docs，不留口头承诺。（`docs/VERIFY.md` 的"可测性边界"表逐条映射到测试与它的上限）

## 风险与回滚方案

| # | 风险 | 影响 | 缓解 | 回滚方式 |
|---|---|---|---|---|
| R1 | GoReleaser 迁移打断现有 NSIS / 安装测试链 | 发布流水线不可用 | 先在 `v*-rc` tag 验证全链路；归档命名与内部结构逐项对照旧产物 | 保留 `release-legacy.yml`，revert 单文件 |
| R2 | cosign identity 写错 | 用户验证全部失败 | 已核实 remote；CI 内自验 `cosign verify-blob` | 补发 `.sig`/`.pem`，**不需重打 tag** |
| R3 | 声称可复现但实际不可 | 过度承诺 | 文档划定范围（NSIS 不保证）；双构建持续校验 | 调整文档声明，不移除签名 |
| R4 | 非交互下拒绝 `--purge` 打断既有自动化 | 现有脚本卡住 | 同版本提供 bypass 旗标并在 CHANGELOG 标为 breaking；默认分类只覆盖真正不可逆 | `guard.enabled=false`，**无需回滚代码** |
| R5 | MCP 移除 `yes` 字段 | 已有客户端配置失效 | 该字段本就是空实现（零行为变化）；传入时忽略而非报错 | 保留字段并忽略 |
| R6 | 修复 `rm --dry-run` 改变既有行为 | 依赖"dry-run 其实会删"的脚本失效 | 实为修复虚假宣传，CHANGELOG 列 **Security Fix** | **不建议回滚** |
| R7 | `filex upgrade` 因资产命名/归档结构变化失败 | 用户无法自升级 | GoReleaser 命名模板与归档结构对齐旧产物；rc tag 实测 `upgrade` | revert 到 legacy workflow |
| R8 | 观测期数据不足或无代表性 | 决策门无法判定 | 明确最小样本与观察窗；数据不足就继续观测，**不默认开建拦截** | 无需回滚（纯读操作） |
| R9 | bypass 被滥用成默认姿势 | 熔断形同虚设 | bypass 旗标名刻意冗长；每次必审计；`journal stats` 统计 bypass 频次 | 配置 `allow_bypass=false` |

> v1 的 R7（跨进程计数损坏）、R9（哈希链兼容性）、R6（审批队列死锁）**已因 v2 收缩范围而消失** —— 这是瘦版最直接的收益。

## 文档与发布说明变更清单

| 文件 | 变更 | 归属 |
|---|---|---|
| `docs/VERIFY.md`（新建） | cosign + Rekor 验签、provenance 核对、可选复现构建比对、**可复现范围声明** | 签名 |
| `.github/workflows/ci.yml`（新建） | 通用 build/vet/test + 双构建比对 | 签名 |
| `.goreleaser.yaml`（新建） | build / archive / checksums / SBOM | 签名 |
| `.github/workflows/release.yml` | GoReleaser 迁移、安装器哈希纳入、sign/provenance job、Release body 增 Verification 段 | 签名 |
| `Makefile` | `BUILD_TIME` 改用 commit date | 签名 |
| `LICENSE` | **纳入 git 跟踪**（当前未跟踪但被发布流程引用） | 签名 |
| `README.md` / `README.en.md` `## 安全模型` | ①回收站语义更正为 `~/.filex/trash` ②新增"确认通道与 bypass" ③新增"供应链验证" ④新增"可测性边界"（哪些由 CI 保证） | 三者 |
| `skills/filex/SKILL.md` | 旗标语义更新；**Agent 收到 deny 时不要重试**，应报告并等待人工确认 | 熔断 |
| `cmd/filex/help.go` | rm 旗标描述对齐；新增 `journal stats` | 熔断 + 观测 |
| `internal/mcpserver/tools.go` | rm tool description 更正 "recycle bin" 措辞 | 熔断 |
| 配置文档 | `~/.filex/config.json` 的 `guard` 块字段与默认值 | 熔断 |
| CHANGELOG | **Security Fixes** 单列：dry-run 真删、`--yes` 空实现、MCP 自我授权参数、安装器 checksum 缺失；**Breaking**：非交互 purge 默认拒绝 | 三者 |

## Non-Goals（本轮明确不做）

**供应链**
- 不追求 SLSA 最高等级，不做正式认证流程；不引入长期私钥 / KMS / 自建 CA。
- 不保证 NSIS 安装器 bit-for-bit 可复现。
- 不签名每个独立归档（以 `checksums.txt` 为单一信任根）。

**熔断**
- 不建审批队列、不建分类规则引擎、不做 `warn` 灰度档。
- 不把所有写操作变成人工确认；只覆盖真正不可逆的类别。
- 不重建权限系统；不扩展 MCP profile；不引入 API Token。
- 不做审计哈希链（已按"安全剧场"判断移入 backlog）。
- 不改 `rm` 为真 Windows 回收站。

**观测**
- 不建配额存储、不做拦截、不做降级阶梯、不做 TTL 恢复。
- 不做 `ppid` 会话派生（不可靠且会误伤手工用户）。
- 不实现能力令牌，**连接口都不预留**（避免订下难改的协议）。
- 不做全局限流、分布式配额、守护进程。

**通用**
- 不新建操作队列。
- 不为假设中的未来需求做抽象；需要时再加。

## 观测期与决策门（跨子任务）

观测子任务的真正交付物不是 `journal stats`，而是**一个用实测数据才能做、且默认不做的事**。门槛完整定义在子任务 `10-30-session-usage-observability/design.md` §4，此处复制结论，避免只存在于任务内部文档里。

**观测期起始：2026-10-30**；本轮落地前最新已发布 tag 为 `v0.3.0`（2026-10-29），观测窗从下一个发布版本起算。

| 指标 | 取法 |
|---|---|
| M1 会话键可靠性 | `journal stats --json` 里 `session_source` 为 `mcp_connection` + `env` 的合计占比 |
| M2 高危操作的会话内分布 | 按 session 分组的高危操作数的 p50 / p99 / max |
| M3 拒绝与 bypass 频次 | `decision=deny` 次数、`bypass=true` 次数 |

**最小样本**：≥1 个版本周期 **或** 累计 ≥500 条带非 absent session 的条目（取后达成者），且样本必须覆盖 **≥2 个不同 MCP 宿主** —— 单一宿主不能代表 `session_source` 的真实可得性。

| 条件 | 结论 | 动作 |
|---|---|---|
| M1 < 50% | 会话身份在真实环境大多不可得 | 关闭 backlog B4；若仍担心高频风险，改走"按路径/按 verb 的无状态上限"这条完全不同的路 |
| M1 ≥ 50% 且 M2 无明显长尾 | "单动作合法、合起来异常"没有真实样本 | 关闭 backlog B4，把"我们查过，没发现这个模式"写进 README 安全模型 |
| M1 ≥ 50% 且 M2 存在明显长尾 / M3 的 deny 集中在少数 session | 假设得到支持 | 重启 backlog B4，用实测 p99 的若干倍定标阈值 |

**默认结论是"关闭"。** 数据不足、样本不够或指标不支持，都走关闭或继续观测，**不默认开建拦截**。

## Backlog

被移出本轮的条目及其重启条件见 [backlog.md](./backlog.md)。
