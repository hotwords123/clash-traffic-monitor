# 项目上下文与开发约定

## 目标与范围

Clash 流量观察是 Windows 本地流量统计工具，为 Clash Verge Rev 内置的 Mihomo 提供按应用、域名和时间的历史流量统计。不要求开启 TUN；统计范围是经过 Mihomo 的连接，不是全机网卡流量。经过核心但最终走 DIRECT 的连接也可能被记录；当前没有仅保留代理节点流量的过滤。

保持独立伴生程序、SQLite 本地存储、单个 Windows EXE 交付。只采集连接元数据和字节计数，不解密 HTTPS。用户界面使用中文。

## 提交约定

- 完成阶段性任务并通过相应验证后，直接提交该阶段相关更改，无需再次确认。提交信息遵循 Conventional Commits，使用 `feat:`、`fix:`、`docs:`、`chore:` 等合适类型，可按需添加 scope。
- 提交前检查 diff 和 Git 状态，只纳入当前任务相关文件；不提交本地配置、密钥、运行数据或无关更改。此授权仅包含本地提交，推送或发布仍按用户指令执行。用户明确要求暂不提交时遵从该要求。

## 技术栈与代码导航

- 后端：Go；模块要求及依赖版本以 `go.mod` 为准（当前 Go 1.26）。SQLite 使用 `modernc.org/sqlite`，不需要 C 编译器。
- 前端：Vue 3、TypeScript、Vite、Apache ECharts；使用 npm 和已提交的 `frontend/package-lock.json`。
- Windows 托盘：`github.com/gogpu/systray`；Windows 注册表：`golang.org/x/sys/windows/registry`。当前应用入口依赖 Windows 平台实现，不应宣称跨平台可运行。
- `cmd/clash-traffic-monitor/main.go`：配置、日志、采集、HTTP 服务、托盘生命周期及关闭流程。
- `internal/mihomo/`：连接模型及 `/connections?interval=...` WebSocket 客户端；Bearer 密钥认证。
- `internal/collector/`：快照差分、自动重连、进程/域名归属、暂停状态。
- `internal/store/`：SQLite 建表、写入、清理及统计查询；修改数据模型时从这里查兼容性。
- `internal/api/`：历史统计和状态 API；当前接口为 `/api/status`、`/api/summary`、`/api/series`、`/api/top/apps`、`/api/top/domains`、`/api/connections`、`/api/proxies`；历史接口支持按分钟对齐的 `from` / `to`、进程与域名组合筛选；重复 `app_selected` / `domain_selected` 参数指定精确多选（优先于搜索）；重复 `proxy_selected` 按代理链最外层组 / 出口精确多选，空值选择未记录；`/api/proxies` 返回当前时间及应用、域名条件下各出口的流量与首次观察连接数，忽略出口自身选择，排行带 `page` / `page_size` 返回分页对象；`/api/series` 的 `bucket_minutes` 支持预设聚合粒度。
- `internal/config/`、`internal/logging/`：配置默认值、数据路径及日志。
- `internal/tray/`、`internal/platform/`：托盘菜单、图标、打开浏览器/文件夹及当前用户登录自启动。
- `frontend/src/`：面板与样式；统计页每 5 秒刷新，连接查询页在查询、翻页或手动刷新时更新；尚无后端向前端推送通道。
- `internal/webui/dist/`：本地生成且不提交的前端构建资源，通过 `go:embed` 嵌入 EXE。不要手改哈希资源，修改前端源码后重新构建；新检出仓库必须先构建前端，才能运行 Go 测试或编译应用。
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

# 正式构建：安装依赖、测试并构建前端、测试与 vet Go、生成无黑窗口 EXE
.\scripts\build.ps1
```

- 前端热更新：在 `frontend/` 执行 `npm run dev`，Vite 将 `/api` 代理至 `http://127.0.0.1:17890`。后端需另行运行。
- 前端类型检查：在 `frontend/` 执行 `npm run typecheck`。生产构建也包含类型检查。
- 必须先完成前端构建，再运行需要嵌入资源的 Go 测试/构建；不要并行执行这两步。Vite 清理旧哈希资源时会与 Go 的嵌入扫描发生竞态。
- 正式 EXE 构建参数为 `-trimpath -ldflags="-H=windowsgui -s -w"`。仅改 Go 时可以沿用本地已构建的前端资源；前端资源、EXE、日志和数据库均不提交，依赖锁文件 `frontend/package-lock.json` 和 `go.sum` 保持提交。
- 后端改动按风险运行相关测试和静态检查；前端改动运行类型检查/生产构建。纯文档改动检查内容与 `git diff --check` 即可。
- 现有 Go 测试使用临时数据库，不连接真实 Mihomo，不修改开机启动注册表。
- Windows 无法直接覆盖正在运行的 EXE，构建测试版可使用另一个文件名。
- 用户已授权本地更新流程：先完成测试并成功构建到临时 EXE，再关闭本项目目录中旧 EXE 对应的运行进程（如果有），替换正式 `clash-traffic-monitor.exe`，并启动新版，无需再次确认。关闭前核对进程的可执行文件绝对路径，避免影响其他目录的实例；优先正常退出，必要时才终止进程，并等待进程退出后替换。构建失败时保留旧进程和正式 EXE；替换前保留旧 EXE 备份，更新或启动失败时恢复旧版。启动新版使用项目目录作为工作目录，并通过 `-config` 显式指定本地配置文件的绝对路径，带 `-background` 避免自动重启弹出浏览器。用户明确表示已手动更新或不需重启时，不重复执行更新。
- `scripts/build.ps1 -OutputPath .\clash-traffic-monitor-test.exe` 可指定输出文件；每个外部命令均检查退出码，失败立即停止。GitHub Actions 在 Windows 上调用同一脚本并上传 x64 构建产物。

