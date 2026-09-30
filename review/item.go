package review

// Part is which half of an item is being asked.
type Part int

const (
	Meaning Part = iota
	Reading
)

func (p Part) String() string {
	if p == Reading {
		return "reading"
	}
	return "meaning"
}

// Item is one thing to review, already flattened from the API's subject,
// assignment, and study-material data.
type Item struct {
	AssignmentID  int
	Type          string // "radical", "kanji", "vocabulary", "kana_vocabulary"
	Characters    string
	Meanings      []string // accepted answers, primary first
	Blacklist     []string
	Readings      []string // accepted answers, primary first; empty means meaning only
	OtherReadings []string // kanji readings WaniKani knows but is not asking for
	ReadingKind   string   // "on'yomi", "kun'yomi", "nanori", or ""
}

// HasReading reports whether this item asks for a reading at all.
// Radicals and kana-only vocabulary do not.
func (it Item) HasReading() bool { return len(it.Readings) > 0 }
