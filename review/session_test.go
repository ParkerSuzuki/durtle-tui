package review

import (
	"math/rand/v2"
	"testing"
)

var ground = Item{AssignmentID: 1, Type: "radical", Characters: "一", Meanings: []string{"Ground"}}

func seeded(n uint64) *rand.Rand { return rand.New(rand.NewPCG(n, n)) }

// correct returns a right answer for the given part of an item.
func correct(it Item, p Part) string {
	if p == Reading {
		return it.Readings[0]
	}
	return it.Meanings[0]
}

func TestRadicalAsksMeaningOnly(t *testing.T) {
	s := NewSession([]Item{ground}, seeded(1))
	if _, p, _ := s.Current(); p != Meaning {
		t.Fatalf("first part = %v, want meaning", p)
	}
	if g, sub := s.Answer("sky"); g.Verdict != Wrong || sub != nil {
		t.Fatalf("wrong answer: got %v, %v", g.Verdict, sub)
	}
	g, sub := s.Answer("ground")
	if g.Verdict != Correct || sub == nil {
		t.Fatalf("right answer: got %v, %v", g.Verdict, sub)
	}
	want := Submission{AssignmentID: 1, IncorrectMeaning: 1}
	if *sub != want {
		t.Errorf("submission = %+v, want %+v", *sub, want)
	}
	if _, _, ok := s.Current(); ok {
		t.Error("session should be over")
	}
}

func TestBackToBack(t *testing.T) {
	kanji := big
	kanji.AssignmentID = 2
	s := NewSession([]Item{kanji, ground}, seeded(7))
	if it, p, _ := s.Current(); !it.HasReading() {
		s.Answer(correct(it, p)) // get the radical out of the way
	}
	first, p1, _ := s.Current()
	if _, sub := s.Answer(correct(first, p1)); sub != nil {
		t.Fatal("kanji finished after only one part")
	}
	same, p2, _ := s.Current()
	if same.AssignmentID != first.AssignmentID || p2 == p1 {
		t.Fatalf("after part %v of %s, got part %v of %s; want the other part of the same item",
			p1, first.Characters, p2, same.Characters)
	}
	wrong := map[Part]string{Meaning: "definitely wrong", Reading: "ぜんぜん"}
	warn := map[Part]string{Meaning: "あ", Reading: "xyz"}
	if g, _ := s.Answer(wrong[p2]); g.Verdict != Wrong {
		t.Errorf("wrong %v answer graded %v", p2, g.Verdict)
	}
	if g, _ := s.Answer(warn[p2]); g.Verdict != Warn {
		t.Errorf("wrong-kind %v answer graded %v, want Warn", p2, g.Verdict)
	}
	_, sub := s.Answer(correct(same, p2))
	if sub == nil {
		t.Fatal("item should be finished")
	}
	if got := sub.IncorrectMeaning + sub.IncorrectReading; got != 1 {
		t.Errorf("incorrect total = %d, want 1 (warnings do not count)", got)
	}
	if s.Total() != 2 {
		t.Errorf("Total = %d, want 2", s.Total())
	}
}

func TestPartOrderIsRandom(t *testing.T) {
	seen := map[Part]bool{}
	for seed := range uint64(40) {
		_, p, _ := NewSession([]Item{big}, seeded(seed)).Current()
		seen[p] = true
	}
	if !seen[Meaning] || !seen[Reading] {
		t.Errorf("over 40 seeds saw %v; want both meaning-first and reading-first", seen)
	}
}
