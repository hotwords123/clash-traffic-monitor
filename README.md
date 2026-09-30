# Clash 流量观察

一个面向 Clash Verge Rev / Mihomo 的本地流量统计工具。它只读取经过代理核心的连接元数据，不解密或保存 HTTPS 内容。

## 当前功能

- 按分钟保存上传和下载流量
- 按应用、域名统计排行
- 查看最近 1 小时、24 小时、7 天或 30 天趋势
- 显示当前连接状态和活跃连接数
- 数据只保存在本机 SQLite
- Windows 系统托盘常驻，不显示控制台黑窗口
- 托盘菜单支持打开面板、打开日志、暂停/恢复采集和退出
- 可从托盘直接开启或关闭当前用户的开机启动

## Clash Verge Rev 设置

1. 打开“设置 → Clash 设置 → 外部控制”。
2. 监听地址设为 `127.0.0.1:9097`，不要使用 `0.0.0.0`。
3. 设置一个随机密钥。
4. 在项目根目录把 `config.example.json` 复制为 `config.json`，填入相同密钥。
5. 建议在 Mihomo 配置中启用 `enable-process: true` 和 `find-process-mode: always`，否则应用名称可能显示为“未知应用”。

## 开发

```powershell
cd frontend
npm install
npm run build
cd ..
go mod tidy
go run ./cmd/clash-traffic-monitor -config config.json
```

浏览器打开 <http://127.0.0.1:17890>。

## 构建

运行 `scripts\build.ps1`。脚本会构建前端、运行测试，并生成不显示控制台窗口的 `clash-traffic-monitor.exe`。

```powershell
.\scripts\build.ps1
```

双击程序后，它会常驻 Windows 通知区域。左键托盘图标可打开流量面板；右键可以打开面板或日志文件夹、暂停/恢复采集、设置开机启动以及退出。

如需在控制台中调试日志，可以构建普通版本并使用 `-no-tray`：

```powershell
go run ./cmd/clash-traffic-monitor -config config.json -no-tray
```

数据库默认位于 `%LOCALAPPDATA%\ClashTrafficMonitor\traffic.db`。
日志默认位于 `%LOCALAPPDATA%\ClashTrafficMonitor\logs\traffic-monitor.log`，超过 5 MB 时保留一份轮转备份。
