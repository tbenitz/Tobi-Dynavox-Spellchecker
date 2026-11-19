package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf16"
)

const maxDraft = 10000

type writableField interface {
	read() (string, error)
	set(string) error
}

// Unconfirmed writes might already have changed the field. Their callers
// should keep a guarded undo that only runs if the proposed text is present.
type unconfirmedWrite struct{ error }

func mayHaveWritten(err error) bool {
	var attempted unconfirmedWrite
	return errors.As(err, &attempted)
}

// Compare at write time, not just when opening the review panel. There is no
// atomic compare-and-set in UI Automation. Read back to confirm what landed.
func writeReviewed(f writableField, expected, replacement string) error {
	if strings.TrimSpace(replacement) == "" || strings.ContainsRune(replacement, 0) || len(utf16.Encode([]rune(replacement))) > maxDraft {
		return errors.New("The replacement must contain 1 to 10,000 characters.")
	}
	current, err := f.read()
	if err != nil {
		return errors.New("The captured field is no longer readable. Capture it again or use Copy correction.")
	}
	if current != expected {
		return errors.New("The draft changed in the AAC app. No text was written. Capture the latest draft.")
	}
	if replacement == current {
		return nil
	}
	if err = f.set(replacement); err != nil {
		return unconfirmedWrite{fmt.Errorf("The AAC app did not confirm the write. Check the field before retrying. %w", err)}
	}
	confirmed, err := f.read()
	if err != nil || confirmed != replacement {
		return unconfirmedWrite{errors.New("The replacement could not be verified. Check the AAC field before retrying.")}
	}
	return nil
}

type edit struct {
	start, length int
	replacement   string
}

func applyEdits(s string, changes []edit) (string, int) {
	units := utf16.Encode([]rune(s))
	changes = append([]edit(nil), changes...)
	sort.SliceStable(changes, func(i, j int) bool { return changes[i].start < changes[j].start })
	var out []uint16
	pos, n := 0, 0
	boundary := func(i int) bool { return i == len(units) || units[i] < 0xdc00 || units[i] > 0xdfff }
	for _, e := range changes {
		if e.start < pos || e.start >= len(units) || e.length <= 0 || e.length > len(units)-e.start {
			continue
		}
		end := e.start + e.length
		if !boundary(e.start) || !boundary(end) || strings.ContainsRune(e.replacement, 0) {
			continue
		}
		if string(utf16.Decode(units[e.start:end])) == e.replacement {
			continue
		}
		out = append(out, units[pos:e.start]...)
		out = append(out, utf16.Encode([]rune(e.replacement))...)
		pos = end
		n++
	}
	if n == 0 {
		return s, 0
	}
	out = append(out, units[pos:]...)
	return string(utf16.Decode(out)), n
}

func matchLineEndings(s, reference string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if strings.Contains(reference, "\r\n") {
		return strings.ReplaceAll(s, "\n", "\r\n")
	}
	return s
}
