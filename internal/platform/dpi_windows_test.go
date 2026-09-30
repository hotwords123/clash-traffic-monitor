package platform

import (
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"runtime"
	"testing"
)

// Process DPI is immutable: verify in an isolated process, without creating a tray.
func TestDPIAwareness(t *testing.T) {
	if os.Getenv("CTM_DPI_TEST_CHILD") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestDPIAwareness$")
		cmd.Env = append(os.Environ(), "CTM_DPI_TEST_CHILD=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("DPI child: %v\n%s", err, output)
		}
		return
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := EnableDPIAwareness(); err != nil {
		t.Fatal(err)
	}
	user32 := windows.NewLazySystemDLL("user32.dll")
	context, _, _ := user32.NewProc("GetThreadDpiAwarenessContext").Call()
	equal, _, _ := user32.NewProc("AreDpiAwarenessContextsEqual").Call(context, ^uintptr(3))
	if equal == 0 {
		t.Fatal("thread did not inherit Per Monitor V2 awareness")
	}
}
