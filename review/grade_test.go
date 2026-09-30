package review

import (
	"strings"
	"testing"
)

var (
	big = Item{
		Type: "kanji", Characters: "大",
		Meanings:      []string{"Big", "Large"},
		Readings:      []string{"たい", "だい"},
		OtherReadings: []string{"おお"},
		ReadingKind:   "on'yomi",
	}
	adult = Item{
		Type: "vocabulary", Characters: "大人",
		Meanings:  []string{"Adult", "Grown Up"},
		Blacklist: []string{"Big Person"},
		Readings:  []string{"おとな"},
	}
)

func TestGradeMeaning(t *testing.T) {
	tests := []struct {
		item  Item
		input string
		want  Verdict
	}{
		{big, "big", Correct},
		{big, "  BIG ", Correct},
		{adult, "  Grown   UP ", Correct},
		{big, "larg", CorrectTypo},    // "large" has 5 letters: 1 edit allowed
		{adult, "adlut", CorrectTypo}, // transposition counts as 1 edit
		{big, "bog", Wrong},           // 3 letters: no edits allowed
		{adult, "big person", Wrong},  // blacklisted
		{adult, "adult2", Wrong},      // digits must match exactly
		{big, "たい", Warn},             // kana in a meaning answer
	}
	for _, tt := range tests {
		if got := GradeMeaning(tt.item, tt.input); got.Verdict != tt.want {
			t.Errorf("GradeMeaning(%s, %q) = %v, want %v", tt.item.Characters, tt.input, got.Verdict, tt.want)
		}
	}
}

func TestGradeReading(t *testing.T) {
	tests := []struct {
		input string
		want  Verdict
	}{
		{"tai", Correct},
		{"dai", Correct},
		{"タイ", Correct},
		{"たい", Correct},
		{"ookii", Wrong},
		{"big", Warn}, // still has latin letters after conversion
	}
	for _, tt := range tests {
		if got := GradeReading(big, tt.input); got.Verdict != tt.want {
			t.Errorf("GradeReading(%q) = %v, want %v", tt.input, got.Verdict, tt.want)
		}
	}
	g := GradeReading(big, "oo")
	if g.Verdict != Warn || !strings.Contains(g.Hint, "on'yomi") {
		t.Errorf("GradeReading(oo) = %+v, want a Warn mentioning on'yomi", g)
	}
}

func TestOSADistance(t *testing.T) {
	for _, tt := range []struct {
		a, b string
		want int
	}{{"abc", "acb", 1}, {"kitten", "sitting", 3}, {"", "abc", 3}, {"same", "same", 0}} {
		if got := osaDistance(tt.a, tt.b); got != tt.want {
			t.Errorf("osaDistance(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestTypoTolerance(t *testing.T) {
	for n, want := range map[int]int{3: 0, 4: 1, 5: 1, 6: 2, 7: 2, 8: 3, 14: 4} {
		if got := typoTolerance(n); got != want {
			t.Errorf("typoTolerance(%d) = %d, want %d", n, got, want)
		}
	}
}
