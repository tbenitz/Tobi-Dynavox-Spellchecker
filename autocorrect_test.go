package main

import (
	"errors"
	"testing"
)

type automaticTestField struct {
	text   string
	writes int
}

func (f *automaticTestField) read() (string, error) { return f.text, nil }
func (f *automaticTestField) set(s string) error    { f.text = s; f.writes++; return nil }

func TestOneTapCorrectionWritesExactlyOnce(t *testing.T) {
	f := &automaticTestField{text: typoSample}
	checks := 0
	got, changed, err := autoCorrect(f, typoSample, func(s string) (string, error) {
		checks++
		if s != typoSample {
			t.Fatalf("wrong draft: %q", s)
		}
		return correctedSample, nil
	}, func() error { return nil })
	if err != nil || !changed || got != correctedSample || f.text != got || checks != 1 || f.writes != 1 {
		t.Fatalf("text=%q changed=%v check calls=%d writes=%d err=%v", f.text, changed, checks, f.writes, err)
	}
}

func TestOneTapCorrectionLeavesDraftAloneOnFailure(t *testing.T) {
	for _, tc := range []struct {
		name, preview                       string
		checkErr, targetErr, stale, wantErr bool
	}{
		{"engine failure", "", true, false, false, true},
		{"invalid proposal", "", false, false, false, true},
		{"no changes", typoSample, false, false, false, false},
		{"target changed", correctedSample, false, true, false, true},
		{"draft changed during check", correctedSample, false, false, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &automaticTestField{text: typoSample}
			_, changed, err := autoCorrect(f, typoSample, func(string) (string, error) {
				if tc.stale {
					f.text = "New draft"
				}
				if tc.checkErr {
					return "", errors.New("offline")
				}
				return tc.preview, nil
			}, func() error {
				if tc.targetErr {
					return errors.New("wrong window")
				}
				return nil
			})
			if changed || f.writes != 0 || (err != nil) != tc.wantErr {
				t.Fatalf("changed=%v writes=%d err=%v", changed, f.writes, err)
			}
		})
	}
}

func TestOneTapKeepsLineEndings(t *testing.T) {
	f := &automaticTestField{text: "Helo\r\nwurld"}
	got, _, err := autoCorrect(f, f.text, func(string) (string, error) { return "Hello\nworld", nil }, func() error { return nil })
	if err != nil || got != "Hello\r\nworld" || f.text != got {
		t.Fatalf("got %q / %q, %v", got, f.text, err)
	}
}

func TestOneTapRejectsRewritesAndNumberChanges(t *testing.T) {
	for _, v := range [][2]string{
		{"I don't want that.", "I do want that."},
		{"I can not go.", "I can now go."},
		{"Call my sister.", "Phone your brother, please."},
		{"I need 2 mL.", "I need 3 mL."},
		{"Please call Chris.", ""},
	} {
		f := &automaticTestField{text: v[0]}
		_, changed, err := autoCorrect(f, f.text, func(string) (string, error) { return v[1], nil }, func() error { return nil })
		if changed || err == nil || f.writes != 0 {
			t.Fatalf("accepted rewrite %q -> %q", v[0], v[1])
		}
	}
}

type uncertainTestField struct {
	text   string
	checks int
}

func (f *uncertainTestField) read() (string, error) {
	f.checks++
	if f.checks == 2 {
		return "", errors.New("read back unavailable")
	}
	return f.text, nil
}
func (f *uncertainTestField) set(s string) error { f.text = s; return nil }

func TestOneTapRetainsPossibleWriteForGuardedUndo(t *testing.T) {
	f := &uncertainTestField{text: "I wnat water."}
	original := f.text
	replacement, changed, err := autoCorrect(f, original, func(string) (string, error) { return "I want water.", nil }, func() error { return nil })
	if changed || !mayHaveWritten(err) || replacement != f.text {
		t.Fatalf("proposal=%q live=%q changed=%v err=%v", replacement, f.text, changed, err)
	}
	if err := writeReviewed(f, replacement, original); err != nil || f.text != original {
		t.Fatalf("guarded undo returned %q: %v", f.text, err)
	}
}
