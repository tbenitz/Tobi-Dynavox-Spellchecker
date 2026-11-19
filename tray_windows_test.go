package main

import (
	"context"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func TestTrayDataABIAndStrings(t *testing.T) {
	n := iconData()
	if n.size != 976 || unsafe.Offsetof(n.tip) != 40 || unsafe.Offsetof(n.info) != 304 || unsafe.Offsetof(n.balloonIcon) != 968 {
		t.Fatalf("unexpected NOTIFYICONDATAW layout: size %d, tip %d, info %d, icon %d", n.size, unsafe.Offsetof(n.tip), unsafe.Offsetof(n.info), unsafe.Offsetof(n.balloonIcon))
	}
	for _, want := range []struct{ input, truncated string }{{"ABC", "ABC"}, {"🙂x", "🙂x"}, {"ab🙂", "ab"}} {
		var b [4]uint16
		wideCopy(b[:], want.input)
		if got := syscall.UTF16ToString(b[:]); got != want.truncated {
			t.Fatalf("%q is %q, want %q", want.input, got, want.truncated)
		}
	}
}

func TestTrayHostStartsHiddenAndReopens(t *testing.T) {
	const role = "AAC_SPELLCHECK_TRAY_TEST_HOST"
	if os.Getenv(role) == "1" {
		os.Args = []string{os.Args[0], "--check"}
		main()
		return
	}
	find := func(class string) uintptr {
		h, _, _ := findWindow.Call(uintptr(unsafe.Pointer(wide(class))), 0)
		return h
	}
	if find("Shell_TrayWnd") == 0 {
		t.Skip("Windows Explorer tray is not available in this test session")
	}
	if find(hostWindowClass) != 0 {
		t.Skip("an AAC Spellchecker host is already running on this desktop")
	}
	t.Setenv("APPDATA", t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestTrayHostStartsHiddenAndReopens$")
	cmd.Env = append(os.Environ(), role+"=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); _ = cmd.Wait() }()
	until := func(problem string, f func() bool) {
		t.Helper()
		end := time.Now().Add(3 * time.Second)
		for !f() {
			if time.Now().After(end) {
				t.Fatal(problem)
			}
			time.Sleep(25 * time.Millisecond)
		}
	}
	var host uintptr
	until("tray host did not start", func() bool { host = find(hostWindowClass); return host != 0 })
	var pid uint32
	windowPID.Call(host, uintptr(unsafe.Pointer(&pid)))
	if pid != uint32(cmd.Process.Pid) {
		t.Fatal("found another tray host")
	}
	visible := func() bool { v, _, _ := isWindowVisible.Call(host); return v != 0 }
	if visible() {
		t.Fatal("the AAC launch opened a window")
	}
	handoffExisting(wmOpenSettings, 0)
	until("settings didn't open", visible)
	post.Call(host, 0x0010, 0, 0) // WM_CLOSE hides settings if Explorer registered the tray icon
	until("closing didn't hide settings", func() bool { return !visible() })
	handoffExisting(wmOpenSettings, 0)
	until("settings did not reopen in the resident process", func() bool { return host == find(hostWindowClass) && visible() })
}
