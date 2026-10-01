package collector

import (
	"context"
	"encoding/json"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hotwords123/clash-traffic-monitor/internal/mihomo"
	"github.com/hotwords123/clash-traffic-monitor/internal/store"
)

type totals struct {
	upload   int64
	download int64
}

type Status struct {
	Connected         bool      `json:"connected"`
	Paused            bool      `json:"paused"`
	LastSnapshot      time.Time `json:"last_snapshot"`
	LastError         string    `json:"last_error,omitempty"`
	ActiveConnections int       `json:"active_connections"`
	UploadTotal       int64     `json:"upload_total"`
	DownloadTotal     int64     `json:"download_total"`
}

type Collector struct {
	client *mihomo.Client
	store  *store.Store
	mu     sync.RWMutex
	status Status
	paused atomic.Bool
}

func New(client *mihomo.Client, store *store.Store) *Collector {
	return &Collector{client: client, store: store}
}

func (c *Collector) Status() Status {
	c.mu.RLock()
	defer c.mu.RUnlock()
	status := c.status
	status.Paused = c.paused.Load()
	return status
}

func (c *Collector) Paused() bool { return c.paused.Load() }

func (c *Collector) SetPaused(paused bool) {
	c.paused.Store(paused)
	c.mu.Lock()
	c.status.Paused = paused
	c.mu.Unlock()
}

func (c *Collector) Run(ctx context.Context) {
	backoff := time.Second
	previous := make(map[string]totals)
	for ctx.Err() == nil {
		err := c.client.Stream(ctx, func(snapshot mihomo.Snapshot) error {
			now := time.Now()
			deltas := make([]store.Delta, 0, len(snapshot.Connections))
			next := make(map[string]totals, len(snapshot.Connections))
			for _, connection := range snapshot.Connections {
				current := totals{upload: connection.Upload, download: connection.Download}
				old, existed := previous[connection.ID]
				upload := max(0, current.upload-old.upload)
				download := max(0, current.download-old.download)
				next[connection.ID] = current
				if upload == 0 && download == 0 && existed {
					continue
				}
				details, _ := json.Marshal(struct {
					Metadata    mihomo.Metadata `json:"metadata"`
					Chains      []string        `json:"chains"`
					Start       string          `json:"start"`
					RulePayload string          `json:"rule_payload"`
				}{connection.Metadata, connection.Chains, connection.Start, connection.RulePayload})
				deltas = append(deltas, store.Delta{
					Details: details,
					At:      now, Connection: connection.ID,
					App: appName(connection.Metadata), Domain: domainName(connection.Metadata),
					Upload: upload, Download: download, Rule: connection.Rule,
					Proxy:         last(connection.Chains),
					NewConnection: !existed,
				})
			}
			previous = next
			backoff = time.Second
			if !c.paused.Load() {
				if err := c.store.Record(ctx, deltas); err != nil {
					return err
				}
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			c.mu.Lock()
			c.status = Status{Connected: true, Paused: c.paused.Load(), LastSnapshot: now, ActiveConnections: len(snapshot.Connections), UploadTotal: snapshot.UploadTotal, DownloadTotal: snapshot.DownloadTotal}
			c.mu.Unlock()
			return nil
		})
		if ctx.Err() != nil {
			return
		}
		c.mu.Lock()
		c.status.Connected = false
		c.status.LastError = err.Error()
		c.mu.Unlock()
		slog.Warn("collector disconnected", "error", err, "retry_in", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 30*time.Second)
	}
}

func appName(metadata mihomo.Metadata) string {
	if metadata.Process != "" {
		return metadata.Process
	}
	if metadata.ProcessPath != "" {
		return filepath.Base(metadata.ProcessPath)
	}
	return "未知应用"
}

func domainName(metadata mihomo.Metadata) string {
	if metadata.Host != "" {
		return strings.ToLower(strings.TrimSuffix(metadata.Host, "."))
	}
	if metadata.SniffHost != "" {
		return strings.ToLower(strings.TrimSuffix(metadata.SniffHost, "."))
	}
	if metadata.DestinationIP != "" {
		return metadata.DestinationIP
	}
	return "未知目标"
}

func last(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[len(values)-1]
}
