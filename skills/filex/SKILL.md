---
name: filex
slug: filex
version: 0.4.0
displayName: FileX 编码感知的 Windows 文件操作
summary: Windows 编码感知文件操作 CLI：GBK/GB18030/UTF-16/BOM 自动检测保持、可恢复删除、文件锁诊断、环境变量持久化、备份与一键恢复，含二进制与 MCP 安装指引。
tags: [windows, encoding, gbk, file-operations, mcp]
license: MIT
homepage: https://github.com/BaixuanZhu/FileX
description: 面向 Windows 的编码感知、可恢复文件操作 CLI 工具。read/write/edit/grep/tail 自动检测并保持 GBK、GB18030、UTF-16、BOM 编码，非 UTF-8 文件不会被写成乱码；rm 默认把文件移到 ~/.filex/trash 随时可找回（不是 Windows 回收站），并带审计日志、备份与一键恢复；lock 命令诊断文件被哪个进程占用（文件正在使用 / file in use / 删不掉）；env 命令持久修改用户级/系统级环境变量并广播生效。凡遇到文件可能不是 UTF-8、删除需要可撤销、rm 因文件被占用而失败、需要修改 PATH 或注册表环境变量的场景，都应使用本技能——即使用户没有提到编码问题。纯 UTF-8 文件的简单读写请继续使用内置 Read/Edit 工具。
---

# FileX — 面向 AI Agent 的确定性文件操作

`filex` 是一个单一静态二进制，为文件 I/O 封装了编码检测、审计日志与备份。
文件与环境变量类命令输出 JSON；破坏性行为（绕过恢复目录、无备份覆盖）必须
通过显式旗标开启，绝不默认发生。

高危操作（永久删除、机器级环境变量、写入 filex 自己的配置目录、自我升级）
会经过一道确认闸门：有终端时向人询问 y/N，没有终端时直接 deny。拿到 deny
**不要重试，也不要加旗标绕过**——把决定交回给人类。

## 第 0 步 — 安装二进制

本技能依赖 PATH 上的 `filex` CLI。如果 `filex version` 失败，先安装：

```powershell
# Windows (PowerShell)
irm https://raw.githubusercontent.com/BaixuanZhu/FileX/main/install.ps1 | iex
```

```bash
# Linux / macOS
curl -fsSL https://raw.githubusercontent.com/BaixuanZhu/FileX/main/install.sh | bash
```

