# filex 设计方案（v0.1 评审稿）

> 目标：给任意模型提供一个**确定性的文件与环境变量操作层**，把 Windows 上容易静默出错的操作用代码锁死，让弱模型也能安全、稳定地完成这类工作。
>
> 状态：评审稿 · 命令名已定为 `filex` · 日期：2026-10-29

---

## 1. 问题定义

当前 agent 已经能通过内置工具 + shell 完成绝大多数文件操作。真正出问题的不是"能不能做"，而是三类**不报错的错误**：

| 类别 | 现象 | 后果 |
|---|---|---|
| 编码错配 | UTF-16/GBK 文件被当 UTF-8 读 | 模型基于错乱内容"修复"，源文件被永久改坏 |
| 文件占用 | 被进程/杀软持有的文件删除或覆盖失败 | 错误信息含糊，模型原地重试 |
| 环境变量语义 | `setx` 不影响当前进程、超 1024 截断、`REG_EXPAND_SZ` 不展开 | 写入成功但新进程看不到，或变量被截断 |

这三类错误的共同点是：**命令返回成功，模型无从自检**。强模型可能靠经验察觉，弱模型不会。因此必须由确定性代码兜住，而不是写进提示词。

---

## 2. 设计原则

1. **机制在代码，流程在技能，判断在模型。**
   安全约束（白名单、原子写、编码保真）写在二进制里，模型绕不过去；Skill 只负责教"什么时候用哪个命令"。

2. **CLI 优先，MCP 是薄壳。**
   CLI 与 MCP 共用同一套 `internal/ops`，MCP 层只做参数映射，不 exec CLI、不解析 stdout。

3. **安全默认值。**
   MCP 默认只读；删除默认进回收站；越界路径默认拒绝；编辑默认保留原编码。

4. **结构化、可恢复的错误。**
   每个错误带稳定错误码、`hint`（下一条该执行的命令）、`retryable`。弱模型的差距主要在出错后的恢复能力，这里投入产出比最高。

5. **靠发现，不靠记忆。**
   `filex help --json` 返回完整命令表；Skill 没加载、换了客户端，模型也能自己发现用法。

---

## 3. 非目标（v0.1 明确不做）

- 不做通用 shell 执行器（不提供 `exec` 动词）
- 不做网络、包管理、进程管理
- 不做跨机同步、不做 GUI
- 不做 agent 编排
- 不在 v0.1 支持 Windows 以外的平台（代码分层保留可移植性，但只测 Windows）
- 不替代内置 `Read`/`Write`/`Edit`，只覆盖它们做不好的场景

---

## 4. 架构

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

**关键约束**：`mcpserver` 不允许 import `cmd/filex`，只能 import `ops`。这条约束保证两个入口不会分叉。

---

## 5. 命令清单（v0.1）

### 5.1 文件

| 命令 | 作用 | 关键参数 |
|---|---|---|
| `filex read` | 读取并**报告编码** | `--encoding=auto\|utf8\|utf8bom\|gbk\|gb18030\|utf16le\|utf16be`、`--max-bytes`、`--offset`、`--limit` |
| `filex write` | 原子写入 | `--encoding`、`--create-parents`、`--expect-sha256`、`--backup` |
| `filex edit` | 字节级查找替换，保留原编码/BOM/换行 | `--find`、`--replace`、`--all`、`--dry-run`、`--expect-sha256`、`--backup` |
| `filex grep` | 解码后匹配（不是字节匹配） | `--paths`、`--encoding=auto`、`--max`、`--stream` |
| `filex stat` | 文件元信息（含是否被占用） | — |
| `filex lock` | 占用诊断 | — |
| `filex ls` | 列目录 | `--depth`、`--json` |
| `filex hash` | 校验和 | `--algo=sha256` |
| `filex rm` | 删除（默认进回收站） | `--dry-run`、`--purge`、`--yes` |
| `filex tail` | 读取文件末尾 N 行（编码感知） | `--lines=10`、`--encoding=auto`、`--follow` |

### 5.2 环境变量

| 命令 | 作用 |
|---|---|
| `filex env get NAME --scope=process\|user\|machine` | 读取单个变量（区分 `REG_SZ` / `REG_EXPAND_SZ`，后者做展开） |
| `filex env set NAME VALUE --scope=user\|machine [--expand]` | 写注册表 + 广播 `WM_SETTINGCHANGE` |
| `filex env list --scope=user` | 列出（默认对敏感值脱敏） |
| `filex env unset NAME --scope=user` | 删除 |

### 5.3 备份

