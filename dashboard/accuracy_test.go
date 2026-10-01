package dashboard

import (
	"testing"
	"time"

	"github.com/ParkerSuzuki/durtle-tui/wanikani"
)

func stat(id int, at time.Time, mc, mi, rc, ri int) wanikani.Resource[wanikani.ReviewStatistic] {
	return wanikani.Resource[wanikani.ReviewStatistic]{ID: id, DataUpdatedAt: at, Data: wanikani.ReviewStatistic{
		MeaningCorrect: mc, MeaningIncorrect: mi, ReadingCorrect: rc, ReadingIncorrect: ri}}
}

func TestAddDeltas(t *testing.T) {
	loc := time.FixedZone("test", -6*3600)
	today := time.Date(2026, 9, 30, 9, 0, 0, 0, loc)
	yesterday := time.Date(2026, 9, 29, 23, 30, 0, 0, loc)
	old := map[int]wanikani.Resource[wanikani.ReviewStatistic]{1: stat(1, yesterday, 5, 1, 4, 0)}

	days := Days{}
	AddDeltas(days, nil, []wanikani.Resource[wanikani.ReviewStatistic]{stat(1, today, 9, 9, 9, 9)}, loc)
	if len(days) != 0 {
		t.Errorf("first sync has no baseline and must record nothing, got %v", days)
	}

	fresh := []wanikani.Resource[wanikani.ReviewStatistic]{
		stat(1, today, 6, 2, 5, 0),     // +2 correct, +1 incorrect today
		stat(2, yesterday, 1, 0, 1, 1), // new item: counts in full, on its own day
		stat(3, today, 0, 0, 0, 0),     // nothing answered yet
	}
	AddDeltas(days, old, fresh, loc)
	if got := days["2026-09-30"]; got != (Answers{Correct: 2, Incorrect: 1}) {
		t.Errorf("today = %+v", got)
	}
	if got := days["2026-09-29"]; got != (Answers{Correct: 2, Incorrect: 1}) {
		t.Errorf("yesterday = %+v", got)
	}
}

func TestZoneOf(t *testing.T) {
	for _, tt := range []struct {
		a    Answers
		want Zone
	}{
		{Answers{}, NoData},
		{Answers{Correct: 8499, Incorrect: 1501}, BelowZone}, // 84.99%
		{Answers{Correct: 85, Incorrect: 15}, InZone},
		{Answers{Correct: 95, Incorrect: 5}, InZone},
		{Answers{Correct: 9501, Incorrect: 499}, AboveZone}, // 95.01%
	} {
		if got := ZoneOf(tt.a); got != tt.want {
			t.Errorf("ZoneOf(%+v) = %v, want %v", tt.a, got, tt.want)
		}
	}
}

func TestPrune(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	days := Days{"2026-09-30": {Correct: 1}, "2026-09-01": {Correct: 1}, "2026-08-31": {Correct: 1}}
	days.Prune(now, 30)
	if _, ok := days["2026-08-31"]; ok || len(days) != 2 {
		t.Errorf("after pruning to 30 days: %v", days)
	}
}

func statFor(id, subject int, at time.Time, mi, ri int) wanikani.Resource[wanikani.ReviewStatistic] {
	return wanikani.Resource[wanikani.ReviewStatistic]{ID: id, DataUpdatedAt: at, Data: wanikani.ReviewStatistic{
		SubjectID: subject, MeaningCorrect: 5, MeaningIncorrect: mi, ReadingIncorrect: ri}}
}

func TestAddMistakes(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	earlier := now.Add(-2 * time.Hour)
	old := map[int]wanikani.Resource[wanikani.ReviewStatistic]{
		1: statFor(1, 101, earlier, 1, 0),
		2: statFor(2, 102, earlier, 0, 0),
		3: statFor(3, 103, earlier, 2, 2),
	}
	m := Mistakes{}
	AddMistakes(m, nil, []wanikani.Resource[wanikani.ReviewStatistic]{statFor(1, 101, now, 9, 9)})
	if len(m) != 0 {
		t.Errorf("first sync is a baseline, got %v", m)
	}
	AddMistakes(m, old, []wanikani.Resource[wanikani.ReviewStatistic]{
		statFor(1, 101, now, 2, 0),     // meaning missed again
		statFor(2, 102, now, 0, 1),     // reading missed
		statFor(3, 103, now, 2, 2),     // answered, but no new mistakes
		statFor(4, 104, earlier, 1, 0), // new item, already missed
	})
	if len(m) != 3 || !m[101].Equal(now) || !m[102].Equal(now) || !m[104].Equal(earlier) {
		t.Errorf("mistakes = %v; want subjects 101, 102 (now) and 104 (earlier)", m)
	}
	m[999] = now.Add(-25 * time.Hour)
	m.Prune(now)
	if _, ok := m[999]; ok || len(m) != 3 {
		t.Errorf("after pruning to 24 hours: %v", m)
	}
}