升级自管理：`filex upgrade --check` 只看有没有新版本，不动任何文件。真正的
替换动作属于高危操作：`filex upgrade --yes` 不再是"静默跳过提示"，而是一次
**会被记入审计日志的 bypass**；人类可以把 `guard.allow_bypass` 设成 `false`
彻底拒绝。默认请优先 `--check`，把真正的升级交回给人类。其他安装方式
（安装器 EXE、预构建归档、`go install`）见
[FileX README](https://github.com/BaixuanZhu/FileX#安装)。

## 可选 — 注册 MCP 服务器

想把 filex 以 MCP 工具形式接入 AI 客户端而非 shell 命令时：

```bash
filex mcp install --client=<claude|claude-code|codex|cursor|vscode|windsurf|zcode> [--profile=rw]
```

默认 `ro` 配置只暴露只读工具与只读动作（`env get|list`、`backup list`）；`rw`
增加 `write` / `edit` / `rm` 以及 `env set|unset`、`backup restore|prune`。
profile 只决定"能不能调到这个工具"，高危动作仍要过确认闸门。

## 何时使用 FileX

| 场景 | 命令 | 原因 |
|---|---|---|
| 编码不确定（GBK、UTF-16、BOM） | `filex read/write/edit` | 内置工具假定 UTF-8 |
| 删除文件/目录 | `filex rm` | 默认移到 `~/.filex/trash`，可恢复 |
| 文件删不掉（被锁？） | `filex lock` | 报告独占/可删除状态 |
| 修改环境变量 | `filex env` | 注册表 + 广播 + 3 种作用域 |
| 大文件 tail/grep | `filex tail` / `filex grep --stream` | 省内存、编码感知 |
| 需要审计/回滚 | 任意操作加 `--backup` | 日志 + 备份在 `~/.filex/` |
| 想知道最近谁在做什么危险操作 | `filex journal stats` | 只读聚合审计日志，不改任何东西 |
| UTF-8、简单读写 | 内置 `Read`/`Edit` | 更快、占用上下文更少 |

## 命令

**文件**：`read` `write` `edit` `grep` `stat` `lock` `ls` `hash` `rm` `tail`
**环境变量**：`env get|set|list|unset`（作用域：process|user|machine）
**备份**：`backup list|restore|prune`
**审计**：`journal stats [--since=48h] [--group-by=session|verb|class|source] [--json]`、`journal path`
**MCP**：`mcp serve [--profile=ro|rw]` `mcp install --client=<client> [--profile=ro|rw]`
**升级**：`upgrade [--check] [--yes]`

## 关键旗标

- `--encoding=auto|utf8|utf8bom|gbk|gb18030|utf16le|utf16be` — 强制指定或自动检测编码
- `--backup` — 写/编辑前先做快照
- `--expect-sha256=<hash>` — 乐观锁，防并发覆盖
- `--stream` — 流式 grep 大文件，不整载内存
- `--dry-run` — 只报告会发生什么，不改动任何文件
- `--purge` — 永久删除（跳过恢复目录），属于高危操作
- `--scope=user|machine|process` — 环境变量作用域（Windows）
- `--i-understand-this-is-destructive` — 显式 bypass 确认闸门，会在审计日志里
  标记为 bypass；`guard.allow_bypass=false` 时直接被拒绝

## 示例

```bash
# 编辑 GBK 编码的配置文件，并保留回滚快照
filex edit config.ini --find="old" --replace="new" --encoding=gbk --backup

# 安全删除，需要时可撤销（先预览，再动手）
filex rm old.txt --dry-run
filex rm old.txt
filex backup list
filex backup restore <backup-id>

# 不加载整个大文件直接看末尾
filex tail app.log --lines=50

# 设置用户级环境变量（持久化并广播生效）
filex env set NODE_ENV production --scope=user

# 只检查有没有新版本，不替换二进制
filex upgrade --check
```

## 拿到 deny 怎么办

`filex rm --purge`、`filex env set --scope=machine`、写入 filex 自己的配置目录
（`~/.filex/`）等操作会触发确认闸门。没有终端（agent 驱动、CI、MCP）时它们
直接返回 `E_DENIED`（退出码 3），错误里带上 `guard_class` 和更安全的替代方案。

- 不要重试同一条命令，也不要加上 bypass 旗标硬闯——这是设计出来让人类拍板的。
- 有不需要永久化的做法就先换过去：去掉 `--purge` 走恢复目录、
  `--scope=machine` 降到 `--scope=user`。
- 确实需要绕过时，把命令和 `guard_class` 原样报给用户，由用户在终端里确认。

## 会话标识（可选）

同一个任务里跨多次进程调用时，设 `FILEX_SESSION_ID` 把它们归到一条会话上，
`filex journal stats --group-by=session` 就能按会话统计高危次数：

```bash
export FILEX_SESSION_ID=refactor-login-2026-10-30   # 只允许字母数字和 . _ -，≤64 字符
filex rm cache.tmp --backup
filex journal stats --session=refactor-login-2026-10-30
```

非法值（含空格、换行、引号或超长）会被丢弃，日志里退化成"无会话"记录——
宁可看不出归属，也不能让一个环境变量注入出第二条日志。

## 安全模型

- **路径策略**：允许 CWD；拒绝系统目录（Windows、Program Files、
  `.git/objects`）；拒绝规则永远优先于允许规则
- **确认闸门**：高危操作默认要求终端确认，无终端时拒绝；bypass 旗标会被
  审计，并可被 `guard.allow_bypass=false` 禁用
- **审计日志**：每个操作追加到 `~/.filex/journal/filex.jsonl`，记录前后的
  SHA-256、`guard_class` 与决策结果，敏感值脱敏
- **删除**默认移到 `~/.filex/trash`（不是 Windows 回收站），除非显式传
  `--purge`

## 输出格式

文件与环境变量类命令输出 JSON 信封：
`{"schema_version":1,"ok":true,"data":{...}}` 或
`{"schema_version":1,"ok":false,"error":{"code":"...","message":"...","hint":"...","retryable":false}}`。
被闸门拒绝时 `error` 还带 `guard_class` / `reason` / `safer_alternative` 三个可直接分支的字段。
`help`、`mcp`、`upgrade`、`journal stats` 是人类可读文本，`journal stats --json`
切换成 JSON。
退出码：0 = 成功，1 = 内部错误，2 = 用法错误，3 = 被确认闸门拒绝，
4 = 文件被占用，5 = 未找到，6 = 冲突，7 = 编码，8 = IO，9 = 过大，10 = 不支持。

---

## 安装本技能（写给人类读者）

```bash
# skills.sh 安装器——自动识别 70+ 种 agent 并选择正确的目录
npx skills add BaixuanZhu/FileX
```

或者从 [FileX 仓库](https://github.com/BaixuanZhu/FileX) 把 `skills/filex/`
目录复制到你的 agent 技能目录：`~/.agents/skills/`（跨 agent 通用）、
`~/.zcode/skills/`（ZCode）或 `~/.claude/skills/`（Claude Code）。
