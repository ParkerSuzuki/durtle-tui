package dashboard

import (
	"testing"
	"time"

	"github.com/ParkerSuzuki/durtle-tui/wanikani"
)

var now = time.Date(2026, 9, 30, 14, 30, 0, 0, time.UTC)

// hour is the start of the hour n hours from now's hour.
func hour(n int) time.Time { return now.Truncate(time.Hour).Add(time.Duration(n) * time.Hour) }

func entry(at time.Time, ids ...int) wanikani.SummaryEntry {
	return wanikani.SummaryEntry{AvailableAt: at, SubjectIDs: ids}
}

func TestAvailableAndForecast(t *testing.T) {
	sum := wanikani.Summary{
		Lessons: []wanikani.SummaryEntry{entry(hour(0), 1, 2, 3)},
		Reviews: []wanikani.SummaryEntry{
			entry(hour(0), 1, 2), // 14:00, before now: available
			entry(now, 3),        // exactly now: available, not forecast
			entry(hour(5), 7),    // out of order on purpose
			entry(hour(2), 4, 5),
			entry(hour(1)),     // empty hour: left out
			entry(hour(25), 9), // more than 24 hours away: left out
		},
	}
	d := Build(now, 1, sum, nil, nil)
	if d.Lessons != 3 || d.Reviews != 3 {
		t.Errorf("lessons %d reviews %d, want 3 and 3", d.Lessons, d.Reviews)
	}
	want := []Hour{{hour(2), 2, 5}, {hour(5), 1, 6}}
	if len(d.Forecast) != len(want) {
		t.Fatalf("forecast = %+v, want %+v", d.Forecast, want)
	}
	for i := range want {
		if d.Forecast[i] != want[i] {
			t.Errorf("forecast[%d] = %+v, want %+v", i, d.Forecast[i], want[i])
		}
	}
}

func TestProgressAndSRS(t *testing.T) {
	subjects := map[int]wanikani.Resource[wanikani.Subject]{}
	add := func(id int, object string, level int, hidden bool) {
		s := wanikani.Subject{Level: level}
		if hidden {
			s.HiddenAt = &now
		}
		subjects[id] = wanikani.Resource[wanikani.Subject]{ID: id, Object: object, Data: s}
	}
	for i := 0; i < 33; i++ {
		add(100+i, "kanji", 12, false)
	}
	add(200, "radical", 12, false)
	add(201, "radical", 12, false)
	add(300, "kanji", 12, true) // hidden: ignored everywhere
	add(400, "kanji", 11, false)

	assignments := map[int]wanikani.Resource[wanikani.Assignment]{}
	assign := func(id, subject, stage int, passed bool) {
		a := wanikani.Assignment{SubjectID: subject, SRSStage: stage}
		if stage > 0 {
			a.StartedAt = &now
		}
		if passed {
			a.PassedAt = &now
		}
		assignments[id] = wanikani.Resource[wanikani.Assignment]{ID: id, Data: a}
	}
	for i := 0; i < 21; i++ {
		assign(1000+i, 100+i, 5, true) // 21 level-12 kanji at Guru, passed
	}
	assign(2000, 200, 1, false) // radical in Apprentice
	assign(2001, 201, 9, true)  // radical burned
	assign(3000, 300, 8, true)  // hidden subject
	assign(4000, 400, 7, true)  // level-11 kanji at Master: counts in SRS only
	assign(5000, 132, 0, false) // lesson not done yet: not in SRS

	d := Build(now, 12, wanikani.Summary{}, assignments, subjects)
	if want := (Progress{Radicals: 2, RadicalsPassed: 1, Kanji: 33, KanjiPassed: 21, KanjiNeeded: 30}); d.Progress != want {
		t.Errorf("progress = %+v, want %+v", d.Progress, want)
	}
	if want := (SRS{1, 21, 1, 0, 1}); d.SRS != want {
		t.Errorf("SRS = %v, want %v", d.SRS, want)
	}
	if d.Level != 12 {
		t.Errorf("level = %d", d.Level)
	}
}

func TestKanjiNeeded(t *testing.T) {
	for n, want := range map[int]int{0: 0, 10: 9, 33: 30, 34: 31, 40: 36} {
		if got := kanjiNeeded(n); got != want {
			t.Errorf("kanjiNeeded(%d) = %d, want %d", n, got, want)
		}
	}
}
