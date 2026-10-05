package review

import (
	"slices"
	"strings"
	"unicode/utf8"
)

// Verdict is the outcome of grading one answer.
type Verdict int

const (
	Wrong       Verdict = iota
	Correct             // exact match
	CorrectTypo         // accepted within typo tolerance
	Warn                // not graded: the user typed the wrong kind of answer
)

func (v Verdict) String() string {
	return [...]string{"Wrong", "Correct", "CorrectTypo", "Warn"}[v]
}

// Grade is a Verdict plus, for Warn, a hint to show the user.
type Grade struct {
	Verdict Verdict
	Hint    string
}

func normalizeMeaning(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// GradeMeaning checks a meaning answer the way the website does: exact match
// against accepted meanings and synonyms, blacklist first, then typo tolerance.
func GradeMeaning(it Item, input string) Grade {
	answer := normalizeMeaning(input)
	if containsKana(answer) {
		return Grade{Warn, "We want the meaning, not the reading."}
	}
	for _, b := range it.Blacklist {
		if answer == normalizeMeaning(b) {
			return Grade{Verdict: Wrong}
		}
	}
	for _, m := range it.Meanings {
		if answer == normalizeMeaning(m) {
			return Grade{Verdict: Correct}
		}
	}
	// The reading typed in romaji ("ju" for じゅ) means the user answered the
	// wrong question. Checked after the accepted meanings, so a name whose
	// meaning is its own romanized reading (Tashirojima) stays correct.
	if kana := ToHiragana(strings.ReplaceAll(answer, " ", ""), true); !containsLatin(kana) {
		for _, r := range append(slices.Clone(it.Readings), it.OtherReadings...) {
			if kana == KatakanaToHiragana(r) {
				return Grade{Warn, "That's the reading. We want the meaning."}
			}
		}
	}
	if strings.ContainsAny(answer, "0123456789") {
		return Grade{Verdict: Wrong}
	}
	for _, m := range it.Meanings {
		m = normalizeMeaning(m)
		if osaDistance(answer, m) <= typoTolerance(utf8.RuneCountInString(m)) {
			return Grade{Verdict: CorrectTypo}
		}
	}
	return Grade{Verdict: Wrong}
}

// GradeReading checks a reading answer: exact match after conversion to
// hiragana. A kanji reading of the wrong type is a warning, not a miss.
func GradeReading(it Item, input string) Grade {
	answer := KatakanaToHiragana(ToHiragana(strings.TrimSpace(input), true))
	for _, r := range it.Readings {
		if answer == KatakanaToHiragana(r) {
			return Grade{Verdict: Correct}
		}
	}
	// The input box converts as you type, so a meaning typed here arrives
	// half converted ("mankind" becomes "まんきんd"). Convert each meaning the
	// same way to recognize it. Checked after the accepted readings: some
	// names' meaning is their own romanized reading (田代島, "Tashirojima").
	for _, m := range it.Meanings {
		if answer == ToHiragana(normalizeMeaning(m), true) {
			return Grade{Warn, "That's the meaning. We want the reading."}
		}
	}
	if containsLatin(answer) {
		return Grade{Warn, "Some letters didn't turn into kana. We want the reading."}
	}
	for _, r := range it.OtherReadings {
		if answer == KatakanaToHiragana(r) {
			if it.ReadingKind != "" {
				return Grade{Warn, "Close, but we want the " + it.ReadingKind + "."}
			}
			return Grade{Warn, "Close, but that's not the reading we want here."}
		}
	}
	return Grade{Verdict: Wrong}
}

// typoTolerance is how many edits a meaning of n letters allows.
// Community reverse-engineered from the website (see the spec).
func typoTolerance(n int) int {
	switch {
	case n <= 3:
		return 0
	case n <= 5:
		return 1
	case n <= 7:
		return 2
	default:
		return 2 + n/7
	}
}

// osaDistance is the Optimal String Alignment distance: Levenshtein edits
// plus swapping two adjacent letters as a single edit.
func osaDistance(a, b string) int {
	s, t := []rune(a), []rune(b)
	d := make([][]int, len(s)+1)
	for i := range d {
		d[i] = make([]int, len(t)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(s); i++ {
		for j := 1; j <= len(t); j++ {
			cost := 1
			if s[i-1] == t[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && s[i-1] == t[j-2] && s[i-2] == t[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(s)][len(t)]
}
