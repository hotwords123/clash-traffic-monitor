package platform

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const (
	runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValue   = "ClashTrafficMonitor"
)

func OpenURL(target string) error {
	return exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", target).Start()
}

func OpenFolder(path string) error {
	return exec.Command("explorer.exe", path).Start()
}

func StartupEnabled() (bool, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("open startup registry key: %w", err)
	}
	defer key.Close()
	_, _, err = key.GetStringValue(runValue)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read startup registry value: %w", err)
	}
	return true, nil
}

func SetStartup(enabled bool, executable, configPath string) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open startup registry key: %w", err)
	}
	defer key.Close()
	if !enabled {
		if err := key.DeleteValue(runValue); err != nil && !errors.Is(err, registry.ErrNotExist) {
			return fmt.Errorf("remove startup registry value: %w", err)
		}
		return nil
	}
	command := startupCommand(executable, configPath)
	if err := key.SetStringValue(runValue, command); err != nil {
		return fmt.Errorf("write startup registry value: %w", err)
	}
	return nil
}

func startupCommand(executable, configPath string) string {
	return quoteWindowsArgument(executable) + " -config " + quoteWindowsArgument(configPath)
}

func quoteWindowsArgument(value string) string {
	if value != "" && !strings.ContainsAny(value, " \t\"") {
		return value
	}
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}
