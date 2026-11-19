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

type gazeReview struct {
	busy, attached, valid, modelFailed bool
	draft, proposal, engineID          string
}

func (s gazeReview) actions() (apply, fallback bool) {
	if s.busy || !s.attached || strings.TrimSpace(s.draft) == "" {
		return false, false
	}
	return s.valid && strings.TrimSpace(s.proposal) != "" && s.proposal != s.draft,
		s.modelFailed && getEngine(s.engineID).id != "windows"
}
