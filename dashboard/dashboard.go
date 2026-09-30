// Package dashboard turns WaniKani data into the numbers the home screen
// shows. It does no I/O, so every panel is unit tested.
package dashboard

import (
	"slices"
	"time"

	"github.com/ParkerSuzuki/durtle-tui/wanikani"
)

// Hour is one hour of the review forecast.
type Hour struct {
	At    time.Time
	Added int // reviews that become available at At
	Total int // everything available by then, including what is available now
}

// Progress is how far along the current level is.
type Progress struct {
	Radicals, RadicalsPassed int
	Kanji, KanjiPassed       int
	KanjiNeeded              int // passing this many kanji levels you up
}

// SRS counts started items per stage group, in StageNames order.
type SRS [5]int

var StageNames = [5]string{"Apprentice", "Guru", "Master", "Enlightened", "Burned"}

// Dashboard is everything the home screen shows.
type Dashboard struct {
	Level, Lessons, Reviews int
	Forecast                []Hour
	Progress                Progress
	SRS                     SRS
}

// Build computes the dashboard at time now.
func Build(now time.Time, level int, sum wanikani.Summary,
	assignments map[int]wanikani.Resource[wanikani.Assignment],
	subjects map[int]wanikani.Resource[wanikani.Subject]) Dashboard {
	d := Dashboard{Level: level}
	for _, e := range sum.Lessons {
		if !e.AvailableAt.After(now) {
			d.Lessons += len(e.SubjectIDs)
		}
	}
	var upcoming []wanikani.SummaryEntry
	for _, e := range sum.Reviews {
		switch {
		case !e.AvailableAt.After(now):
			d.Reviews += len(e.SubjectIDs)
		case len(e.SubjectIDs) > 0 && !e.AvailableAt.After(now.Add(24*time.Hour)):
			upcoming = append(upcoming, e)
		}
	}
	slices.SortFunc(upcoming, func(a, b wanikani.SummaryEntry) int { return a.AvailableAt.Compare(b.AvailableAt) })
	total := d.Reviews
	for _, e := range upcoming {
		total += len(e.SubjectIDs)
		d.Forecast = append(d.Forecast, Hour{At: e.AvailableAt, Added: len(e.SubjectIDs), Total: total})
	}

	passed := map[int]bool{} // by subject ID
	for _, a := range assignments {
		s, ok := subjects[a.Data.SubjectID]
		if !ok || s.Data.HiddenAt != nil || a.Data.Hidden {
			continue
		}
		if a.Data.PassedAt != nil {
			passed[s.ID] = true
		}
		if a.Data.StartedAt != nil && a.Data.SRSStage > 0 {
			d.SRS[stageGroup(a.Data.SRSStage)]++
		}
	}
	for _, s := range subjects {
		if s.Data.Level != level || s.Data.HiddenAt != nil {
			continue
		}
		switch s.Object {
		case "radical":
			d.Progress.Radicals++
			if passed[s.ID] {
				d.Progress.RadicalsPassed++
			}
		case "kanji":
			d.Progress.Kanji++
			if passed[s.ID] {
				d.Progress.KanjiPassed++
			}
		}
	}
	d.Progress.KanjiNeeded = kanjiNeeded(d.Progress.Kanji)
	return d
}

// stageGroup maps an SRS stage (1-9) to its index in SRS.
func stageGroup(stage int) int {
	switch {
	case stage <= 4:
		return 0
	case stage <= 6:
		return 1
	default:
		return stage - 5 // 7 master, 8 enlightened, 9 burned
	}
}

// kanjiNeeded is 90% of n rounded up, in integer math: ceil(9n/10).
func kanjiNeeded(n int) int { return (n*9 + 9) / 10 }
