package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"testing"
	"time"
	"unsafe"
)

// Use a separate process because UI Automation intentionally rejects fields in
// the spellchecker itself. This is a Win32 test field, not an emulation of TD Snap.
func TestLinkedGazeFieldReadWrite(t *testing.T) {
	const childMode = "SPELLCHECK_GAZE_TEST_CHILD"
	if os.Getenv(childMode) == "1" {
		runtime.LockOSThread()
		instance, _, _ = getModule.Call(0)
		root := create(0, "STATIC", "Test AAC window", 0x00cf0000, 30, 30, 400, 250, 0, 0)
		field := create(0x200, "EDIT", "i has a speling problem.", 0x50010080, 20, 20, 275, 28, root, 4242)
		if root == 0 || field == 0 {
			os.Exit(1)
		}
		show.Call(root, 8) // visible without taking desktop focus
		fmt.Println(root)
		var m message
		for {
			r, _, _ := getMsg.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if int32(r) <= 0 {
				return
			}
			translate.Call(uintptr(unsafe.Pointer(&m)))
			dispatch.Call(uintptr(unsafe.Pointer(&m)))
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLinkedGazeFieldReadWrite$")
	cmd.Env = append(os.Environ(), childMode+"=1")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); _ = cmd.Wait() }()
	line := bufio.NewScanner(out)
	if !line.Scan() {
		t.Fatal("test AAC process did not supply a window")
	}
	raw, err := strconv.ParseUint(line.Text(), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	window := uintptr(raw)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hr, _, _ := coInit.Call(0, 0)
	if !ok(hr) {
		t.Fatalf("COM failed: 0x%08X", uint32(hr))
	}
	defer coUninit.Call()
	ui, err := makeCOM(&uiaClass, &uiaIID)
	if err != nil {
		t.Fatal(err)
	}
	defer ui.release()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	p := gazeProfile{WindowClass: windowClass(window), Field: fieldKey{exe, "4242", "Edit"}}
	field, original, err := findGazeField(ui, window, p)
	if err != nil {
		t.Fatal(err)
	}
	defer field.el.release()
	if original != "i has a speling problem." {
		t.Fatalf("unexpected text in linked field: %q", original)
	}
	const corrected = "I have a spelling problem."
	sp, err := spellChecker("en-US")
	if err != nil {
		t.Fatal(err)
	}
	defer sp.release()
	record := autoRecord{source: window, profile: p, field: field, original: original}
	var changed bool
	record.corrected, changed, err = autoCorrect(field, original, func(draft string) (string, error) {
		proposed, n, err := propose(sp, basicProofread(draft))
		proposed, _, err = finishProofreading(draft, proposed, n, err)
		return proposed, err
	}, func() error { return record.verify(ui) })
	if err != nil || !changed || record.corrected != corrected {
		t.Fatalf("automatic correction got %q, changed=%v, err=%v", record.corrected, changed, err)
	}
	if err := writeReviewed(field, original, "stale"); err == nil {
		t.Fatal("stale preview should not overwrite the field")
	}
	again, got, err := findGazeField(ui, window, p)
	if err != nil {
		t.Fatal(err)
	}
	defer again.el.release()
	if got != corrected {
		t.Fatalf("write-back verification returned %q", got)
	}
	if err := writeReviewed(field, record.corrected, record.original); err != nil {
		t.Fatal(err)
	}
	if got, err := again.read(); err != nil || got != original {
		t.Fatalf("undo returned %q / %v", got, err)
	}
}
