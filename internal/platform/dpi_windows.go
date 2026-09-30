package platform

import (
	"fmt"
	"golang.org/x/sys/windows"
)

// EnableDPIAwareness must run before systray creates its hidden window or menus.
// Per Monitor V2 lets Windows render native popup menus at the monitor's DPI.
func EnableDPIAwareness() error {
	proc := windows.NewLazySystemDLL("user32.dll").NewProc("SetProcessDpiAwarenessContext")
	if err := proc.Find(); err != nil {
		return fmt.Errorf("find DPI awareness API: %w", err)
	}
	// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 is the signed pseudo-handle -4.
	result, _, err := proc.Call(^uintptr(3))
	if result == 0 {
		return fmt.Errorf("enable Per Monitor V2 DPI awareness: %w", err)
	}
	return nil
}
