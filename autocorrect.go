package main

import (
	"errors"
	"strings"
	"unicode"
)

// A single request can check and write the same draft. Revalidate the target
// after slow local or cloud inference, then compare the exact original and
// read the replacement back before reporting success.
func autoCorrect(field writableField, original string, check func(string) (string, error), verifyTarget func() error) (string, bool, error) {
	if strings.TrimSpace(original) == "" {
		return original, false, nil
	}
	proposed, err := check(original)
	if err != nil {
		return original, false, err
	}
	proposed, n, err := modelPreview(original, proposed)
	if err != nil || n == 0 {
		return original, false, err
	}
	if !sameNumbers(original, proposed) {
		return original, false, errors.New("The spelling engine changed a number. Its result was not applied.")
	}
	if !looksLikeSpellingEdit(original, proposed) {
		return original, false, errors.New("The spelling engine proposed a rewrite. That result was not applied.")
	}
	if err = verifyTarget(); err != nil {
		return original, false, err
	}
	if err = writeReviewed(field, original, proposed); err != nil {
		if mayHaveWritten(err) {
			return proposed, false, err
		}
		return original, false, err
	}
	return proposed, true, nil
}

// A generative engine can return valid JSON that's still a rewrite. Allow
// limited per-word spelling edits and obvious word joins/splits, not a new
// sentence. This is a rejection heuristic, not a claim of semantic equivalence.
func looksLikeSpellingEdit(original, corrected string) bool {
	words := func(s string) []string {
		s = strings.NewReplacer("'", "", "’", "", "ʼ", "").Replace(strings.ToLower(s))
		return strings.FieldsFunc(s, func(c rune) bool {
			return !unicode.IsLetter(c) && !unicode.IsMark(c) && !unicode.IsNumber(c)
		})
	}
	a, b := words(original), words(corrected)
	for len(a) > 0 && len(b) > 0 {
		switch {
		case a[0] == b[0]:
		case negation(a[0]):
			return false
		case len(a) > 1 && !negation(a[1]) && a[0]+a[1] == b[0]:
			a = a[1:]
		case len(b) > 1 && a[0] == b[0]+b[1]:
			b = b[1:]
		case !nearWord(a[0], b[0]):
			return false
		}
		a, b = a[1:], b[1:]
	}
	return len(a) == 0 && len(b) == 0
}

func negation(s string) bool {
	switch s {
	case "no", "not", "never", "none", "nor", "neither", "nothing", "nobody", "cannot",
		"aint", "arent", "cant", "couldnt", "didnt", "doesnt", "dont", "hadnt", "hasnt",
		"havent", "isnt", "mustnt", "neednt", "shouldnt", "wasnt", "werent", "wont", "wouldnt":
		return true
	default:
		return false
	}
}

func nearWord(a, b string) bool {
	x, y := []rune(a), []rune(b)
	k := min(len(x), len(y))
	limit := 1
	if k >= 5 {
		limit = 2
	}
	if k >= 9 {
		limit = 3
	}
	if max(len(x), len(y)) > 80 || max(len(x), len(y))-k > limit {
		return false
	}
	// Damerau-style edit distance counts a neighboring transposition as one typo.
	prev, last, next := make([]int, len(y)+1), make([]int, len(y)+1), make([]int, len(y)+1)
	for j := range last {
		last[j] = j
	}
	for i, r := range x {
		next[0] = i + 1
		for j, s := range y {
			cost := 1
			if r == s {
				cost = 0
			}
			next[j+1] = min(next[j]+1, last[j+1]+1, last[j]+cost)
			if i > 0 && j > 0 && x[i-1] == y[j] && x[i] == y[j-1] {
				next[j+1] = min(next[j+1], prev[j-1]+1)
			}
		}
		prev, last, next = last, next, prev
	}
	return last[len(y)] <= limit
}
