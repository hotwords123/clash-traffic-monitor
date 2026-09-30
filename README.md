# Clash 流量观察

一个面向 Clash Verge Rev / Mihomo 的本地流量统计工具。它只读取经过代理核心的连接元数据，不解密或保存 HTTPS 内容。

## 当前功能

- 按分钟保存上传和下载流量
- 按应用、域名统计排行
- 查看最近 1 小时、24 小时、7 天或 30 天趋势
- 显示当前连接状态和活跃连接数
- 数据只保存在本机 SQLite

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

```powershell
cd frontend
npm run build
cd ..
go build -trimpath -o clash-traffic-monitor.exe ./cmd/clash-traffic-monitor
```

数据库默认位于 `%LOCALAPPDATA%\ClashTrafficMonitor\traffic.db`。
