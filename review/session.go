package review

import (
	"math/rand/v2"
	"slices"
)

// Submission is what gets sent to WaniKani when an item is finished.
type Submission struct {
	AssignmentID     int
	IncorrectMeaning int
	IncorrectReading int
}

// Result pairs a finished item with its submission, for the summary screen.
type Result struct {
	Item       Item
	Submission Submission
}

// Session runs items back to back: one item at a time, meaning or reading
// first by coin flip, and a wrong part is asked again until it is right.
type Session struct {
	items   []Item
	pos     int
	parts   []Part // parts of the current item still to answer; parts[0] is being asked
	wrong   [2]int // incorrect answers for the current item, indexed by Part
	results []Result
	rng     *rand.Rand
}

// NewSession shuffles a copy of items and starts on the first one.
func NewSession(items []Item, rng *rand.Rand) *Session {
	s := &Session{items: slices.Clone(items), rng: rng}
	rng.Shuffle(len(s.items), func(i, j int) { s.items[i], s.items[j] = s.items[j], s.items[i] })
	s.start()
	return s
}

func (s *Session) start() {
	s.wrong = [2]int{}
	s.parts = nil
	if s.pos >= len(s.items) {
		return
	}
	s.parts = []Part{Meaning}
	if s.items[s.pos].HasReading() {
		s.parts = append(s.parts, Reading)
		if s.rng.IntN(2) == 0 {
			s.parts[0], s.parts[1] = Reading, Meaning
		}
	}
}

// Current returns the item and part being asked. ok is false once every item is done.
func (s *Session) Current() (it Item, p Part, ok bool) {
	if s.pos >= len(s.items) {
		return Item{}, Meaning, false
	}
	return s.items[s.pos], s.parts[0], true
}

// Answer grades input against the current question. If that finishes the
// item, it returns the Submission to send; otherwise the Submission is nil.
func (s *Session) Answer(input string) (Grade, *Submission) {
	it, part, ok := s.Current()
	if !ok {
		return Grade{Verdict: Wrong}, nil
	}
	var g Grade
	if part == Meaning {
		g = GradeMeaning(it, input)
	} else {
		g = GradeReading(it, input)
	}
	switch g.Verdict {
	case Wrong:
		s.wrong[part]++
	case Correct, CorrectTypo:
		s.parts = s.parts[1:]
		if len(s.parts) == 0 {
			sub := Submission{it.AssignmentID, s.wrong[Meaning], s.wrong[Reading]}
			s.results = append(s.results, Result{it, sub})
			s.pos++
			s.start()
			return g, &sub
		}
	}
	return g, nil
}

// Total is the number of items in the session.
func (s *Session) Total() int { return len(s.items) }

// Results lists finished items in the order they were finished.
func (s *Session) Results() []Result { return s.results }
