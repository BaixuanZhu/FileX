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

输出（形状取自真实运行；检测是启发式的，GBK 文件可能报成 `gb18030` + `probable`）：
```json
{
  "schema_version": 1,
  "ok": true,
  "data": {
    "content": "...\n",
    "encoding": { "encoding": "gb18030", "bom": false, "confidence": "probable" },
    "size": 5
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
# 删除到恢复目录（可恢复）
filex rm old-file.txt

# 预览永久删除会做什么，不做任何改动
filex rm old-file.txt --purge --dry-run

# 永久删除：在终端输入确认；没有终端（脚本、AI agent）时直接拒绝
filex rm old-file.txt --purge
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

# 不做终端确认直接升级（会写入审计；guard.allow_bypass=false 时被拒绝）
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
| `rm` | 删除（默认移入 `~/.filex/trash`，可恢复） |
| `tail` | 读取末尾 N 行（编码感知） |

### 环境变量

| 命令 | 用途 |
|---|---|
| `env get` | 读取变量值（带展开） |
| `env set` | 设置变量（注册表 + 广播） |
| `env list` | 列出变量（敏感值脱敏） |
| `env unset` | 删除变量 |

### 备份、审计与 MCP

| 命令 | 用途 |
|---|---|
| `backup list` | 列出备份 |
| `backup restore` | 从备份恢复 |
| `backup prune` | 清理旧备份 |
| `journal stats` | 聚合审计日志：按会话/操作/风险类别分布（只读） |
| `journal path` | 打印审计日志位置 |
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

### 恢复目录

- `rm` 默认把文件移入 `~/.filex/trash`（Windows 为 `%USERPROFILE%\.filex\trash`），按
  `原名-时间戳` 重命名后保存，可随时取回
- 这是 filex 自己的目录，**不是 Windows 回收站**：不会出现在回收站界面里，也不会被
  它的清理策略处理
- `rm --purge` 永久删除，需要确认（见下）
- `rm --dry-run` 预览将要做什么，不改动文件系统，也不写审计记录

### 确认通道与 bypass

不可逆操作由 `internal/guard` 分类，共四类：`purge_delete`（永久删除）、
`machine_env_write`（machine 作用域环境变量写入）、`self_config_write`（写
`~/.filex` 自身配置目录）、`self_update`（升级与删除已装版本）。

判定只看两件事：**当前进程有没有可交互终端**，以及**这个类别是否被允许 bypass**。

- 有终端 → 提示 y/N，只有 `y`/`yes` 通过；直接回车算拒绝
- 没有终端（脚本、CI、AI agent）→ 直接拒绝，不提示
- **确认只读终端，绝不读 stdin**：管道喂进来的 `y` 不是人类的同意。进程即使持有
  `CONIN$`，只要 stdin 是被接管的管道就一律判定为"不可交互"，因此 agent 既不能自我
  授权，也不会卡在等待按键上
- MCP 路径永远是"不可交互"：模型反复请求不构成人类批准

绕过确认的唯一方式是显式旗标：

```bash
filex rm big/dir --purge --i-understand-this-is-destructive
```

它被刻意设计得很啰嗦——加上它应当像是做了一个决定，而不是补全了一条命令。审计记录
里留下 `bypass=true` 与 `confirm_source=bypass:flag`，让"被推翻的反对意见"和"被执行
的操作"同时可见。`~/.filex/config.json` 里 `guard.allow_bypass=false` 可以让旗标失效
（此时配置说了算，入口层装错确认器也覆盖不了它）；`guard.enabled=false` 才彻底关掉
整层判定，默认是开启的。

`filex upgrade --yes` 沿用同一语义：它是被审计的 bypass，不再是"跳过那个其实读 stdin
的提示"。旧的 `filex rm --yes` 已废除——它过去什么都不做，现在会明确报用法错误。

被拒绝的操作同样写审计记录（`result=denied`、`decision`、`decision_reason`），否则日
志无法回答"拦下了什么"。

### 会话消耗观测

审计条目可带 `session_id` 与 `session_source`（`mcp_connection` / `env` /
`absent`），用来把"一轮任务里的多次操作"归到一起，再由 `filex journal stats` 聚合成
分布：

```bash
filex journal stats --since=72h --group-by=session
filex journal stats --json
```

必须说清楚的一点：**`session_id` 是观测标签，不是安全凭据。** 它不参与路径策略、不
参与 guard 判定，伪造它的后果只是弄脏伪造者自己的统计。因此：

- 不设置 `FILEX_SESSION_ID` 是正常状态，不触发任何行为变化
- 本功能**不拦截、不降级、不限流**；聚合命令只读，读坏了也影响不到文件操作
- 审计里既没有会话键的旧记录，与新日志中"没有会话键"的记录逐字节一致，因此老消费方
  无需升级

`journal stats` 输出 `session key share`、高危操作的会话内 p50/p99/max、拒绝与 bypass
频次，以及见过的 MCP 宿主列表——这些正是决定"要不要做会话级限额"的数据。结论门槛写
在任务文档里，工具本身只报实测值，不做判定。

### 供应链验证

发布产物带可复现构建、cosign 签名、SLSA provenance 与 SBOM。验证步骤见
[docs/VERIFY.md](docs/VERIFY.md)。可复现性在 CI 里用两次独立构建对比证明；签名与
provenance 只能在真实发布流水线上下文中验收。

### MCP 配置文件

- `ro`（默认）：只读工具（read、grep、stat、lock、ls、tail、env get/list、backup list）
- `rw`：全部工具，包括写操作

`rw` 也不改变上面的判定：`rm --purge`、machine 作用域环境变量写入、升级在 MCP 路径
下依然被拒绝，`rw` 只多提供 `write`、`edit`、`rm`（可恢复的那部分）、`env set/unset`
与 `backup restore/prune` 这些能力。

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

文件与环境变量类命令输出 JSON 信封（`help`、`mcp`、`upgrade`、`journal stats`
输出人类可读文本）：

**成功**：
```json
{
  "schema_version": 1,
  "ok": true,
  "data": { ... }
}
```

**失败**：
```json
{
  "schema_version": 1,
  "ok": false,
  "error": {
    "code": "E_NOT_FOUND",
    "message": "file not found",
    "hint": "check the path and try again",
    "retryable": false
  }
}
```

被熔断闸门拒绝时（`code` 为 `E_DENIED`），`error` 里额外带三个可判别的字段，
供调用方直接分支而不是解析句子：

```json
{
  "code": "E_DENIED",
  "message": "denied by guard (purge_delete)",
  "retryable": false,
  "reason": "permanent delete is unrecoverable",
  "safer_alternative": "drop --purge and the file moves to the trash dir, where it can be recovered",
  "guard_class": "purge_delete"
}
```

退出码：
- `0`：成功
- `1`：内部错误
- `2`：用法错误
- `3`：被熔断闸门拒绝
- `4`：文件被占用
- `5`：未找到
- `6`：冲突（例如 `--expect-sha256` 不匹配）
- `7`：编码错误
- `8`：IO 错误
- `9`：超出大小限制
- `10`：不支持的操作

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
