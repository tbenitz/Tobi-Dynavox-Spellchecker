package main

import "testing"

func TestBindingMatchesOnlySavedControl(t *testing.T) {
	key := fieldKey{Executable: `C:\AAC\Snap.exe`, ID: "MessageWindow", Class: "TextBox"}
	if !key.matches(fieldKey{Executable: `c:\aac\snap.EXE`, ID: "MessageWindow", Class: "TextBox"}) {
		t.Fatal("same field must survive process restarts and path case")
	}
	for _, other := range []fieldKey{
		{Executable: `C:\Elsewhere\Snap.exe`, ID: "MessageWindow", Class: "TextBox"},
		{Executable: key.Executable, ID: "Search", Class: "TextBox"},
		{Executable: key.Executable, ID: key.ID, Class: "SearchBox"},
		{Executable: key.Executable, ID: "", Class: "TextBox"},
		{},
	} {
		if key.matches(other) {
			t.Fatalf("unexpected match to %+v", other)
		}
	}
	if (fieldKey{Executable: key.Executable, ID: "", Class: "TextBox"}).matches(fieldKey{Executable: key.Executable, ID: "", Class: "TextBox"}) {
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
