package main

import (
	"runtime"
	"strings"
	"testing"
)

func TestWindowsDictionaryMakesSpellingProposal(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hr, _, _ := coInit.Call(0, 0)
	if !ok(hr) {
		t.Fatalf("COM failed: 0x%08X", uint32(hr))
	}
	defer coUninit.Call()
	c, err := spellChecker("en-US")
	if err != nil {
		t.Fatal(err)
	}
	defer c.release()
	got, n, err := propose(c, "I have a speling problem.")
	if err != nil || n == 0 || strings.Contains(got, "speling") || !strings.HasSuffix(got, " problem.") {
		t.Fatalf("got %q, %d, %v", got, n, err)
	}
}

func TestWindowsAutomationServiceInitializes(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hr, _, _ := coInit.Call(0, 0)
	if !ok(hr) {
		t.Fatalf("COM failed: 0x%08X", uint32(hr))
	}
	defer coUninit.Call()
	a, err := makeCOM(&uiaClass, &uiaIID)
	if err != nil {
		t.Fatal(err)
	}
	defer a.release()
}
