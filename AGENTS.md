# 项目上下文与开发约定

## 目标与范围

Clash 流量观察是 Windows 本地流量统计工具，为 Clash Verge Rev 内置的 Mihomo 提供按应用、域名和时间的历史流量统计。用户使用系统代理、未开启 TUN；统计范围是经过 Mihomo 的连接，不是全机网卡流量。经过核心但最终走 DIRECT 的连接也可能被记录；当前没有仅保留代理节点流量的过滤。

保持独立伴生程序、SQLite 本地存储、单个 Windows EXE 交付。只采集连接元数据和字节计数，不解密 HTTPS。用户界面及日常沟通使用中文。

## 技术栈与代码导航

- 后端：Go；模块要求及依赖版本以 `go.mod` 为准（当前 Go 1.26）。SQLite 使用 `modernc.org/sqlite`，不需要 C 编译器。
- 前端：Vue 3、TypeScript、Vite、Apache ECharts；使用 npm 和已提交的 `frontend/package-lock.json`。
- Windows 托盘：`github.com/gogpu/systray`；Windows 注册表：`golang.org/x/sys/windows/registry`。当前应用入口依赖 Windows 平台实现，不应宣称跨平台可运行。
- `cmd/clash-traffic-monitor/main.go`：配置、日志、采集、HTTP 服务、托盘生命周期及关闭流程。
- `internal/mihomo/`：连接模型及 `/connections?interval=...` WebSocket 客户端；Bearer 密钥认证。
- `internal/collector/`：快照差分、自动重连、进程/域名归属、暂停状态。
- `internal/store/`：SQLite 建表、写入、清理及统计查询；修改数据模型时从这里查兼容性。
- `internal/api/`：历史统计和状态 API；当前接口为 `/api/status`、`/api/summary`、`/api/series`、`/api/top/apps`、`/api/top/domains`。
- `internal/config/`、`internal/logging/`：配置默认值、数据路径及日志。
- `internal/tray/`、`internal/platform/`：托盘菜单、图标、打开浏览器/文件夹及当前用户登录自启动。
- `frontend/src/`：面板与样式；查询 API 每 5 秒刷新，尚无后端向前端推送通道。
- `internal/webui/dist/`：已提交的前端构建资源，通过 `go:embed` 嵌入 EXE。不要手改哈希资源，修改前端源码后重新构建。
- 使用说明查 `README.md`；发布构建查 `scripts/build.ps1`。按任务读取相关文件，不要求每次遍历整个仓库。

## 本地开发与构建

从仓库根目录执行；Windows PowerShell 为主要环境。

```powershell
# 首次安装或前端依赖变更
cd frontend
npm ci
npm run build
cd ..

# 普通控制台开发（无托盘）
go run ./cmd/clash-traffic-monitor -config config.json -no-tray

# 后端验证
go test ./...
go vet ./...

# 正式构建：安装前端依赖、构建前端、测试 Go、生成无黑窗口 EXE
.\scripts\build.ps1
```

- 前端热更新：在 `frontend/` 执行 `npm run dev`，Vite 将 `/api` 代理至 `http://127.0.0.1:17890`。后端需另行运行。
- 前端类型检查：在 `frontend/` 执行 `npm run typecheck`。生产构建也包含类型检查。
- 必须先完成前端构建，再运行需要嵌入资源的 Go 测试/构建；不要并行执行这两步。Vite 清理旧哈希资源时会与 Go 的嵌入扫描发生竞态。
- 正式 EXE 构建参数为 `-trimpath -ldflags="-H=windowsgui -s -w"`。仅改 Go 时可以沿用已构建的前端资源。
- 后端改动按风险运行相关测试和静态检查；前端改动运行类型检查/生产构建。纯文档改动检查内容与 `git diff --check` 即可。
- 现有 Go 测试使用临时数据库，不连接真实 Mihomo，不修改开机启动注册表。
- 已运行的 EXE 可能无法直接覆盖。构建测试版可使用另一个文件名；不要未经授权停止用户正在运行的采集器。

## 配置与运行数据

