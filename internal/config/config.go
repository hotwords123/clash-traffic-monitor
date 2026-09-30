package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Listen           string `json:"listen"`
	MihomoURL        string `json:"mihomo_url"`
	MihomoSecret     string `json:"mihomo_secret"`
	SampleIntervalMS int    `json:"sample_interval_ms"`
	RawRetentionDays int    `json:"raw_retention_days"`
	DatabasePath     string `json:"database_path,omitempty"`
}

func Defaults() Config {
	return Config{
		Listen:           "127.0.0.1:17890",
		MihomoURL:        "http://127.0.0.1:9097",
		SampleIntervalMS: 1000,
		RawRetentionDays: 7,
	}
}

func Load(path string) (Config, error) {
	cfg := Defaults()
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err == nil {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("parse config: %w", err)
		}
	}
	if cfg.Listen == "" || cfg.MihomoURL == "" {
		return cfg, errors.New("listen and mihomo_url must not be empty")
	}
	if cfg.SampleIntervalMS < 250 {
		cfg.SampleIntervalMS = 250
	}
	if cfg.RawRetentionDays < 1 {
		cfg.RawRetentionDays = 7
	}
	if cfg.DatabasePath == "" {
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			var err error
			base, err = os.UserConfigDir()
			if err != nil {
				return cfg, fmt.Errorf("locate data directory: %w", err)
			}
		}
		cfg.DatabasePath = filepath.Join(base, "ClashTrafficMonitor", "traffic.db")
	}
	return cfg, nil
}
