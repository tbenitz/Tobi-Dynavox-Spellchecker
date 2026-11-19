package main

import (
	"errors"
	"strings"
)

type fieldKey struct {
	Executable string
	ID         string
	Class      string
	Name       string
}

func (k fieldKey) valid() bool {
	return k.Executable != "" && (k.ID != "" || k.Name != "") && k.Class != ""
}

func (k fieldKey) matches(candidate fieldKey) bool {
	return k.valid() && strings.EqualFold(k.Executable, candidate.Executable) &&
		k.Class == candidate.Class && (k.ID != "" && k.ID == candidate.ID || k.ID == "" && k.Name == candidate.Name)
}

func getEngine(id string) engine {
	for _, e := range engines {
		if e.id == id {
			return e
		}
	}
	return engines[0]
}

// Foreground is not proof of which app launched the command. Require one
// positively matched link before reading or sending a draft.
func chooseLinkedWindow(matches []uintptr) (uintptr, error) {
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return 0, errors.New("No linked AAC field was found. Open its message page or use Link AAC field in Settings.")
	default:
		return 0, errors.New("More than one AAC window matches this link. Close the other AAC window before checking.")
	}
}
