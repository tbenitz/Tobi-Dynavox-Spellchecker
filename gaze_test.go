package main

import "testing"

func TestBindingMatchesOnlySavedControl(t *testing.T) {
	key := fieldKey{`C:\AAC\Snap.exe`, "MessageWindow", "TextBox"}
	if !key.matches(fieldKey{`c:\aac\snap.EXE`, "MessageWindow", "TextBox"}) {
		t.Fatal("same field must survive process restarts and path case")
	}
	for _, other := range []fieldKey{
		{`C:\Elsewhere\Snap.exe`, "MessageWindow", "TextBox"},
		{key.Executable, "Search", "TextBox"},
		{key.Executable, key.ID, "SearchBox"},
		{key.Executable, "", "TextBox"},
		{},
	} {
		if key.matches(other) {
			t.Fatalf("unexpected match to %+v", other)
		}
	}
	if (fieldKey{key.Executable, "", "TextBox"}).matches(fieldKey{key.Executable, "", "TextBox"}) {
		t.Fatal("unnamed field must never be selected automatically")
	}
}

func TestStoredEngineDefaultsToWindows(t *testing.T) {
	for _, id := range []string{"", "bogus", "windows"} {
		if getEngine(id).id != "windows" {
			t.Fatalf("wrong default for %q", id)
		}
	}
	for _, e := range engines {
		if getEngine(e.id) != e {
			t.Fatalf("stored choice %q lost", e.id)
		}
	}
}

func TestGazeReviewActions(t *testing.T) {
	cases := []struct {
		name            string
		state           gazeReview
		apply, fallback bool
	}{
		{"loading", gazeReview{busy: true, attached: true, draft: "wtaer", engineID: "tiny"}, false, false},
		{"correction ready", gazeReview{attached: true, valid: true, draft: "wtaer", proposal: "water", engineID: "tiny"}, true, false},
		{"identical", gazeReview{attached: true, valid: true, draft: "water", proposal: "water", engineID: "tiny"}, false, false},
		{"optional model failed", gazeReview{attached: true, draft: "wtaer", engineID: "tiny", modelFailed: true}, false, true},
		{"field inaccessible", gazeReview{draft: "wtaer", engineID: "tiny", modelFailed: true}, false, false},
		{"default failed", gazeReview{attached: true, draft: "wtaer", engineID: "windows", modelFailed: true}, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if apply, fallback := tc.state.actions(); apply != tc.apply || fallback != tc.fallback {
				t.Fatalf("got apply=%v fallback=%v, want %v %v", apply, fallback, tc.apply, tc.fallback)
			}
		})
	}
}
