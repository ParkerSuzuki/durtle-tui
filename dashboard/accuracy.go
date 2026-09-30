package dashboard

import (
	"time"

	"github.com/ParkerSuzuki/durtle-tui/wanikani"
)

// Answers counts answers: every meaning and reading answer separately,
// wrong attempts included, the way WaniKani counts review accuracy.
type Answers struct {
	Correct   int `json:"correct"`
	Incorrect int `json:"incorrect"`
}

func (a Answers) Total() int { return a.Correct + a.Incorrect }

// Percent is the share of correct answers, 0 when there are none.
func (a Answers) Percent() float64 {
	if a.Total() == 0 {
		return 0
	}
	return 100 * float64(a.Correct) / float64(a.Total())
}

// Days maps local dates (time.DateOnly, "2006-01-02") to that day's answers.
type Days map[string]Answers

// AddDeltas adds, for each statistic that grew since old, the new answers
// to the local day of its last update. With no old statistics (the first
// sync) there is no baseline, so nothing is recorded. A statistic missing
// from old is a new item: its answers count in full.
func AddDeltas(days Days, old map[int]wanikani.Resource[wanikani.ReviewStatistic],
	fresh []wanikani.Resource[wanikani.ReviewStatistic], loc *time.Location) {
	if len(old) == 0 {
		return
	}
	for _, r := range fresh {
		prev := old[r.ID].Data // zero value for a new item
		correct := r.Data.MeaningCorrect + r.Data.ReadingCorrect - prev.MeaningCorrect - prev.ReadingCorrect
		incorrect := r.Data.MeaningIncorrect + r.Data.ReadingIncorrect - prev.MeaningIncorrect - prev.ReadingIncorrect
		if r.Data.Hidden || (correct <= 0 && incorrect <= 0) {
			continue
		}
		day := r.DataUpdatedAt.In(loc).Format(time.DateOnly)
		a := days[day]
		a.Correct += max(correct, 0) // counters only shrink on a level reset
		a.Incorrect += max(incorrect, 0)
		days[day] = a
	}
}

// Prune drops days older than keep days before now.
func (d Days) Prune(now time.Time, keep int) {
	cutoff := now.AddDate(0, 0, 1-keep).Format(time.DateOnly)
	for day := range d {
		if day < cutoff { // ISO dates sort as strings
			delete(d, day)
		}
	}
}

// Zone is where accuracy sits relative to WaniKani's Learning Zone (85-95%).
type Zone int

const (
	NoData Zone = iota
	BelowZone
	InZone
	AboveZone
)

// Learning Zone bounds, from https://knowledge.wanikani.com/widgets/correct-percentage/
const (
	ZoneLow  = 85.0
	ZoneHigh = 95.0
)

func ZoneOf(a Answers) Zone {
	switch p := a.Percent(); {
	case a.Total() == 0:
		return NoData
	case p < ZoneLow:
		return BelowZone
	case p > ZoneHigh:
		return AboveZone
	}
	return InZone
}
