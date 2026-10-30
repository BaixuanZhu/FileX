# 技术设计：破坏性操作熔断（瘦版）

## 1. 分层与拦截点

```
入口层                        收敛层 (internal/ops)              基础层
───────────────────────────  ────────────────────────────────  ──────────────
cmd/filex/*.go               ┌─ checkPolicy(path) ───────────┐  internal/policy
  ↓ 注入 TTYConfirmer          │  ① 权限（既有，不动）          │
    + Interactive=<真终端?>    ├─ gate(Action) ────────────────┤  internal/guard ★新增
                             │  ② guard.Evaluate → Decision  │  （纯函数 + 接口）
internal/mcpserver/*.go      │  ③ Confirmer.Confirm(...)     │
  ↓ 注入 DenyConfirmer         ├─ 执行（既有 os/winfs 调用）    │  internal/winfs
    + Interactive=false      └─ recordJournal(决策 + 结果) ───┘  internal/journal（扩展字段）
```

**三个"在哪一层"的明确答复：**

| 关注点 | 层 | 为什么不放别处 |
|---|---|---|
| **拦截** | `internal/ops.gate()`，在 `checkPolicy` 之后、真实 syscall 之前 | 这是 CLI 与 MCP 的唯一收敛点；放入口层则另一条路径绕过 |
| **记录** | `internal/journal`，由 `gate()` 与各 op 末尾写入 | 决策与结果必须同源记录，否则无法审计"拒绝了什么" |
| **拒绝** | `internal/guard` 产出决策，`internal/ops` 执行 | 入口层只提供 `Confirmer` 实现，**无权改写决策** |

**明确排除的反模式**：不在 MCP tool description 写"请谨慎"之类提示词约束；不把 `yes` 交给模型。约束必须在代码路径上，不在文字里。

## 2. `internal/guard` 包（刻意做小）

### 2.1 数据结构

```go
package guard

type Verb string

const (
    VerbRm       Verb = "rm"
    VerbWrite    Verb = "write"
    VerbEdit     Verb = "edit"
    VerbEnvSet   Verb = "env_set"
    VerbEnvUnset Verb = "env_unset"
)

type Action struct {
    Verb  Verb
    Path  string // 绝对路径
    Purge bool   // 不可逆删除
    Scope string // env: process/user/machine
    Actor string // "cli" / "mcp:<clientName>"
}

type Outcome string

const (
    OutcomeAllow        Outcome = "allow"
    OutcomeNeedsConfirm Outcome = "require_confirmation"
    OutcomeDeny         Outcome = "deny"
)

type Decision struct {
    Outcome          Outcome
    Class            string // 命中的硬编码分类名，用于审计追溯
    Reason           string
    SaferAlternative string
}

type Environment struct {
    Interactive bool // 由入口层注入；MCP 恒为 false
}

type Config struct {
    Enabled     bool `json:"enabled"`
    AllowBypass bool `json:"allow_bypass"`
}

// 纯函数，无 I/O —— 这是 ~100% 单测覆盖的前提
func Evaluate(a Action, cfg Config, env Environment) Decision
```

> **设计约束**：`Evaluate` 不得读文件、不得读环境变量、不得判断终端。
> 若实现时发现需要 I/O，说明职责划错了，退回重构 —— 这直接决定了可测性。

### 2.2 硬编码分类（v2 不做规则引擎）

| Class | 匹配条件 | Interactive | Non-interactive | Safer Alternative |
|---|---|---|---|---|
| `purge_delete` | `Verb=rm && Purge` | `require_confirmation` | `deny` | "去掉 `--purge`，文件将移入 `~/.filex/trash` 可恢复" |
| `machine_env_write` | `Verb=env_set\|env_unset && Scope=machine` | `require_confirmation` | `deny` | "改用 `--scope=user`，仅影响当前用户" |
| `self_config_write` | `Verb=write\|edit && Path 命中 ~/.filex/**` | `require_confirmation` | `deny` | "filex 的策略配置被改写等于放宽自身限制；如确需修改请在终端手工编辑" |

其余操作一律 `allow`（路径策略已在上一层把关）。

路径匹配复用 `policy.matchPattern`（`internal/policy/policy.go:136`）—— 需将其从 policy 包导出或提到共享位置，**不新写第二套匹配器**。

**为什么硬编码**：v2 只有 3 个类且稳定。为 3 个类建配置化规则引擎是过早抽象（规则引擎当初一半动机是服务已被移出的预算规则）。扩展到第 6–7 个类时再引入，见 backlog B3。

### 2.3 Confirmer 接口与三个实现

```go
type Confirmer interface {
    // 返回 (已确认, 确认来源)。来源写入审计。
    Confirm(a Action, d Decision) (confirmed bool, source string, err error)
}
```

| 实现 | 绑定方 | 行为 | 审计 `confirm_source` |
|---|---|---|---|
| `TTYConfirmer` | CLI | **仅当能打开真实终端**才提示 y/N | `tty` |
| `DenyConfirmer` | MCP | 永不提示，直接返回未确认 | `none` |
| `BypassConfirmer` | 显式 bypass 旗标（包装上面两者） | 直接确认，**强制审计** | `bypass:flag` |

