package main

import (
	"strings"
	"testing"
)

func TestEngineCatalog(t *testing.T) {
	want := []string{"windows", "neu-bert", "neu-lstm", "bart", "t5", "mt5", "tiny", "deepseek", "grok"}
	if len(engines) != len(want) {
		t.Fatalf("catalog has %d entries", len(engines))
	}
	for i, id := range want {
		if e := engineAt(i); e.id != id || e.label == "" {
			t.Fatalf("engine %d: %+v", i, e)
		}
		if engineIndex(id) != i {
			t.Fatalf("incorrect selector index for %q", id)
		}
	}
	if engineIndex("unknown") != 0 {
		t.Fatal("unknown engine should return to Windows")
	}
	for _, i := range []int{-1, len(want)} {
		if engineAt(i).id != "windows" {
			t.Fatalf("invalid index %d bypassed default", i)
		}
	}
}

func TestModelPreviewValidation(t *testing.T) {
	for _, s := range []string{"", "  ", "a\x00b", strings.Repeat("😀", maxDraft/2+1)} {
		if _, _, err := modelPreview("Draft", s); err == nil {
			t.Fatalf("invalid model result accepted")
		}
	}
	if s, n, err := modelPreview("Helo\r\nthere", "Hello\nthere"); err != nil || n != 1 || s != "Hello\r\nthere" {
		t.Fatalf("unexpected preview: %q %d %v", s, n, err)
	}
	if _, n, err := modelPreview("Draft", "Draft"); n != 0 || err != nil {
		t.Fatalf("unchanged result: %d %v", n, err)
	}
}
