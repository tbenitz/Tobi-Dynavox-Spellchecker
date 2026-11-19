package main

import "testing"

func TestChooseLinkedWindowRequiresUniqueSavedMatch(t *testing.T) {
	for _, tc := range []struct {
		name    string
		matches []uintptr
		want    uintptr
		wantErr bool
	}{
		{"two linked windows", []uintptr{10, 20}, 0, true},
		{"one saved link", []uintptr{20}, 20, false},
		{"not open", nil, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, err := chooseLinkedWindow(tc.matches)
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