| 命令 | 作用 |
|---|---|
| `filex backup list [--path=<p>]` | 列出备份（可按路径过滤） |
| `filex backup restore <backup-id>` | 从备份恢复（恢复前自动再备份） |
| `filex backup prune` | 清理超出保留数量的旧备份 |

### 5.4 元命令

`filex help --json` · `filex version` · `filex mcp serve` · `filex mcp install --client=...`

---

## 6. 行为规范（评审重点）

### 6.1 编码

**读取时的判定顺序**（必须把结果告诉模型）：

1. 显式 `--encoding`
2. BOM（UTF-8 / UTF-16LE / UTF-16BE / UTF-32）
3. policy 中的默认值
4. 启发式：能严格解码为 UTF-8 → `utf8`；否则尝试 GB18030（GBK 的超集）；都失败 → 判定为二进制并**拒绝**，要求显式指定

输出必须包含：`{"encoding":"utf16le","bom":true,"confidence":"exact|probable"}`。

**写入时的规则**：

- 新建文件：UTF-8 无 BOM、LF（可配置）
- 覆盖已有文件：**默认保持原编码、原 BOM、原换行符**
- 无法确定原编码时：拒绝写入，要求 `--encoding` 显式指定（不允许猜）

**`edit` 的实现方式（关键设计）**：
不采用"解码 → 字符串替换 → 整体重编码"，而是**字节级拼接**：

```
encode(find, detectedEncoding) → 在原始字节中定位 → 用 encode(replace) 替换该区间
```

这样除被替换区间外的所有字节（BOM、CRLF、行尾空格、其他语言的字符）完全不变。这是防止"改一行坏整个文件"最可靠的做法。

**已知局限**：GBK 与 GB18030 在纯 GBK 内容上字节一致，但含扩展汉字时可能不同。v0.1 的处理是：解码按 GB18030，写回按原始标注编码；提供 `--strict-encoding` 让用户强制指定。

### 6.2 原子写

1. 解析为绝对路径 → policy 校验
2. 若给了 `--expect-sha256`，先比对当前文件哈希，不符返回 `E_CONFLICT`
3. 在**同目录**创建临时文件 `.filex-tmp-<random>`
4. 写入 → `Flush` → `Sync`
5. `MoveFileEx(temp, target, REPLACE_EXISTING | WRITE_THROUGH)`
6. 记录 journal

同目录是必须的（跨卷 rename 会退化为拷贝，失去原子性）。目标被占用时 rename 返回共享冲突 → 映射为 `E_LOCKED`，而不是笼统的 IO 错误。

### 6.3 锁检测

`filex lock` 返回两个**独立**维度，不要合并成一个布尔值：

| 字段 | 判定方式 | 含义 |
|---|---|---|
| `exclusive` | `CreateFileW` 以 `GENERIC_READ\|WRITE` + 不共享打开 | 是否能拿到独占句柄（杀软扫描、mmap 都会使其为 false） |
| `deletable` | `CreateFileW` 以 `DELETE` 权限 + `FILE_SHARE_READ\|WRITE\|DELETE` 打开 | 是否可删除。为 true 时即使 `exclusive=false` 也能删 |

现实中最常见的组合是 `exclusive=false, deletable=true` —— 这正是"看起来被占用、其实能删"的情形。只报一个布尔值会误导模型放弃操作。

### 6.4 路径与策略

