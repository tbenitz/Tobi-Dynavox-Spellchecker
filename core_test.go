package main

import (
	"errors"
	"testing"
	"unicode/utf16"
)

type sampleField struct {
	value  string
	writes int
	ignore bool
	fail   bool
}

func (f *sampleField) read() (string, error) {
	if f.fail {
		return "", errors.New("gone")
	}
	return f.value, nil
}
func (f *sampleField) set(s string) error {
	f.writes++
	if !f.ignore {
		f.value = s
	}
	return nil
}

func TestReviewedWriteRejectsChangedDraft(t *testing.T) {
	f := &sampleField{value: "new words"}
	if err := writeReviewed(f, "old words", "fixed words"); err == nil || f.writes != 0 {
		t.Fatal("overwrote newer text")
	}
}
func TestReviewedWriteAndUndo(t *testing.T) {
	f := &sampleField{value: "I wnat water."}
	if err := writeReviewed(f, f.value, "I want water."); err != nil || f.value != "I want water." {
		t.Fatal("apply failed", err)
	}
	if err := writeReviewed(f, "I want water.", "I wnat water."); err != nil || f.value != "I wnat water." {
		t.Fatal("undo failed", err)
	}
}
func TestWriteIsNotReportedWhenProviderIgnoresIt(t *testing.T) {
	f := &sampleField{value: "old", ignore: true}
	if err := writeReviewed(f, "old", "new"); err == nil {
		t.Fatal("unverified write was treated as successful")
	}
}
func TestUnreadableAndEmptyFieldsAreNotWritten(t *testing.T) {
	f := &sampleField{value: "old", fail: true}
	if err := writeReviewed(f, "old", "new"); err == nil || f.writes != 0 {
		t.Fatal("unreadable field was overwritten")
	}
	f.fail = false
	if err := writeReviewed(f, "old", "  "); err == nil || f.writes != 0 {
		t.Fatal("empty correction wrote to field")
	}
}
func TestSpellingEditsPreserveUnicodeLineEndsAndPunctuation(t *testing.T) {
	in := "🙂 I wnat this.\r\nThnks!"
	off := func(s string) int { return len(utf16.Encode([]rune(s))) }
	got, n := applyEdits(in, []edit{{off("🙂 I wnat this.\r\n"), 5, "Thanks"}, {off("🙂 I "), 4, "want"}})
	if want := "🙂 I want this.\r\nThanks!"; got != want || n != 2 {
		t.Fatalf("got %q, %d", got, n)
	}
}
func TestBadOffsetsAndOverlapAreSkipped(t *testing.T) {
	got, n := applyEdits("🙂test", []edit{{1, 1, "x"}, {2, 4, "okay"}, {3, 1, "y"}, {15, 1, "z"}})
	if got != "🙂okay" || n != 1 {
		t.Fatalf("got %q, %d", got, n)
	}
}

func TestLineEndingsPreserveProviderStyle(t *testing.T) {
	if got := matchLineEndings("one\r\ntwo", "old\nmessage"); got != "one\ntwo" {
		t.Fatalf("got %q", got)
	}
	if got := matchLineEndings("one\ntwo", "old\r\nmessage"); got != "one\r\ntwo" {
		t.Fatalf("got %q", got)
	}
}
