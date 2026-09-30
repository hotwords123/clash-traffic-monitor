package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/local/clash-traffic-monitor/internal/api"
	"github.com/local/clash-traffic-monitor/internal/collector"
	"github.com/local/clash-traffic-monitor/internal/config"
	"github.com/local/clash-traffic-monitor/internal/mihomo"
	"github.com/local/clash-traffic-monitor/internal/store"
	"github.com/local/clash-traffic-monitor/internal/webui"
)

func main() {
	configPath := flag.String("config", "config.json", "配置文件路径")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fatal("load config", err)
	}
	db, err := store.Open(cfg.DatabasePath)
	if err != nil {
		fatal("open database", err)
	}
	defer db.Close()

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
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	slog.Info("Clash 流量观察已启动", "dashboard", "http://"+cfg.Listen, "database", cfg.DatabasePath)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fatal("serve dashboard", err)
	}
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

func fatal(action string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", action, err)
	os.Exit(1)
}
