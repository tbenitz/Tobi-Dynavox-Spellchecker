package main

import "testing"

func TestChooseLinkedWindowUsesExactLaunchTargetOrUniqueSavedMatch(t *testing.T) {
	for _, tc := range []struct {
		name      string
		preferred uintptr
		matches   []uintptr
		want      uintptr
		wantErr   bool
	}{
		{"direct AAC launch", 20, []uintptr{10, 20}, 20, false},
		{"launcher took focus", 55, []uintptr{20}, 20, false},
		{"no foreground", 0, []uintptr{20}, 20, false},
		{"two linked windows and neither launched", 55, []uintptr{10, 20}, 0, true},
		{"not open", 55, nil, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, err := chooseLinkedWindow(tc.preferred, tc.matches)
			if w != tc.want || (err != nil) != tc.wantErr {
				t.Fatalf("got %d, %v", w, err)
			}
		})
	}
}

func TestAlternateLinkIdentityRequiresNameWhenAutomationIDIsMissing(t *testing.T) {
	key := fieldKey{Executable: "app.exe", Name: "Message Window", Class: "TextBox"}
	if !key.matches(fieldKey{Executable: "APP.EXE", Name: "Message Window", Class: "TextBox"}) {
		t.Fatal("stable named field could not be matched")
	}
	if key.matches(fieldKey{Executable: "app.exe", Name: "Search", Class: "TextBox"}) || (fieldKey{Executable: "app.exe", Class: "TextBox"}).valid() {
		t.Fatal("an unnamed or different field was blindly accepted")
	}
}
