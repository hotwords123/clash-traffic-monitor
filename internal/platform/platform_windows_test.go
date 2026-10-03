package platform

import "testing"

func TestStartupCommand(t *testing.T) {
	got := startupCommand(`D:\Apps\Traffic Monitor\monitor.exe`, `D:\Apps\Traffic Monitor\config.json`)
	want := `"D:\Apps\Traffic Monitor\monitor.exe" -config "D:\Apps\Traffic Monitor\config.json" -background`
	if got != want {
		t.Fatalf("startupCommand() = %q, want %q", got, want)
	}
}
