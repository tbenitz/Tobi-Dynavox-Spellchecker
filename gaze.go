package main

import "strings"

type fieldKey struct {
	Executable string
	ID         string
	Class      string
}

func (k fieldKey) valid() bool {
	return k.Executable != "" && k.ID != "" && k.Class != ""
}

func (k fieldKey) matches(candidate fieldKey) bool {
	return k.valid() && strings.EqualFold(k.Executable, candidate.Executable) &&
		k.ID == candidate.ID && k.Class == candidate.Class
}

func getEngine(id string) engine {
	for _, e := range engines {
		if e.id == id {
			return e
		}
	}
	return engines[0]
}
