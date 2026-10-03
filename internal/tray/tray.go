package tray

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/gogpu/systray"
	"github.com/hotwords123/clash-traffic-monitor/internal/collector"
	"github.com/hotwords123/clash-traffic-monitor/internal/platform"
)

type App struct {
	collector  *collector.Collector
	dashboard  string
	logDir     string
	configPath string
	executable string
	stop       context.CancelFunc
	tray       *systray.SystemTray
	once       sync.Once
}

func New(trafficCollector *collector.Collector, dashboard, logDir, configPath string, stop context.CancelFunc) (*App, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return &App{
		collector: trafficCollector, dashboard: dashboard, logDir: logDir,
		configPath: configPath, executable: executable, stop: stop,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	a.tray = systray.New()
	menu := systray.NewMenu()
	statusItem := menu.Add("状态：正在连接", nil)
	statusItem.SetDisabled(true)
	menu.AddSeparator()
	menu.Add("打开流量面板", func() { a.openURL() })
	menu.Add("打开日志文件夹", func() { a.openLogs() })
	// The library does not toggle checkbox/menu state automatically, so callbacks
	// update both the application state and native menu state explicitly.
	var pauseItem *systray.MenuItem
	pauseItem = menu.Add("暂停采集", func() {
		paused := !a.collector.Paused()
		a.collector.SetPaused(paused)
		if paused {
			pauseItem.SetLabel("恢复采集")
		} else {
			pauseItem.SetLabel("暂停采集")
		}
	})
	menu.AddSeparator()
	if err := platform.RefreshStartupCommand(a.executable, a.configPath); err != nil {
		slog.Warn("refresh startup command", "error", err)
	}
	startupEnabled, err := platform.StartupEnabled()
	if err != nil {
		slog.Warn("read startup setting", "error", err)
	}
	var startupItem *systray.MenuItem
	startupItem = menu.AddCheckbox("开机启动", startupEnabled, func() {
		desired := !startupItem.IsChecked()
		if err := platform.SetStartup(desired, a.executable, a.configPath); err != nil {
			slog.Error("update startup setting", "error", err)
			a.tray.ShowNotification("Clash 流量观察", "无法修改开机启动设置，请查看日志。")
			return
		}
		startupItem.SetChecked(desired)
	})
	menu.AddSeparator()
	menu.Add("退出", func() { a.shutdown() })

	a.tray.SetIcon(makeIcon(false)).SetTooltip("Clash 流量观察").SetMenu(menu)
	a.tray.OnClick(func() { a.openURL() })
	a.tray.Show()

	go a.updateStatus(ctx, statusItem, pauseItem)
	go func() {
		<-ctx.Done()
		a.remove()
	}()
	return a.tray.Run()
}

func (a *App) updateStatus(ctx context.Context, statusItem, pauseItem *systray.MenuItem) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	activeIcon := makeIcon(false)
	pausedIcon := makeIcon(true)
	lastState := ""
	for {
		status := a.collector.Status()
		state := "offline"
		label := "状态：未连接"
		tooltip := "Clash 流量观察 · 未连接"
		if status.Paused {
			state = "paused"
			label = "状态：已暂停"
			tooltip = "Clash 流量观察 · 已暂停"
		} else if status.Connected {
			state = "active"
			label = "状态：采集中"
			tooltip = "Clash 流量观察 · 采集中"
		}
		if state != lastState {
			statusItem.SetLabel(label)
			a.tray.SetTooltip(tooltip)
			if state == "paused" {
				pauseItem.SetLabel("恢复采集")
				a.tray.SetIcon(pausedIcon)
			} else {
				pauseItem.SetLabel("暂停采集")
				a.tray.SetIcon(activeIcon)
			}
			lastState = state
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (a *App) openURL() {
	if err := platform.OpenURL(a.dashboard); err != nil {
		slog.Error("open dashboard", "error", err)
	}
}

func (a *App) openLogs() {
	if err := platform.OpenFolder(a.logDir); err != nil {
		slog.Error("open log directory", "error", err)
	}
}

func (a *App) shutdown() {
	a.stop()
	a.remove()
}

func (a *App) remove() {
	a.once.Do(func() {
		if a.tray != nil {
			a.tray.Remove()
		}
	})
}
