// Package lessons decides which lessons to teach today, from the user's own
// rules, and holds what a lesson shows. It does no I/O.
package lessons

import (
	"cmp"
	"slices"
	"time"

	"github.com/ParkerSuzuki/durtle-tui/review"
)

// Order is how today's lessons are arranged.
type Order string

const (
	Classic     Order = "classic"     // level, then radicals, kanji, vocabulary
	Interleaved Order = "interleaved" // the classic order dealt round-robin by type
)

// Types says which subject types to learn. Vocabulary includes kana vocabulary.
type Types struct {
	Radical    bool `json:"radical"`
	Kanji      bool `json:"kanji"`
	Vocabulary bool `json:"vocabulary"`
}

// InfoLayout is how the item-info screen (f after a wrong answer) is laid out.
type InfoLayout string

const (
	InfoPages  InfoLayout = "pages"  // meaning, reading, and context on separate pages
	InfoSingle InfoLayout = "single" // everything on one scrollable page
)

// Settings are the user's rules and preferences, saved in settings.json.
type Settings struct {
	DailyCap   int        `json:"daily_cap"`
	Order      Order      `json:"order"`
	Types      Types      `json:"types"`
	BatchSize  int        `json:"batch_size"`
	InfoLayout InfoLayout `json:"info_layout"`
}

const (
	MaxDailyCap  = 100
	MinBatchSize = 3
	MaxBatchSize = 10
)

// Default is the starting settings, seeded with the user's WaniKani batch
// size (0 when unknown).
func Default(waniKaniBatch int) Settings {
	if waniKaniBatch == 0 {
		waniKaniBatch = 5
	}
	return Settings{DailyCap: 10, Order: Classic, Types: Types{true, true, true}, BatchSize: waniKaniBatch}.Clamp()
}

// Clamp puts every setting back in range.
func (s Settings) Clamp() Settings {
	s.DailyCap = min(max(s.DailyCap, 0), MaxDailyCap)
	s.BatchSize = min(max(s.BatchSize, MinBatchSize), MaxBatchSize)
	if s.Order != Interleaved {
		s.Order = Classic
	}
	if s.Types == (Types{}) {
		s.Types = Types{true, true, true}
	}
	if s.InfoLayout != InfoSingle {
		s.InfoLayout = InfoPages
	}
	return s
}

func (s Settings) wants(subjectType string) bool {
	switch rank(subjectType) {
	case 0:
		return s.Types.Radical
	case 1:
		return s.Types.Kanji
	}
	return s.Types.Vocabulary
}

// rank orders types the way WaniKani teaches them within a level.
func rank(subjectType string) int {
	switch subjectType {
	case "radical":
		return 0
	case "kanji":
		return 1
	}
	return 2 // vocabulary, kana_vocabulary
}

// Candidate is a lesson available now.
type Candidate struct {
	AssignmentID, SubjectID, Level int
	Type                           string
}

// Pick returns today's lessons: available ones of the wanted types, in the
// chosen order, limited to what the daily cap has left.
func Pick(available []Candidate, startedToday int, s Settings) []Candidate {
	var picked []Candidate
	for _, c := range available {
		if s.wants(c.Type) {
			picked = append(picked, c)
		}
	}
	slices.SortFunc(picked, func(a, b Candidate) int {
		return cmp.Or(cmp.Compare(a.Level, b.Level), cmp.Compare(rank(a.Type), rank(b.Type)), cmp.Compare(a.SubjectID, b.SubjectID))
	})
	if s.Order == Interleaved {
		picked = interleave(picked)
	}
	n := min(len(picked), max(s.DailyCap-startedToday, 0))
	if n == 0 {
		return nil
	}
	return picked[:n]
}

// interleave deals classic-ordered lessons round-robin by type: radical,
// kanji, vocabulary, repeat, skipping types that ran out.
func interleave(sorted []Candidate) []Candidate {
	var queues [3][]Candidate
	for _, c := range sorted {
		r := rank(c.Type)
		queues[r] = append(queues[r], c)
	}
	out := make([]Candidate, 0, len(sorted))
	for len(out) < len(sorted) {
		for r := range queues {
			if len(queues[r]) > 0 {
				out = append(out, queues[r][0])
				queues[r] = queues[r][1:]
			}
		}
	}
	return out
}

// StartedToday counts start times at or after local midnight of now's day
// (in now's time zone), from any client, website included.
func StartedToday(now time.Time, started []time.Time) int {
	y, m, d := now.Date()
	midnight := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	n := 0
	for _, t := range started {
		if !t.Before(midnight) {
			n++
		}
	}
	return n
}

// Lesson is one item to teach: its quiz item plus teaching content.
type Lesson struct {
	review.Item
	MeaningMnemonic, MeaningHint string
	ReadingMnemonic, ReadingHint string
	PartsOfSpeech                []string
	KanjiReadings                []KanjiReading // kanji only
	Components                   []Component    // radicals of a kanji, kanji of a vocabulary word
	Sentences                    []Sentence     // vocabulary only, at most 3
}

type KanjiReading struct {
	Reading, Type string // Type: onyomi, kunyomi, nanori
	Accepted      bool
}

type Component struct {
	Characters, Meaning, Type string // Characters is "" for image-only radicals
}

type Sentence struct{ Ja, En string }

// Plan is today's lessons and how many to teach before each quiz.
type Plan struct {
	Lessons   []Lesson
	BatchSize int
}
