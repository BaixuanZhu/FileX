# filex — PRD

> 给任意模型提供一个确定性的文件与环境变量操作层，把 Windows 上容易静默出错的操作用代码锁死，让弱模型也能安全、稳定地完成这类工作。

## 问题定义

当前 agent 已经能通过内置工具 + shell 完成绝大多数文件操作。真正出问题的不是"能不能做"，而是三类不报错的错误：

| 类别 | 现象 | 后果 |
|---|---|---|
| 编码错配 | UTF-16/GBK 文件被当 UTF-8 读 | 模型基于错乱内容"修复"，源文件被永久改坏 |
| 文件占用 | 被进程/杀软持有的文件删除或覆盖失败 | 错误信息含糊，模型原地重试 |
| 环境变量语义 | `setx` 不影响当前进程、超 1024 截断、`REG_EXPAND_SZ` 不展开 | 写入成功但新进程看不到，或变量被截断 |

共同点：命令返回成功，模型无从自检。必须由确定性代码兜住。

## 需求

### 文件操作

- **FR-1** `read` — 读取文件并报告编码（BOM 探测 + 启发式），支持 `--encoding`、`--max-bytes`、`--offset`、`--limit`
- **FR-2** `write` — 原子写入（同目录临时文件 + `MoveFileEx`），支持 `--expect-sha256` 乐观锁、`--backup` 写前快照
- **FR-3** `edit` — 字节级查找替换，保留原编码/BOM/换行，支持 `--dry-run`、`--expect-sha256`、`--backup`
- **FR-4** `grep` — 解码后匹配（非字节匹配），支持 `--stream` 流式模式
- **FR-5** `stat` — 文件元信息（含是否被占用）
- **FR-6** `lock` — 占用诊断（`exclusive` + `deletable` 两个独立维度）
- **FR-7** `ls` — 列目录，支持 `--depth`、`--json`
- **FR-8** `hash` — 校验和（SHA-256）
- **FR-9** `rm` — 删除（默认进回收站），支持 `--purge` 永久删除
- **FR-10** `tail` — 读取文件末尾 N 行（编码感知），支持 `--follow`

### 环境变量

- **FR-11** `env get` — 读取单个变量，区分 `REG_SZ`/`REG_EXPAND_SZ`，后者做展开
- **FR-12** `env set` — 写注册表 + 广播 `WM_SETTINGCHANGE`，支持 `--scope=user|machine`
- **FR-13** `env list` — 列出变量（默认对敏感值脱敏）
- **FR-14** `env unset` — 删除变量

### 备份

- **FR-15** `backup list` — 列出备份（可按路径过滤）
- **FR-16** `backup restore` — 从备份恢复（恢复前自动再备份）
- **FR-17** `backup prune` — 清理超出保留数量的旧备份

### 元命令

- **FR-18** `help --json` — 返回完整命令表
- **FR-19** `version` — 版本信息
- **FR-20** `mcp serve` — 启动 MCP stdio 服务器
- **FR-21** `mcp install --client=...` — 安装 MCP 配置

## 约束

- **C-1** v0.1 只支持 Windows（代码分层保留可移植性，但只测 Windows）
- **C-2** 不做通用 shell 执行器（不提供 `exec` 动词）
- **C-3** 不做网络、包管理、进程管理、跨机同步、GUI、agent 编排
- **C-4** 不替代内置 `Read`/`Write`/`Edit`，只覆盖它们做不好的场景
- **C-5** CLI 与 MCP 共用同一套 `internal/ops`，MCP 层只做参数映射
- **C-6** 安全默认值：MCP 默认只读；删除默认进回收站；越界路径默认拒绝；编辑默认保留原编码
- **C-7** 环境变量测试（写注册表）一律在 GitHub Actions 隔离 VM 中运行，禁止在本机执行

## 验收标准

### 编码矩阵（必须全过）

- 编码：UTF-8 / UTF-8+BOM / UTF-16LE+BOM / UTF-16BE+BOM / GBK / GB18030 / 二进制
- 换行：LF / CRLF / 混合
- 内容：ASCII / 中文 / emoji（代理对）/ 中英混排 / 全角标点
- 文件名：中文 / 空格 / emoji / >260 字符长路径
- 文件状态：被占用 / 只读属性 / 符号链接 / 零字节 / 超大文件
- 验收：`edit` 后除目标区间外字节完全一致（用哈希逐段验证）

### 功能验收

- CLI 与 MCP 共用一张表，两端结果必须逐字段相等
- 原子写：写入中途 kill 进程，原文件哈希不变
- 锁检测：构造"可删但不可独占"的文件，验证 `exclusive=false, deletable=true`
- **环境变量：`env set --scope=user` 后新开 cmd 进程能读到（仅 GitHub Actions CI 验证）**
- 备份策略：`write --backup` 后 `~/.filex/backups/` 存在 `.bak` 文件，其哈希等于写前文件哈希；`backup restore` 后文件哈希恢复；连续 4 次写同一文件，只保留最近 3 个备份
- 大文件：100MB UTF-8 文件 `tail --lines=10` 在 1 秒内返回；`grep --stream` 不加载全文（内存峰值 <50MB）；`read --offset=1000000 --limit=10` 返回正确行号的内容；`edit` 对 >100MB 文件返回 `E_TOO_LARGE`

### 跨模型对照实验（可选，M7）

10 个典型任务 × 3 个档位模型 × 每任务 5 次，对照"仅内置工具"与"内置 + filex"：

- 指标：成功率、危险操作发生率（误删/覆盖/越界写）、平均步数
- 通过标准：弱模型成功率显著提升，危险操作降为 0
