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
	if !looksLikeProofreading(original, proposed) && !looksLikeProofreading(basicProofread(original), proposed) {
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

// Full-sentence rewrites aren't suitable for automatic write-back. Keep
// the original word order and allow a small number of grammar edits in
// addition to spelling fixes. This is not semantic equivalence checking.
func looksLikeProofreading(original, corrected string) bool {
	words := func(s string) []string {
		s = strings.NewReplacer("'", "", "’", "", "ʼ", "").Replace(strings.ToLower(s))
		return strings.FieldsFunc(s, func(c rune) bool {
			return !unicode.IsLetter(c) && !unicode.IsMark(c) && !unicode.IsNumber(c)
		})
	}
	a, b := words(original), words(corrected)
	budget := max(2, min(6, len(a)/8+1))
	prev, prior := "", ""
	for len(a) > 0 && len(b) > 0 {
		left, right := a[0], b[0]
		switch {
		case left == right:
		case negation(left) || negation(right):
			return false
		case subjectPronoun(prev) && bareInfinitiveTrigger(prior) && grammarAlternatives(left, right):
			return false
		case budget > 0 && len(b) > 1 && structuralWord(right, prev, left) && left == b[1]:
			b, budget = b[1:], budget-1
			continue
		case budget > 0 && len(a) > 1 && article(left) && left == prev && a[1] == right:
			a, budget = a[1:], budget-1
			continue
		case budget > 0 && grammarAlternatives(left, right):
			budget--
		case protectedWord(left) || protectedUnit(right):
			return false
		case len(a) > 1 && !protectedWord(a[1]) && left+a[1] == right:
			a = a[1:]
		case len(b) > 1 && !protectedWord(b[1]) && left == right+b[1]:
			b = b[1:]
		case !nearWord(left, right):
			return false
		}
		prior, prev = prev, b[0]
		a, b = a[1:], b[1:]
	}
	return len(a) == 0 && len(b) == 0
}

func article(s string) bool { return s == "a" || s == "an" || s == "the" }
func subjectPronoun(s string) bool {
	switch s {
	case "i", "you", "he", "she", "it", "we", "they":
		return true
	}
	return false
}
func structuralWord(inserted, preceding, following string) bool {
	if article(inserted) {
		return true
	}
	switch inserted {
	case "am", "is", "are":
		if !subjectPronoun(preceding) || agree(preceding, inserted) != inserted {
			return false
		}
		switch following {
		case "ready", "tired", "hungry", "thirsty", "cold", "hot", "sleepy", "awake", "asleep",
			"happy", "sad", "afraid", "upset", "uncomfortable", "sick", "fine", "dizzy",
			"busy", "late", "early", "here", "there", "going", "coming", "waiting":
			return true
		}
	}
	return false
}
func bareInfinitiveTrigger(s string) bool {
	switch s {
	case "let", "lets", "make", "makes", "made", "help", "helps", "helped",
		"see", "sees", "saw", "watch", "watches", "watched", "hear", "hears", "heard",
		"feel", "feels", "felt", "notice", "notices", "noticed":
		return true
	}
	return false
}
func grammarAlternatives(a, b string) bool {
	for _, group := range [][]string{{"a", "an"}, {"is", "am", "are"}, {"was", "were"}, {"have", "has"}, {"do", "does"}} {
		left, right := false, false
		for _, w := range group {
			left = left || w == a
			right = right || w == b
		}
		if left && right {
			return true
		}
	}
	return false
}
func protectedUnit(s string) bool {
	switch s {
	case "mg", "mcg", "kg", "ml", "dl", "iu", "meq", "mm", "cm", "mmhg":
		return true
	}
	return false
}
func protectedWord(s string) bool {
	if negation(s) || subjectPronoun(s) || protectedUnit(s) {
		return true
	}
	switch s {
	case "me", "him", "her", "us", "them", "my", "your", "his", "its", "our", "their":
		return true
	}
	return false
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