- `config.example.json` 是模板，`config.json` 是被 Git 忽略的本地配置，含 Mihomo 密钥；不要输出或提交密钥。
- 默认面板地址 `127.0.0.1:17890`，默认 Mihomo 地址 `http://127.0.0.1:9097`。实际状态需现场验证，不要将某次端口故障写成长期事实。
- Clash Verge Rev 需开启外部控制器并设置密钥。应用识别依赖 `find-process-mode: always`，可通过当前订阅或全局扩展配置设置；不要直接改会被客户端重建的运行配置文件。
- 默认采样间隔 1000 ms，配置下限 250 ms；明细保留 7 天。其他配置字段见 `internal/config/config.go`。
- 默认数据库：`%LOCALAPPDATA%\ClashTrafficMonitor\traffic.db`；默认日志：`%LOCALAPPDATA%\ClashTrafficMonitor\logs\traffic-monitor.log`。均可通过配置覆盖。
- `-config` 默认相对当前工作目录解析。自动启动使用 EXE 与配置文件绝对路径；移动程序后需重新设置自启动。
- 托盘左键打开面板，菜单支持打开面板、打开日志文件夹、暂停/恢复、开机启动及退出。自启动存于 `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` 的 `ClashTrafficMonitor` 值，不需要管理员权限；实现功能不等于授权替用户开启它。
- 日志先写文件再写 stderr，避免无控制台构建的 stderr 无效导致丢失文件日志。当前仅在启动时检查超过 5 MiB 的日志并保留 `.1` 备份，不是运行中持续轮转。

## 统计语义与实现边界

- 上传/下载单位是字节；每条连接按 ID 对相邻累计计数做差，不能直接把每次快照的累计值相加。
- `traffic_minute` 按分钟、应用、域名聚合；`connection_events` 按连接 ID、分钟合并明细。清理只删除过期明细，分钟聚合长期保留；尚无日聚合表。
- `connections` 计数是采集器首次观察连接时累计的次数，不是每秒采样数，也不是当前活跃连接数。
- 暂停时仍接收快照、更新差分基线，但跳过入库；保留该设计以避免恢复时补记正常采样覆盖的暂停流量。断线跨越暂停区间等边界需单独验证。
- 新进程启动时缺少既有连接的基线，首个快照会把既有累计流量计入当前分钟。短连接可能在采样间隔内结束而漏记，连接消失后的尾部增量也可能漏记。统计不能当作精确计费数据。
- 应用名称来自进程名/路径，缺失时显示“未知应用”；域名优先使用 host，其次 sniffHost，再回退目标 IP。只能归到进程，无法据此断定 VS Code 的具体扩展、浏览器标签页或 HTTPS 数据内容。
- 首次采样、重连、计数重置、暂停恢复、连接消失和核心重启是改动采集算法时应关注的边界。
- 修改 SQLite 表结构需兼容现有数据库：`CREATE TABLE IF NOT EXISTS` 不会为旧表补上字段或唯一约束。不要通过删除真实数据库解决迁移问题。
- 当前没有单实例保护、任意时间范围筛选、应用与域名钻取页面或大流量告警。它们属于后续功能，不要把早期方案描述当成已实现功能。
- 查询真实历史记录使用只读 SQLite 连接；查询结果、进程列表及当时的遥测排行是临时诊断信息，不写入本文件。

## 协作与 Git

- 讨论方案或诊断问题时先解释结论；用户明确要求实现后再改文件。
- 先检查 Git 状态并保留用户改动。按已建立的维护习惯，将完成并验证的项目改动提交为独立、可审查的本地 commit；不要自动 push。
- 使用 `feat:`、`fix:`、`docs:` 等简短提交标题。只暂存本次相关文件；不要提交配置密钥、真实数据库、日志、依赖缓存或 EXE。
- 搜索使用范围明确的 `rg` / `rg --files`；HTTP 请求优先 `curl.exe`。需要 Python 时用 `uv run`，不直接调用 `python`；第三方依赖脚本声明 inline metadata。
- 通用工具缓存保留用户级路径；权限不足申请相应目录权限，不迁入仓库规避限制。
- 更新本文件时保留稳定决策、可执行命令和已验证的项目事实；行为变化时同步修订。避免保存回合流水账、临时 PID、流量数字或重复全局指令。

编写依据：[OpenAI AGENTS.md 指南](https://learn.chatgpt.com/docs/agent-configuration/agents-md)及[保持指令简洁、按需读取的官方建议](https://developers.openai.com/blog/rethinking-skills-and-prompts-for-gpt-6-astra)。