- 所有输入路径先解析为绝对路径并 `EvalSymlinks`，再做 policy 校验（防止软链接逃逸）
- 长路径：统一走绝对路径；Go 的 `os` 包在 Windows 上会自动加 `\\?\`，但跨库调用仍需自行规范化，并用 >260 字符用例实测
- 默认 deny：`C:\Windows\**`、`C:\Program Files\**` 根目录本身、盘符根、`.git\objects\**`
- `rm` 默认移动到 `~/.filex/trash/<timestamp>/`，manifest 写入 journal；永久删除需 `--purge --yes`
- MCP 侧不信任客户端声明的 cwd，一律以 policy root 为基准解析

### 6.5 环境变量

- 三个作用域独立：`process`（`os.Setenv`）、`user`（`HKCU\Environment`）、`machine`（`HKLM\...\Environment`，需管理员）
- 值中含 `%VAR%` 时必须写 `REG_EXPAND_SZ`，否则不展开
- 写入后广播 `WM_SETTINGCHANGE`（`SendMessageTimeout` + `"Environment"`），否则新开的终端也看不到
- 超过 1024 字符给出警告（`cmd`/`setx` 生态的兼容上限），但不阻止写入
- 敏感变量（名称匹配 `key|token|secret|password|credential`）在 `env list` 默认脱敏，journal 只记录变量名和值的哈希

### 6.6 输出与错误

统一信封：

```json
{"schema_version":1,"ok":true,"data":{...}}
```

```json
{"schema_version":1,"ok":false,"error":{
  "code":"E_LOCKED",
  "message":"target is held by another process",
  "hint":"先执行 filex lock <path> 确认，或改用 filex write --expect-sha256 重试",
  "retryable":false
}}
```

- 结构化结果与错误一律走 **stdout**（模型读 stdout，stderr 常被合并或丢弃）；人类可读提示走 stderr
- 输出编码固定 UTF-8
- 退出码：

| 码 | 含义 |
|---|---|
| 0 | 成功 |
| 1 | `E_INTERNAL` |
| 2 | `E_USAGE` |
| 3 | `E_DENIED`（策略拒绝） |
| 4 | `E_LOCKED` |
| 5 | `E_NOT_FOUND` |
| 6 | `E_CONFLICT`（并发修改 / 匹配不唯一） |
| 7 | `E_ENCODING` |
| 8 | `E_IO` |
| 9 | `E_TOO_LARGE` |
| 10 | `E_UNSUPPORTED` |

### 6.7 备份策略

写前快照，不是事后审计。journal 记录"做了什么"，备份保留"改之前是什么"，两者互补。

**触发条件**：

- `filex write` / `filex edit` 覆盖已有文件时，若 policy 配置 `backup.before_write=true`（默认开启），自动在写入前把原文件复制到 `~/.filex/backups/<relative-path-hash>.bak`
- `--backup` 显式开启（覆盖 policy 默认值）
- `--no-backup` 显式关闭（跳过备份，journal 仍记录 `backup: false`）
- `filex rm` 默认进回收站，不额外备份（回收站本身就是快照）

**备份文件命名**：

```
~/.filex/backups/<sha256-of-absolute-path>-<unix-timestamp>.bak
```

用路径哈希而非路径本身，避免备份目录泄露文件结构。同一路径多次修改会产生多个时间戳不同的 `.bak`。

**保留策略**：

- 默认保留每个文件最近 3 个备份（可配置）
- `filex backup list [--path=<p>]` 列出备份
- `filex backup restore <backup-id>` 从备份恢复（恢复前再做一个备份，形成闭环）
- `filex backup prune` 清理超出保留数量的旧备份

**与 journal 的关系**：

journal 记录 `{"op":"write","path":"...","backup":"<backup-id>","sha256_before":"...","sha256_after":"..."}`，形成"操作 → 快照 → 校验"的完整审计链。恢复时先校验 `sha256_before` 与当前文件是否一致，不一致说明文件又被改过，要求用户确认。

### 6.8 大文件处理

模型上下文有限，filex 不能把 100MB 日志全读进 stdout。大文件场景用"流式 + 摘要"代替全量加载。

**阈值定义**：

- `read` / `edit` / `grep` 对 >10MB 的文件自动切换到流式模式（阈值可配置）
- 流式模式下 `read` 返回 `{"truncated":true,"size":104857600,"read":1048576,"encoding":"utf8","content":"..."}`，明确告知模型"只读了一部分"

**`filex tail`**：

- 从文件末尾向前扫描，按编码感知的换行符切分（不是字节切分）
- `--lines=N` 默认 10，最大 1000
- `--follow` 持续输出新增行（类似 `tail -f`），但通过 context 取消可中断
- 输出带行号（全局行号，不是 tail 内的相对行号），方便模型引用

**`filex grep --stream`**：

- 逐行解码匹配，不加载全文
- `--max=N` 限制返回条数（默认 100），超出时返回 `{"truncated":true,"matched":100,"total_estimated":3472}`
- 对二进制文件（编码探测失败）拒绝流式 grep，要求显式 `--encoding`

**`filex edit` 的大文件限制**：

- `edit` 是全量读写的（字节级拼接需要定位），对 >100MB 的文件返回 `E_TOO_LARGE`
- hint 建议：先用 `grep --stream` 定位行号，再用 `tail --lines=N` 读上下文，确认修改区间后用 `read --offset --limit` 读出片段，最后用 `edit` 写入（edit 本身不受限制，只是读出来给模型看的部分需要截断）

**`filex read --offset --limit`**（补充参数）：

- `--offset=N` 从第 N 行开始读（0-indexed）
- `--limit=N` 读 N 行
- 行号按编码感知的换行符计算
- 输出带全局行号前缀，模型可以直接引用

---

## 7. MCP 层

- stdio transport，Go SDK（官方 `modelcontextprotocol/go-sdk` 或 `mark3labs/mcp-go`，M0 时定）
- **一个动词一个 tool**，不提供 `exec`
- tool 集合（11 个，控制上下文成本）：

```
read  write  edit  grep  stat  lock  ls  rm  tail  backup(action=list|restore|prune)  env(action=get|set|list|unset)
```

- profile：

| profile | 暴露的 tool | 默认 |
|---|---|---|
| `ro` | read grep stat lock ls tail env(get/list) backup(list) | ✅ 默认 |
| `rw` | 全部 | 需显式 `--profile=rw` |

- 客户端在 `initialize` 里传的 `clientInfo` 写入 journal，形成"哪个模型/客户端做了什么"的审计链
- 请求取消通过 `context.Context` 贯穿到 ops，长操作可中断
- 不依赖 MCP 的 `roots` 能力（各客户端支持不一），但若提供则作为白名单**上限**使用

---

## 8. 安全模型

| 威胁 | 对策 |
|---|---|
| 模型误删 | 白名单 root + 默认进回收站 + journal 可回滚 + `--purge` 才永久删 |
| 越界写入 | 绝对路径 + `EvalSymlinks` 后校验 + 默认 deny 系统目录 |
| 并发覆盖（多 agent） | `--expect-sha256` 乐观锁 |
| 密钥泄露 | env 值默认脱敏、journal 只存哈希、`--show-secrets` 显式开启 |
| 任意客户端拉起 MCP | 默认只读 profile；machine 级写操作直接 `E_DENIED` 并提示改用 CLI |
| 提示注入导致的危险操作 | 危险操作要求显式 `--yes`，不能由默认值触发 |
| 写坏文件后无法恢复 | 默认写前备份到 `~/.filex/backups/`，`backup restore` 可回滚；journal 记录 `sha256_before` 用于校验 |
| 备份本身泄露敏感内容 | 备份路径用哈希而非原文；备份文件权限继承系统 ACL；`backup prune` 可彻底清除 |

---

## 9. 测试与验收

### 9.1 编码矩阵（必须全过）

- 编码：UTF-8 / UTF-8+BOM / UTF-16LE+BOM / UTF-16BE+BOM / GBK / GB18030 / 二进制
- 换行：LF / CRLF / 混合
- 内容：ASCII / 中文 / emoji（代理对）/ 中英混排 / 全角标点
- 文件名：中文 / 空格 / emoji / >260 字符长路径
- 文件状态：被占用 / 只读属性 / 符号链接 / 零字节 / 超大文件

验收：`edit` 后除目标区间外**字节完全一致**（用哈希逐段验证）。

### 9.2 一致性与功能

- CLI 与 MCP 共用一张表（操作、参数、期望错误码、期望字段），两端结果必须逐字段相等
- 原子写：写入中途 kill 进程，原文件哈希不变
- 锁检测：构造"可删但不可独占"的文件，验证 `exclusive=false, deletable=true`
- 环境变量：`env set --scope=user` 后，新开 cmd 进程能读到（**仅 GitHub Actions CI 验证，本机禁止运行写注册表测试**）
- 备份策略：`write --backup` 后 `~/.filex/backups/` 存在 `.bak` 文件，其哈希等于写前文件哈希；`backup restore` 后文件哈希恢复；连续 4 次写同一文件，只保留最近 3 个备份
- 大文件：100MB UTF-8 文件 `tail --lines=10` 在 1 秒内返回；`grep --stream` 不加载全文（内存峰值 <50MB）；`read --offset=1000000 --limit=10` 返回正确行号的内容；`edit` 对 >100MB 文件返回 `E_TOO_LARGE`

### 9.3 跨模型对照实验（M7，可选）

10 个典型任务 × 3 个档位模型 × 每任务 5 次，对照"仅内置工具"与"内置 + filex"：

- 指标：成功率、危险操作发生率（误删/覆盖/越界写）、平均步数
- 通过标准：弱模型成功率显著提升，危险操作降为 0

---

## 10. 里程碑

| 阶段 | 内容 | 预估 |
|---|---|---|
| M0 | 仓库骨架、`ops` 接口、错误码表、JSON 信封、`help --json` | 0.5 天 |
| M1 | 编码内核（探测 + 字节级 edit）+ 编码矩阵测试 | 1.5 天 |
| M2 | `stat` / `lock` / `ls` / `hash` / 原子写 / 备份策略（`--backup` + `backup list/restore/prune`） | 1.5 天 |
| M3 | `env` 三作用域、`REG_EXPAND_SZ`、广播、脱敏 | 1 天 |
| M4 | policy + journal + 回收站式 `rm` | 1 天 |
| M5 | `tail` / `read --offset --limit` / `grep --stream` + 大文件测试 | 1 天 |
| M6 | MCP adapter、profile、一致性测试、`mcp install` | 1 天 |
| M7 | 打包、`SKILL.md`、跨模型评测 | 0.5–1 天 |

合计约 7.5–8.5 个工作日。M1 结束时即可日常使用（CLI 部分）。

---

## 11. 与内置工具的分工（写进 SKILL.md）

| 场景 | 用什么 |
|---|---|
| UTF-8 源码的常规读写、精确替换 | 内置 `Read` / `Write` / `Edit`（更快、上下文更省） |
| 编码不确定、需要保真、非 UTF-8 文件 | `filex read` / `filex write` / `filex edit` |
| 删除文件或目录 | `filex rm`（默认回收站） |
| 文件删不掉、怀疑被占用 | `filex lock` |
| 修改环境变量 | `filex env` |
| 大文件（日志、数据导出）末尾查看 | `filex tail` |
| 需要审计/可回滚 | 任何会改磁盘的操作走 `filex` |

Skill 只写触发条件和完整示例命令，不写语法模板（弱模型拼参数容易出错）。

---

## 12. 风险与权衡

| 风险 | 说明 | 应对 |
|---|---|---|
| 编码启发式误判 | 短文件、纯 ASCII 可能被误判 | 报告 `confidence`；写操作不确定就拒绝，不猜 |
| 锁检测语义不完整 | 无法枚举持有者 | 只回答"能否独占/能否删除"，不猜测 |
| MCP 上下文成本 | 11 个 tool 约 1.5–2k token 常驻 | profile 机制；默认只读 6–7 个 |
| 与内置工具重叠 | 模型可能选错 | SKILL.md 明确分工；只在触发器命中时推荐 filex |
| 跨卷 rename | 失去原子性 | 同目录临时文件；跨卷场景直接报 `E_UNSUPPORTED` |
| 权限差异 | MCP 通常非提权 | 明确 `E_DENIED` + 提示改用 CLI |
| 备份磁盘膨胀 | 频繁写入产生大量 `.bak` | 默认保留 3 个 + `backup prune` 定期清理；policy 可关闭备份 |
| 大文件 tail 行号漂移 | 文件边追加边读，行号可能不准 | `tail` 输出快照行号，不保证实时一致；`--follow` 模式明确标注"近似行号" |
| 备份路径哈希冲突 | 不同文件映射到同一哈希 | 用 SHA-256 前 16 字节，碰撞概率 <2^-64；极端场景加路径原文做二次校验 |

---

## 13. 待确认的决策点

| # | 问题 | 建议默认值 |
|---|---|---|
| 1 | 命令名 | ✅ 已定为 `filex`。本机 `fx` 原本无冲突，但 npm 上 `fx` 已被 antonmedv/fx（命令行 JSON 查看器，v40.0.1）占用，且模型会带错误先验，故弃用 |
| 2 | v0.1 只支持 Windows？ | 是 |
| 3 | `rm` 默认进回收站、需 `--purge` 才永久删？ | 是 |
| 4 | MCP 默认 `ro`、`rw` 需显式开启？ | 是 |
| 5 | policy 默认 root 范围 | 当前工作目录 + 显式配置，不默认放开全盘 |
| 6 | 是否需要 `--expect-sha256`（并发覆盖保护） | 需要 |
| 7 | MCP 工具粒度：11 个独立 tool，还是 env/backup 合并为多 action tool | 按上表合并 env 和 backup（省上下文） |
| 8 | 代码放哪 | 新建独立仓库（建议 `D:\code\filex`），不塞进当前 workspace |
| 9 | 备份策略默认开启还是关闭 | 默认开启（`backup.before_write=true`），`--no-backup` 显式关闭 |
| 10 | 备份保留数量 | 每个文件最近 3 个（可配置） |
| 11 | 大文件阈值 | `read`/`edit`/`grep` 对 >10MB 文件自动流式（可配置） |
| 12 | `tail --follow` 是否进 v0.1 | 进，但标记为实验性（M5 交付） |

---

## 14. 评审通过后的第一步

M0 + M1：先交付

- 可运行的 `filex.exe`
- `read` / `write` / `edit` / `stat` / `help --json`
- 12 个编码矩阵用例的自动化测试报告

拿到这两项，就能先替换掉"改非 UTF-8 文件"这个最高频的风险点，再继续往下做。
