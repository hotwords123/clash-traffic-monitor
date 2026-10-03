package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/hotwords123/clash-traffic-monitor/internal/api"
	"github.com/hotwords123/clash-traffic-monitor/internal/collector"
	"github.com/hotwords123/clash-traffic-monitor/internal/config"
	"github.com/hotwords123/clash-traffic-monitor/internal/logging"
	"github.com/hotwords123/clash-traffic-monitor/internal/mihomo"
	"github.com/hotwords123/clash-traffic-monitor/internal/platform"
	"github.com/hotwords123/clash-traffic-monitor/internal/store"
	trayapp "github.com/hotwords123/clash-traffic-monitor/internal/tray"
	"github.com/hotwords123/clash-traffic-monitor/internal/webui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Clash 流量观察启动失败：%v\n", err)
		os.Exit(1)
	}
}

func run() error {
	dpiError := platform.EnableDPIAwareness()
	configPath := flag.String("config", "config.json", "配置文件路径")
	noTray := flag.Bool("no-tray", false, "不显示托盘图标，用于控制台调试")
	background := flag.Bool("background", false, "后台启动，不自动打开浏览器，仍可通过托盘打开面板")
	flag.Parse()

	absoluteConfigPath, err := filepath.Abs(*configPath)
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}
	cfg, err := config.Load(absoluteConfigPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logFile, logPath, err := logging.Setup(cfg.LogDirectory)
	if err != nil {
		return fmt.Errorf("setup logging: %w", err)
	}
	defer logFile.Close()
	if dpiError != nil {
		slog.Warn("initialize DPI awareness", "error", dpiError)
	}

	db, err := store.Open(cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()
	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return fmt.Errorf("listen for dashboard: %w", err)
	}
	defer listener.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	client := mihomo.New(cfg.MihomoURL, cfg.MihomoSecret, cfg.SampleIntervalMS)
	trafficCollector := collector.New(client, db)
	go trafficCollector.Run(ctx)
	go cleanupLoop(ctx, db, cfg.RawRetentionDays)

	server := &http.Server{
		Addr:              cfg.Listen,
		Handler:           api.New(db, trafficCollector, webui.Files()).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	serverError := make(chan error, 1)
	go func() {
		err := server.Serve(listener)
		if err != nil && err != http.ErrServerClosed {
			serverError <- err
			stop()
			return
		}
		serverError <- nil
	}()

	dashboardURL := "http://" + cfg.Listen
	slog.Info("Clash 流量观察已启动", "dashboard", dashboardURL, "database", cfg.DatabasePath, "log", logPath)

	var runError error
	if *noTray {
		select {
		case <-ctx.Done():
		case runError = <-serverError:
		}
	} else {
		tray, err := trayapp.New(trafficCollector, dashboardURL, cfg.LogDirectory, absoluteConfigPath, stop)
		if err != nil {
			stop()
			runError = fmt.Errorf("create tray: %w", err)
		} else {
			if !*background {
				if err := platform.OpenURL(dashboardURL); err != nil {
					slog.Warn("open dashboard on startup", "error", err)
				}
			}
			if err := tray.Run(ctx); err != nil {
				stop()
				runError = fmt.Errorf("run tray: %w", err)
			}
		}
	}
	select {
	case err := <-serverError:
		if err != nil && runError == nil {
			runError = fmt.Errorf("serve dashboard: %w", err)
		}
	default:
	}

	stop()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil && runError == nil {
		runError = fmt.Errorf("stop dashboard: %w", err)
	}
	if runError != nil {
		slog.Error("application stopped with error", "error", runError)
	}
	return runError
}

func cleanupLoop(ctx context.Context, db *store.Store, days int) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		if err := db.Cleanup(ctx, days); err != nil && ctx.Err() == nil {
			slog.Warn("cleanup failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