## 配置与运行数据

- `config.example.json` 是模板，`config.json` 是被 Git 忽略的本地配置，含 Mihomo 密钥；不要输出或提交密钥。
- 面板和 Mihomo 控制器地址通过配置指定；默认值见 `internal/config/config.go`，不要假设开发机器的控制器端口。
- Clash Verge Rev 需开启外部控制器并设置密钥。应用识别依赖 `find-process-mode: always`，可通过当前订阅或全局扩展配置设置；不要直接改会被客户端重建的运行配置文件。
- 默认采样间隔 1000 ms，配置下限 250 ms；明细保留 7 天。其他配置字段见 `internal/config/config.go`。
- 默认数据库：`%LOCALAPPDATA%\ClashTrafficMonitor\traffic.db`；默认日志：`%LOCALAPPDATA%\ClashTrafficMonitor\logs\traffic-monitor.log`。均可通过配置覆盖。
- `-config` 默认相对当前工作目录解析。自动启动使用 EXE 与配置文件绝对路径；移动程序后需重新设置自启动。
- 托盘左键打开面板，菜单支持打开面板、打开日志文件夹、暂停/恢复、开机启动及退出。自启动存于 `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` 的 `ClashTrafficMonitor` 值，不需要管理员权限。
- 手动普通启动在面板端口成功绑定后自动打开统计页面；`-background` 可保留托盘并阻止启动时打开浏览器，自动重启与开机启动均带此参数。已有的本实例旧版开机启动命令会自动补上该参数；`-no-tray` 调试模式不自动打开浏览器。
- 日志先写文件再写 stderr，避免无控制台构建的 stderr 无效导致丢失文件日志。当前仅在启动时检查超过 5 MiB 的日志并保留 `.1` 备份，不是运行中持续轮转。

## 统计语义与实现边界

- 上传/下载单位是字节；每条连接按 ID 对相邻累计计数做差，不能直接把每次快照的累计值相加。
- `traffic_minute` 按分钟、应用、域名、代理组 / 出口聚合；旧版汇总先迁移至空出口（未记录），再通过 `schema_migrations` 标记的一次性明细回填迁移拆分可确认流量；按分钟/应用/域名匹配，仅没有既有已知出口汇总且明细不超出汇总的组合可回填；完整明细为单一已知出口时才能回填连接数，其余保留未记录，保持总量不变；`connection_events` 按连接 ID、分钟合并明细；`details` 保存连接元数据、完整代理链和起始时间，旧记录默认 `{}`。清理只删除过期明细，分钟聚合长期保留；尚无日聚合表。
- `connections` 计数是采集器首次观察连接时累计的次数，不是每秒采样数，也不是当前活跃连接数。
- 暂停时仍接收快照、更新差分基线，但跳过入库；保留该设计以避免恢复时补记正常采样覆盖的暂停流量。断线跨越暂停区间等边界需单独验证。
- 新进程启动时缺少既有连接的基线，首个快照会把既有累计流量计入当前分钟。短连接可能在采样间隔内结束而漏记，连接消失后的尾部增量也可能漏记。统计不能当作精确计费数据。
- 应用名称来自进程名/路径，缺失时显示“未知应用”；域名优先使用 host，其次 sniffHost，再回退目标 IP。只能归到进程，无法识别进程内的插件、浏览器标签页或 HTTPS 数据内容。
- 首次采样、重连、计数重置、暂停恢复、连接消失和核心重启是改动采集算法时应关注的边界。
- 修改 SQLite 表结构需兼容现有数据库：`CREATE TABLE IF NOT EXISTS` 不会为旧表补上字段或唯一约束。不要通过删除真实数据库解决迁移问题。
- 当前没有单实例保护、独立应用与域名钻取页面或大流量告警。统计与连接查询页共用时间框选、进程、域名和代理组 / 出口多选筛选；浏览器 localStorage 仅保存时间与代理组，预设还原时按当前时间重算（框选按相对偏移还原），自定义时段及框选保留绝对时间，不保存应用、域名；统计页仅自定义显示日期选择器，排行独立搜索分页与 Ctrl 多选，各自选择不收缩自己的列表，但会更新另一列表并重置其分页；图表按展示范围以最多 72 个桶选择预设粒度，分桶以展示起始日期的本地零点对齐（首尾部分桶可能多出 1 个），首尾桶仅累计查询范围内流量，框选不会裁剪曲线或改变其粒度；明细接口限于最近 7 天，域名代理信息也依赖保留的最近 7 天明细。
- 查询真实历史记录使用只读 SQLite 连接。
