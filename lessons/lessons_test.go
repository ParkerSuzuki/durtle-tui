package lessons

import (
	"testing"
	"time"
)

func ids(cs []Candidate) []int {
	var out []int
	for _, c := range cs {
		out = append(out, c.SubjectID)
	}
	return out
}

func equal(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

var avail = []Candidate{
	{SubjectID: 30, Level: 2, Type: "vocabulary"},
	{SubjectID: 10, Level: 1, Type: "vocabulary"},
	{SubjectID: 5, Level: 1, Type: "kanji"},
	{SubjectID: 6, Level: 1, Type: "kanji"},
	{SubjectID: 1, Level: 1, Type: "radical"},
	{SubjectID: 11, Level: 1, Type: "kana_vocabulary"},
	{SubjectID: 20, Level: 2, Type: "radical"},
}

func TestPick(t *testing.T) {
	all := Default(5)
	all.DailyCap = 100
	noVocab := all
	noVocab.Types.Vocabulary = false
	inter := all
	inter.Order = Interleaved
	tests := []struct {
		name    string
		s       Settings
		started int
		want    []int
	}{
		{"classic: level, then radical, kanji, vocab, then id", all, 0, []int{1, 5, 6, 10, 11, 20, 30}},
		{"type filter drops vocabulary and kana vocabulary", noVocab, 0, []int{1, 5, 6, 20}},
		{"interleaved deals by type, skipping types that ran out", inter, 0, []int{1, 5, 10, 20, 6, 11, 30}},
		{"cap minus lessons already started today", Settings{DailyCap: 3, Order: Classic, Types: all.Types}, 1, []int{1, 5}},
		{"cap used up", Settings{DailyCap: 2, Order: Classic, Types: all.Types}, 5, nil},
		{"cap zero", Settings{DailyCap: 0, Order: Classic, Types: all.Types}, 0, nil},
	}
	for _, tt := range tests {
		if got := ids(Pick(avail, tt.started, tt.s)); !equal(got, tt.want) {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestStartedToday(t *testing.T) {
	loc := time.FixedZone("test", -6*3600)
	now := time.Date(2026, 9, 30, 0, 30, 0, 0, loc)
	started := []time.Time{
		time.Date(2026, 9, 29, 23, 59, 0, 0, loc),     // yesterday
		time.Date(2026, 9, 30, 0, 0, 0, 0, loc),       // exactly midnight: today
		time.Date(2026, 9, 30, 6, 10, 0, 0, time.UTC), // 00:10 local
	}
	if got := StartedToday(now, started); got != 2 {
		t.Errorf("StartedToday = %d, want 2", got)
	}
}

func TestDefaultAndClamp(t *testing.T) {
	if d := Default(0); d.BatchSize != 5 || d.DailyCap != 10 || d.Order != Classic || d.Types != (Types{true, true, true}) {
		t.Errorf("Default(0) = %+v", d)
	}
	if d := Default(4); d.BatchSize != 4 {
		t.Errorf("Default(4).BatchSize = %d", d.BatchSize)
	}
	got := Settings{DailyCap: 500, Order: "weird", BatchSize: 1}.Clamp()
	if got.DailyCap != 100 || got.Order != Classic || got.BatchSize != 3 || got.Types != (Types{true, true, true}) {
		t.Errorf("Clamp = %+v", got)
	}
}

func TestMarkup(t *testing.T) {
	got := Markup("The <radical>ground</radical> reads <reading>じ</reading>, <b>not</b> <ja>地</ja>.")
	want := []Span{{"The ", ""}, {"ground", "radical"}, {" reads ", ""}, {"じ", "reading"}, {", ", ""},
		{"not", ""}, {" ", ""}, {"地", "ja"}, {".", ""}}
	if len(got) != len(want) {
		t.Fatalf("Markup = %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("span %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if got := Markup("a < b"); len(got) != 1 || got[0].Text != "a < b" {
		t.Errorf("a lone < must stay text: %+v", got)
	}
}
