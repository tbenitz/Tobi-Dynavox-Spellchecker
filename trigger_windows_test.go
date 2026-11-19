package main

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func TestOneTapCommandReachesLinkedFieldColdAndWarm(t *testing.T) {
	const role = "AAC_SPELLCHECK_TRIGGER_HOST"
	if os.Getenv(role) == "1" {
		os.Args = []string{os.Args[0], "--check"}
		main()
		return
	}
	findHost := func() uintptr {
		h, _, _ := findWindow.Call(uintptr(unsafe.Pointer(wide(hostWindowClass))), 0)
		return h
	}
	if findHost() != 0 {
		t.Skip("another AAC Spellchecker host is using this desktop")
	}
	t.Setenv("APPDATA", t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	target := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLinkedGazeFieldReadWrite$")
	target.Env = append(os.Environ(), "SPELLCHECK_GAZE_TEST_CHILD=1")
	out, err := target.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := target.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); _ = target.Wait() }()
	lines := bufio.NewScanner(out)
	if !lines.Scan() {
		t.Fatal("test AAC window did not start")
	}
	parsed, err := strconv.ParseUint(lines.Text(), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	window := uintptr(parsed)
	if activeRoot() == window {
		t.Fatal("fixture must not have keyboard focus")
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	p := gazeProfile{WindowClass: windowClass(window), WindowImage: exe, Field: fieldKey{Executable: exe, ID: "4242", Class: "Edit"}}
	if err := saveProfile(p); err != nil {
		t.Fatal(err)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	r, _, _ := coInit.Call(0, 0)
	if !ok(r) {
		t.Fatal("COM could not start")
	}
	defer coUninit.Call()
	ui, err := makeCOM(&uiaClass, &uiaIID)
	if err != nil {
		t.Fatal(err)
	}
	defer ui.release()
	f, original, err := findGazeField(ui, window, p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.el.release()

	launch := func() *exec.Cmd {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestOneTapCommandReachesLinkedFieldColdAndWarm$")
		cmd.Env = append(os.Environ(), role+"=1")
		return cmd
	}
	cold := launch()
	if err := cold.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); _ = cold.Wait() }()
	waitForText := func(which, want string) {
		t.Helper()
		end := time.Now().Add(7 * time.Second)
		for {
			got, err := f.read()
			if got == want && err == nil {
				return
			}
			if time.Now().After(end) {
				t.Fatalf("%s launch did not correct linked field: got %q, err %v", which, got, err)
			}
			time.Sleep(40 * time.Millisecond)
		}
	}
	const corrected = "I have a spelling problem."
	waitForText("cold", corrected)
	host := findHost()
	if host == 0 {
		t.Fatal("cold launch did not leave a tray host")
	}
	visible, _, _ := isWindowVisible.Call(host)
	if visible != 0 {
		t.Fatal("cold launch opened settings")
	}
	// The write can precede the tray window processing its worker result.
	// Wait for the status set after the host is ready for another request.
	end := time.Now().Add(3 * time.Second)
	for !hostStatusIs(host, "Corrected the AAC draft.") {
		if time.Now().After(end) {
			t.Fatal("cold launch did not publish successful completion")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := writeReviewed(f, corrected, original); err != nil {
		t.Fatal(err)
	}
	if err := launch().Run(); err != nil {
		t.Fatalf("warm handoff: %v", err)
	}
	waitForText("warm", corrected)
	if host != findHost() {
		t.Fatal("warm launch replaced tray host")
	}
	visible, _, _ = isWindowVisible.Call(host)
	if visible != 0 {
		t.Fatal("warm launch opened settings")
	}
}

func hostStatusIs(host uintptr, wanted string) bool {
	h, _, _ := user.NewProc("GetDlgItem").Call(host, uintptr(idStatus))
	if h == 0 {
		return false
	}
	buf := make([]uint16, 256)
	var n uintptr
	ok, _, _ := sendTimeout.Call(h, 0x000D, uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])), 0x23, 300, uintptr(unsafe.Pointer(&n)))
	return ok != 0 && n != 0 && syscall.UTF16ToString(buf) == wanted
}
