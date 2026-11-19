package main

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	proofWord    = regexp.MustCompile(`[\p{L}\p{M}]+(?:['’][\p{L}\p{M}]+)*`)
	proofLiteral = regexp.MustCompile(`(?i)(?:\b[a-z][a-z0-9+.-]*://|\bwww\.)[^\s<>"“”]+|[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}\b`)
)

// Finish every engine's result, including unchanged, correctly spelled drafts.
// Nothing is sent by this step. The original draft still controls write guards.
func finishProofreading(original, proposed string, edits int, err error) (string, int, error) {
	if err != nil {
		return "", 0, err
	}
	proposed, changed, err := modelPreview(original, basicProofread(proposed))
	if err != nil || changed == 0 {
		return proposed, 0, err
	}
	return proposed, max(1, edits), nil
}

func basicProofread(s string) string {
	literals := proofLiteral.FindAllStringIndex(s, -1)
	for _, pos := range literals {
		pos[1] = pos[0] + len(strings.TrimRight(s[pos[0]:pos[1]], ".,!?;:)]}"))
	}
	var out strings.Builder
	pos, literal := 0, 0
	prev, prior := "", ""
	prevClauseStart := false
	for _, span := range proofWord.FindAllStringIndex(s, -1) {
		for literal < len(literals) && literals[literal][1] <= span[0] {
			literal++
		}
		raw := s[span[0]:span[1]]
		gap := s[pos:span[0]]
		out.WriteString(gap)
		if literal < len(literals) && literals[literal][0] < span[1] {
			out.WriteString(raw)
			prev, prior = "", ""
			prevClauseStart = false
			pos = span[1]
			continue
		}
		lineStart := pos == 0 || strings.ContainsAny(gap, "\r\n")
		linePrefix := gap[strings.LastIndexAny(gap, "\r\n")+1:]
		capitalize := lineStart && strings.IndexFunc(linePrefix, unicode.IsNumber) < 0 ||
			afterSentence(gap, prev)
		word := raw
		if prevClauseStart && horizSpace(gap) && raw == strings.ToLower(raw) {
			word = agree(prev, raw)
			if word == "dont" && subjectPronoun(prev) {
				word = "don't"
			}
		}
		word = personalCapital(word)
		if capitalize && word == strings.ToLower(word) {
			r, n := utf8.DecodeRuneInString(word)
			word = string(unicode.ToUpper(r)) + word[n:]
		}
		// Fix unambiguous article sounds without guessing the pronunciation of names.
		if horizSpace(gap) && (prev == "a" || prev == "an") {
			want := articleFor(strings.ToLower(raw))
			if want != "" && want != prev {
				// Previous word is at the tail, before this known whitespace.
				text := out.String()
				start := len(text) - len(gap) - len(prior)
				if start >= 0 {
					if prior == "A" || prior == "An" {
						want = "A" + want[1:]
					}
					out.Reset()
					out.Grow(len(s) + 8)
					out.WriteString(text[:start] + want + gap)
				}
			}
		}
		out.WriteString(word)
		prev, prior, pos = strings.ToLower(word), word, span[1]
		prevClauseStart = capitalize
	}
	out.WriteString(s[pos:])
	return out.String()
}

func horizSpace(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r != ' ' && r != '\t' {
			return false
		}
	}
	return true
}

func afterSentence(gap, prev string) bool {
	tail := strings.TrimRightFunc(gap, func(c rune) bool {
		return unicode.IsSpace(c) || strings.ContainsRune("\"'”’)]}", c)
	})
	if tail == "" {
		return false
	}
	switch tail[len(tail)-1] {
	case '?', '!':
		return true
	case '.':
		if tail == gap {
			return false // not an unspaced acronym, domain, or identifier
		}
		switch prev {
		case "mr", "mrs", "ms", "mx", "dr", "st", "jr", "sr", "prof", "hon", "rev", "vs", "e", "g":
			return false
		}
		if len(prev) == 1 && prev != "i" {
			return false
		}
		return true
	}
	return false
}

func personalCapital(s string) string {
	l := strings.ToLower(s)
	switch l {
	case "i", "i'm", "i’ve", "i've", "i’m", "i'd", "i’d", "i'll", "i’ll":
		return "I" + s[1:]
	case "doesnt", "didnt", "havent", "hasnt", "isnt", "arent", "wouldnt", "couldnt", "shouldnt", "wasnt", "werent":
		if s == l {
			return s[:len(s)-1] + "'t"
		}
	}
	return s
}

func agree(subject, word string) string {
	switch subject {
	case "i", "you", "we", "they":
		switch word {
		case "am", "is", "are":
			if subject == "i" {
				return "am"
			}
			return "are"
		case "has":
			return "have"
		case "does":
			return "do"
		}
	case "he", "she", "it":
		switch word {
		case "am", "are":
			return "is"
		case "have":
			return "has"
		case "do":
			return "does"
		}
	}
	return word
}

func articleFor(next string) string {
	switch next {
	case "apple", "applesauce", "animal", "answer", "appointment", "office", "idea", "issue", "injury", "option", "orange", "umbrella", "accident", "emergency", "elephant", "egg", "hour", "honest":
		return "an"
	case "one", "once", "unicorn", "university", "uniform", "user", "unit", "usual", "unique", "use", "useful", "european":
		return "a"
	default:
		return ""
	}
}
