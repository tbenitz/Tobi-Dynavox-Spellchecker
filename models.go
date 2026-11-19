package main

import (
	"errors"
	"strings"
	"unicode/utf16"
)

type engine struct{ id, label string }

var engines = [...]engine{
	{"windows", "Windows Spell Checking (default)"},
	{"neu-bert", "NeuSpell BERT"},
	{"neu-lstm", "NeuSpell SC-LSTM"},
	{"bart", "BART English Spelling Correction"},
	{"t5", "T5-large-spell (16 GB RAM recommended)"},
	{"mt5", "SAGE mT5 Large (16 GB RAM recommended)"},
	{"tiny", "Spell Correct Transformer (single-word, experimental)"},
}

func engineAt(i int) engine {
	if i < 0 || i >= len(engines) {
		return engines[0]
	}
	return engines[i]
}

func modelPreview(draft, result string) (string, int, error) {
	if strings.TrimSpace(result) == "" || strings.ContainsRune(result, 0) || len(utf16.Encode([]rune(result))) > maxDraft {
		return "", 0, errors.New("The model returned an empty, invalid, or oversized preview. Your draft was not changed.")
	}
	result = matchLineEndings(result, draft)
	if len(utf16.Encode([]rune(result))) > maxDraft {
		return "", 0, errors.New("The model returned an oversized preview. Your draft was not changed.")
	}
	if result == draft {
		return result, 0, nil
	}
	return result, 1, nil
}
