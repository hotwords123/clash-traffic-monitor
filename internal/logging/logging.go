package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

const maxLogSize = 5 << 20

func Setup(directory string) (io.Closer, string, error) {
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return nil, "", fmt.Errorf("create log directory: %w", err)
	}
	path := filepath.Join(directory, "traffic-monitor.log")
	if err := rotate(path); err != nil {
		return nil, "", err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, "", fmt.Errorf("open log file: %w", err)
	}
	// GUI-subsystem builds have no valid stderr handle. Write the file first so
	// the persistent log still succeeds even when the console writer rejects it.
	output := io.MultiWriter(file, os.Stderr)
	slog.SetDefault(slog.New(slog.NewTextHandler(output, &slog.HandlerOptions{Level: slog.LevelInfo})))
	return file, path, nil
}

func rotate(path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) || (err == nil && info.Size() <= maxLogSize) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect log file: %w", err)
	}
	backup := path + ".1"
	if err := os.Remove(backup); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove old log backup: %w", err)
	}
	if err := os.Rename(path, backup); err != nil {
		return fmt.Errorf("rotate log file: %w", err)
	}
	return nil
}