### 2.4 TTY 独立性实现要点（本子任务的安全核心）

- **不能**只用 `os.Stdin.Stat()` 判 `ModeCharDevice` —— 该判断在某些管道/伪终端场景会误判。
- Windows：`syscall.Open("CONIN$")` 或 `windows.CreateFile("CONIN$", ...)` 直接拿控制台输入句柄；现有依赖 `golang.org/x/sys/windows` 已足够。
- 非 Windows：打开 `/dev/tty`。
- **打不开 → 视为非交互，不回退到 `os.Stdin`**。这是防"Agent 管道喂 `y`"的全部要害。
- 读取后恢复/关闭句柄，不影响后续 stdout JSON 输出。

> 这一条是本子任务唯一真正的安全性质；CI 能覆盖它的**负例**（管道必被拒），正例需手工验。

## 3. 审计扩展（不做哈希链）

### 3.1 `journal.Entry` 新增字段（全部 `omitempty`，向后兼容）

```go
Actor          string `json:"actor,omitempty"`          // cli / mcp:<client>
SessionID      string `json:"session_id,omitempty"`     // 观测子任务填充
SessionSource  string `json:"session_source,omitempty"` // mcp_connection / env / absent
GuardClass     string `json:"guard_class,omitempty"`
Decision       string `json:"decision,omitempty"`
DecisionReason string `json:"decision_reason,omitempty"`
ConfirmSource  string `json:"confirm_source,omitempty"`
Bypass         bool   `json:"bypass,omitempty"`
Result         string `json:"result,omitempty"`         // executed / denied / failed
```

`SessionID` / `SessionSource` 由观测子任务（`10-30-session-usage-observability`）填充，字段在本子任务一次性加好，避免两次改结构。

### 3.2 为什么不做哈希链

无外部锚点或签名密钥的链，能改日志的人也能重算 —— 对本地攻击者是安全剧场。真实价值（损坏检测）不足以支撑"防篡改审计"的叙事。见 backlog B2，重启时必须同时解决锚点方案。

### 3.3 审计覆盖补全

补 `recordJournal` 调用：`backup.RestoreBackup`、`backup.PruneBackups`、`upgrade` 流程、版本切换。

## 4. MCP 侧加固

| 变更 | 位置 | 说明 |
|---|---|---|
| **移除 `yes`** | `tools.go:125`（schema）、`:384`（读取） | 模型不得自我授权。传入时**忽略，不报错**（避免打断已有客户端配置） |
| `purge` 保留但受 guard 管 | `tools.go:124` | 模型可以**请求** purge，但非交互下由 guard 判 deny。请求 ≠ 授权 |
| 给 `rm` 增 `dry_run` | `tools.go:119-129` | 与 CLI 对齐，给模型一个安全的探查手段 |
| Actor 注入 | `server.go:18` 已存 clientInfo → 下传到 ops | 审计 `actor=mcp:<clientName>` |
| `Interactive=false` 恒定 | mcpserver 构造处 | stdio 被 JSON-RPC 占用，**永不提示** |
| deny 响应结构化 | `tools.go` 错误返回 | 含 `reason` + `safer_alternative` 字段，便于 Agent 自主改用安全替代 |

## 5. P0 修复的具体改法

| 缺陷 | 改法 |
|---|---|
| `rm --dry-run` 真删 | `RmParams` 增 `DryRun`；`Rm()`（`file_ops.go:322`）在 `checkPolicy` + `gate` 通过后、执行前返回预览并 `return`。预览含目标绝对路径、`purge` 与否、预计去向（trash 路径或"永久删除"） |
| `--yes` 空实现 | **删除** `RmParams.Yes`。CLI 的 `--yes` 下线，改为显式 bypass 旗标 —— 语义更诚实（"我知道这不可逆"而非"跳过提示"） |
| MCP 自授权 | 见 §4 |

## 6. 配置（最小面）

```json
{
  "allowed_dirs": ["..."],
  "deny_patterns": ["..."],
  "guard": {
    "enabled": true,
    "allow_bypass": true
  }
}
```

只有两个开关。无 `mode`、无 `classes`、无 `approval_queue` —— 逃生口已经有 `enabled=false` 与 bypass 旗标两个，再加灰度档只增加配置面和测试组合。

## 7. 兼容性与回滚

| 项 | 说明 |
|---|---|
| `guard.enabled=false` | 一键全关，**无需回滚代码** |
| bypass 旗标 | 名称刻意冗长（如 `--i-understand-this-is-destructive`），防止顺手加上 |
| `journal.Entry` | 新增字段全 `omitempty`，老消费方不受影响 |
| 删除 `--yes` | 本就是空实现（零行为变化），CHANGELOG 列 Security Fix |
| **非交互 purge 默认拒绝** | 唯一的 breaking 变更。同版本提供 bypass 旗标 + CHANGELOG 明示 + README 说明 |
| 依赖 | 不新增第三方依赖 |
