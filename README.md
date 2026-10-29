# FileX

[English](README.en.md) | 简体中文

面向 Windows 上 AI agent 的确定性文件与环境变量操作工具。

## 为什么需要 FileX？

AI agent 需要可靠的文件操作，它应当：
- 保持文件编码（GBK、UTF-16、BOM）——而不只是 UTF-8
- 提供审计轨迹与回滚能力
- 处理 Windows 特有问题（注册表、文件锁、长路径）
- 阻止危险操作（系统目录、并发覆盖）

FileX 是一个 CLI 工具，把这些操作封装为确定性的 JSON 输出接口，专为 AI 模型的程序化调用而设计。

## 安装

### Windows 安装器（推荐）

从 [Releases](https://github.com/BaixuanZhu/FileX/releases) 下载 `filex-v<version>-windows-x64.exe`（如 `filex-v0.3.0-windows-x64.exe`）并运行。

安装器会：
- 将 FileX 安装到 `%LOCALAPPDATA%\FileX`
- 将 FileX 加入 PATH
- 创建开始菜单快捷方式

**静默安装：**
```cmd
filex-v0.3.0-windows-x64.exe /S
```

**自定义目录：**
```cmd
filex-v0.3.0-windows-x64.exe /D=D:\Tools\FileX
```

**迁移已有安装：** 显式 `/D=` 会覆盖上一次的安装位置，但旧目录及其 PATH
条目不会被清理——想要干净迁移请先卸载。

### PowerShell（Windows，备选）

```powershell
irm https://raw.githubusercontent.com/BaixuanZhu/FileX/main/install.ps1 | iex
```

脚本会下载最新 release 归档，解压到 `%LOCALAPPDATA%\FileX\versions\v{version}\`，创建 `current` junction，并把包装器加入 PATH。

### Shell（Linux/macOS）

```bash
curl -fsSL https://raw.githubusercontent.com/BaixuanZhu/FileX/main/install.sh | bash
```

脚本会下载最新 release 归档，解压到 `~/.filex/versions/v{version}/`，创建 `current` 符号链接，并把包装器加入 PATH。

### Go install（需要 Go 1.26+）

```bash
go install github.com/BaixuanZhu/FileX/cmd/filex@latest
```

### 预构建归档

从 [Releases](https://github.com/BaixuanZhu/FileX/releases) 下载对应平台的归档：
- `filex-v{version}-windows-x64.zip`（Windows）
- `filex-v{version}-linux-x64.tar.gz`（Linux）
- `filex-v{version}-darwin-x64.tar.gz`（macOS）

解压到 PATH 中的目录即可。每个归档包含二进制、README 和 LICENSE。

### 从源码构建

```bash
# Windows (PowerShell)
$version = git describe --tags --always
$commit = git rev-parse --short HEAD
$buildTime = Get-Date -Format "yyyy-MM-ddTHH:mm:ssZ"
go build -ldflags "-X filex/internal/buildinfo.Version=$version -X filex/internal/buildinfo.Commit=$commit -X filex/internal/buildinfo.BuildTime=$buildTime" -o filex.exe ./cmd/filex

# Windows (Git Bash) / Unix
go build -ldflags "-X filex/internal/buildinfo.Version=$(git describe --tags --always) -X filex/internal/buildinfo.Commit=$(git rev-parse --short HEAD) -X filex/internal/buildinfo.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o filex ./cmd/filex
```

## 快速上手

### 带编码检测地读取文件

```bash
filex read config.ini --encoding=auto
```

输出：
```json
{
  "success": true,
  "data": {
    "path": "config.ini",
    "encoding": "gbk",
    "content": "[section]\nkey=value\n",
    "size": 1234,
    "sha256": "abc123..."
  }
}
```

### 带备份与回滚的编辑

```bash
# 编辑并自动备份
filex edit config.ini --find="old=value" --replace="new=value" --backup

# 列出备份
filex backup list

# 需要时恢复
filex backup restore <backup-id>
```

### 安全删除

```bash
# 删除到回收站（可恢复）
filex rm old-file.txt

# 永久删除（谨慎使用）
filex rm old-file.txt --purge --yes
```

### 环境变量（Windows）

```bash
# 设置用户级环境变量
filex env set NODE_ENV production --scope=user

# 读取（带变量展开）
filex env get PATH --scope=user

# 列出全部（敏感值脱敏）
filex env list --scope=user
```

### 大文件操作

```bash
# 读取最后 50 行
filex tail app.log --lines=50

# 流式 grep（省内存）
filex grep "ERROR" --paths="*.log" --stream --max=100
```

### 自升级与版本管理

FileX 采用 JVM 风格的版本管理：每个版本安装到独立目录，由 `current` 链接指向当前激活版本。

```bash
# 检查是否有更新
filex upgrade --check

# 升级到最新版本（自动清理旧版本）
filex upgrade

# 跳过确认直接升级
filex upgrade --yes

# 列出所有已安装版本
filex upgrade --list

# 切换到指定已安装版本
filex upgrade --use v0.1.3

# 手动清理旧版本（保留最近 3 个）
filex upgrade --cleanup
```

**工作原理：**
- 下载对应平台的归档（Windows 为 ZIP，Unix 为 tar.gz）
- 校验 SHA-256
- 解压到 `~/.filex/versions/v{version}/`（Windows 为 `%LOCALAPPDATA%\FileX\versions\v{version}\`）
- 原子切换 `current` 链接到新版本
- 自动删除旧版本（保留最近 3 个）
- 下次调用即自动使用新版本

**目录结构：**
```
~/.filex/                          # Linux/macOS
%LOCALAPPDATA%\FileX\              # Windows
├── versions/
│   ├── v0.1.2/
│   │   └── filex (或 filex.exe)
│   ├── v0.1.3/
│   │   └── filex (或 filex.exe)
│   └── v0.2.0/
│       └── filex (或 filex.exe)
└── current/                       # 符号链接（Unix）或 junction（Windows）
    └── -> versions/v0.2.0/
```

**迁移：** 如果存在旧的扁平安装，FileX 会在首次运行时自动迁移到版本化结构。

## 命令参考

完整命令参考与示例见 [filex agent 技能](skills/filex/SKILL.md)。

### 文件操作

| 命令 | 用途 |
|---|---|
| `read` | 带编码检测地读取文件 |
| `write` | 原子写文件 |
| `edit` | 按字节查找替换，保持编码不变 |
| `grep` | 编码感知的搜索 |
| `stat` | 文件元数据（大小、修改时间、编码、锁状态） |
| `lock` | 锁诊断（独占 + 可删除） |
| `ls` | 列目录 |
| `hash` | 计算校验和 |
| `rm` | 删除（默认进回收站） |
| `tail` | 读取末尾 N 行（编码感知） |

### 环境变量

| 命令 | 用途 |
|---|---|
| `env get` | 读取变量值（带展开） |
| `env set` | 设置变量（注册表 + 广播） |
| `env list` | 列出变量（敏感值脱敏） |
| `env unset` | 删除变量 |

### 备份与 MCP

| 命令 | 用途 |
|---|---|
| `backup list` | 列出备份 |
| `backup restore` | 从备份恢复 |
| `backup prune` | 清理旧备份 |
| `mcp serve` | 启动 MCP stdio 服务器 |
| `mcp install` | 为 AI 客户端安装 MCP 配置 |

### 升级与版本管理

| 命令 | 用途 |
|---|---|
| `upgrade` | 升级到最新版本（自动清理旧版本） |
| `upgrade --check` | 只检查更新，不安装 |
| `upgrade --list` | 列出所有已安装版本 |
| `upgrade --use <version>` | 切换到指定已安装版本 |
| `upgrade --cleanup` | 手动清理旧版本（保留最近 3 个） |

## 安全模型

### 路径策略

- **允许 CWD**：默认允许当前工作目录
- **拒绝系统目录**：`C:\Windows`、`C:\Program Files`、`.git/objects` 等
- **拒绝优先于允许**：拒绝规则先检查，永远获胜
- **符号链接保护**：`EvalSymlinks` 防止从允许目录逃逸

### 审计日志

所有操作都会记录到 `~/.filex/journal/filex.jsonl`：
- 时间戳、操作、路径
- 写操作记录前后的 SHA-256
- 环境变量名（敏感值做哈希处理）
- 敏感名称：PASSWORD、SECRET、KEY、TOKEN、CREDENTIAL、PRIVATE、SIGNATURE

### 备份策略

- 带 `--backup` 的写操作会在 `~/.filex/backups/` 创建快照
- 备份路径使用原路径的 SHA-256 哈希（保护隐私）
- 默认保留策略：每个文件 3 份备份
- `backup prune` 清理旧备份

### 回收站

- `rm` 默认将文件移入 Windows 回收站（可恢复）
- `rm --purge` 永久删除（谨慎使用）
- `rm --dry-run` 预览将要删除的内容

### MCP 配置文件

- `ro`（默认）：只读工具（read、grep、stat、lock、ls、tail、env get/list、backup list）
- `rw`：全部工具，包括写操作

## MCP 集成

filex 提供 MCP（Model Context Protocol）服务器，供 AI 客户端集成。

### 启动 MCP 服务器

```bash
filex mcp serve --profile=ro
```

### 为 AI 客户端安装

```bash
# Claude Desktop
filex mcp install --client=claude

# Claude Code（CLI，用户级）
filex mcp install --client=claude-code

# Codex CLI
filex mcp install --client=codex

# Cursor
filex mcp install --client=cursor

# VS Code（用户级 mcp.json）
filex mcp install --client=vscode

# Windsurf
filex mcp install --client=windsurf

# ZCode
filex mcp install --client=zcode
```

命令会把 MCP 配置写入对应客户端的配置目录。加上 `--profile=rw`
可注册读写配置，替代默认的只读配置：

```bash
filex mcp install --client=cursor --profile=rw
```

### 可用的 MCP 工具

11 个工具映射到 filex 操作：
- `read`、`write`、`edit`、`grep`、`stat`、`lock`、`ls`、`rm`、`tail`
- `backup`（动作：list、restore、prune）
- `env`（动作：get、set、list、unset）

配置文件约束保证 `ro` 客户端无法执行写操作。

## Agent 技能

通过可分发技能 [skills/filex/SKILL.md](skills/filex/SKILL.md)，教会你的编码
agent 何时该用 filex 而不是内置工具（编码陷阱、可恢复删除、文件被锁、环境
变量作用域）：

```bash
# skills.sh 安装器——自动识别 70+ 种 agent 并选择正确的目录
npx skills add BaixuanZhu/FileX
```

也可以手动把 `skills/filex/` 目录复制到你的 agent 技能目录
（`~/.agents/skills/`、`~/.zcode/skills/` 或 `~/.claude/skills/`）。

## 输出格式

所有命令输出 JSON 信封：

**成功**：
```json
{
  "success": true,
  "data": { ... }
}
```

**失败**：
```json
{
  "success": false,
  "error": {
    "code": "E_NOT_FOUND",
    "message": "file not found",
    "hint": "check the path and try again",
    "retryable": false
  }
}
```

退出码：
- `0`：成功
- `1`：操作错误
- `2`：用法错误

## 开发

### 构建

```bash
go build ./cmd/filex
```

### 测试

```bash
# 单元测试（本地运行安全）
go test ./...

# 集成测试（环境变量注册表操作）
# 警告：会修改 Windows 注册表，仅在 CI 或虚拟机中运行
go test -tags=integration ./internal/winenv/...
```

### 项目结构

```
filex/
├── cmd/filex/           # CLI 入口
├── internal/
│   ├── ops/             # 操作实现
│   ├── encoding/        # BOM 检测、GBK、UTF-16
│   ├── policy/          # 路径白名单、拒绝规则
│   ├── journal/         # JSONL 审计日志
│   ├── errs/            # 错误码 + 提示
│   ├── mcpserver/       # MCP 工具 schema
│   ├── version/         # JVM 风格版本管理
│   ├── winenv/          # Windows 注册表、广播
│   ├── winfs/           # Windows 文件系统操作
│   └── backup/          # 备份存储、恢复
├── scripts/
│   ├── filex.bat        # Windows 包装脚本
│   └── filex.sh         # Unix 包装脚本
├── skills/
│   └── filex/
│       └── SKILL.md     # 可分发的 agent 技能
├── install.ps1          # Windows 安装脚本
└── install.sh           # Unix 安装脚本
```

## 许可证

MIT

## 参与贡献

本项目遵循 Trellis 开发工作流，详见 `.trellis/workflow.md`。
