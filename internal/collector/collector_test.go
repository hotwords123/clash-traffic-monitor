package collector

import (
	"testing"

	"github.com/hotwords123/clash-traffic-monitor/internal/mihomo"
)

func TestNames(t *testing.T) {
	metadata := mihomo.Metadata{
		ProcessPath:   `C:\Program Files\Browser\browser.exe`,
		SniffHost:     "VIDEO.EXAMPLE.COM.",
		DestinationIP: "203.0.113.10",
	}
	if got := appName(metadata); got != "browser.exe" {
		t.Fatalf("appName() = %q", got)
	}
	if got := domainName(metadata); got != "video.example.com" {
		t.Fatalf("domainName() = %q", got)
	}
}

func TestPauseState(t *testing.T) {
	collector := &Collector{}
	collector.SetPaused(true)
	if !collector.Paused() || !collector.Status().Paused {
		t.Fatal("collector should report paused state")
	}
	collector.SetPaused(false)
	if collector.Paused() || collector.Status().Paused {
		t.Fatal("collector should report resumed state")
	}
}
