---
name: filex
slug: filex
version: 0.3.0
displayName: FileX 编码感知的 Windows 文件操作
summary: Windows 编码感知文件操作 CLI：GBK/GB18030/UTF-16/BOM 自动检测保持、回收站删除、文件锁诊断、环境变量持久化、备份与一键恢复，含二进制与 MCP 安装指引。
tags: [windows, encoding, gbk, file-operations, mcp]
license: MIT
homepage: https://github.com/BaixuanZhu/FileX
description: 面向 Windows 的编码感知、可恢复文件操作 CLI 工具。read/write/edit/grep/tail 自动检测并保持 GBK、GB18030、UTF-16、BOM 编码，非 UTF-8 文件不会被写成乱码；rm 默认进回收站随时可找回，每次修改都有审计日志、备份与一键恢复；lock 命令诊断文件被哪个进程占用（文件正在使用 / file in use / 删不掉）；env 命令持久修改用户级/系统级环境变量并广播生效。凡遇到文件可能不是 UTF-8、删除需要可撤销、rm 因文件被占用而失败、需要修改 PATH 或注册表环境变量的场景，都应使用本技能——即使用户没有提到编码问题。纯 UTF-8 文件的简单读写请继续使用内置 Read/Edit 工具。
---

# FileX — 面向 AI Agent 的确定性文件操作

`filex` 是一个单一静态二进制，为文件 I/O 封装了编码检测、审计日志与备份。
所有命令输出 JSON；破坏性行为（绕过回收站、无备份覆盖）必须通过显式
旗标开启，绝不默认发生。

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

升级自管理：`filex upgrade --yes`（保留最近 3 个版本）。其他安装方式
（安装器 EXE、预构建归档、`go install`）见
[FileX README](https://github.com/BaixuanZhu/FileX#安装)。

## 可选 — 注册 MCP 服务器

想把 filex 以 MCP 工具形式接入 AI 客户端而非 shell 命令时：

```bash
filex mcp install --client=<claude|claude-code|codex|cursor|vscode|windsurf|zcode> [--profile=rw]
```

默认 `ro` 配置只暴露只读工具；`rw` 增加 write/edit/rm。

## 何时使用 FileX

| 场景 | 命令 | 原因 |
|---|---|---|
| 编码不确定（GBK、UTF-16、BOM） | `filex read/write/edit` | 内置工具假定 UTF-8 |
| 删除文件/目录 | `filex rm` | 默认进回收站（可恢复） |
| 文件删不掉（被锁？） | `filex lock` | 报告独占/可删除状态 |
| 修改环境变量 | `filex env` | 注册表 + 广播 + 3 种作用域 |
| 大文件 tail/grep | `filex tail` / `filex grep --stream` | 省内存、编码感知 |
| 需要审计/回滚 | 任意操作加 `--backup` | 日志 + 备份在 `~/.filex/` |
| UTF-8、简单读写 | 内置 `Read`/`Edit` | 更快、占用上下文更少 |

## 命令

**文件**：`read` `write` `edit` `grep` `stat` `lock` `ls` `hash` `rm` `tail`
**环境变量**：`env get|set|list|unset`（作用域：process|user|machine）
**备份**：`backup list|restore|prune`
**MCP**：`mcp serve [--profile=ro|rw]` `mcp install --client=<client> [--profile=ro|rw]`
**升级**：`upgrade [--check] [--yes]`

## 关键旗标

- `--encoding=auto|utf8|utf8bom|gbk|gb18030|utf16le|utf16be` — 强制指定或自动检测编码
- `--backup` — 写/编辑前先做快照
- `--expect-sha256=<hash>` — 乐观锁，防并发覆盖
- `--stream` — 流式 grep 大文件，不整载内存
- `--purge` — 永久删除（跳过回收站）
- `--scope=user|machine|process` — 环境变量作用域（Windows）

## 示例

```bash
# 编辑 GBK 编码的配置文件，并保留回滚快照
filex edit config.ini --find="old" --replace="new" --encoding=gbk --backup

# 安全删除，需要时可撤销
filex rm old.txt --yes
filex backup list
filex backup restore <backup-id>

# 不加载整个大文件直接看末尾
filex tail app.log --lines=50

# 设置用户级环境变量（持久化并广播生效）
filex env set NODE_ENV production --scope=user

# 升级到最新版本
filex upgrade --yes
```

## 安全模型

- **路径策略**：允许 CWD；拒绝系统目录（Windows、Program Files、
  `.git/objects`）；拒绝规则永远优先于允许规则
- **审计日志**：每个操作追加到 `~/.filex/journal/filex.jsonl`，记录前后的
  SHA-256，敏感值脱敏
- **删除**默认进回收站，除非显式传 `--purge`

## 输出格式

所有输出都是 JSON：`{"success": true, "data": {...}}` 或
`{"success": false, "error": {"code": "...", "message": "...", "hint": "..."}}`。
退出码：0 = 成功，1 = 操作错误，2 = 用法错误。

---

## 安装本技能（写给人类读者）

```bash
# skills.sh 安装器——自动识别 70+ 种 agent 并选择正确的目录
npx skills add BaixuanZhu/FileX
```

或者从 [FileX 仓库](https://github.com/BaixuanZhu/FileX) 把 `skills/filex/`
目录复制到你的 agent 技能目录：`~/.agents/skills/`（跨 agent 通用）、
`~/.zcode/skills/`（ZCode）或 `~/.claude/skills/`（Claude Code）。
