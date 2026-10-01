package review

import "fmt"

// Burned is the last SRS stage; a burned item is never reviewed again.
const Burned = 9

// NextStage is the SRS stage an item moves to after a review with the given
// total of incorrect answers (meaning plus reading), by WaniKani's published
// rule (https://knowledge.wanikani.com/wanikani/srs-stages/): up one stage
// with no mistakes; otherwise down by half the incorrect answers, rounded
// up, doubled at Guru (stage 5) and above, never below Apprentice 1.
func NextStage(stage, incorrect int) int {
	if incorrect == 0 {
		return min(stage+1, Burned)
	}
	penalty := 1
	if stage >= 5 {
		penalty = 2
	}
	return max(stage-(incorrect+1)/2*penalty, 1)
}

// StageName names an SRS stage the way WaniKani does.
func StageName(stage int) string {
	switch {
	case stage <= 0:
		return "Lesson"
	case stage <= 4:
		return fmt.Sprintf("Apprentice %d", stage)
	case stage <= 6:
		return fmt.Sprintf("Guru %d", stage-4)
	case stage == 7:
		return "Master"
	case stage == 8:
		return "Enlightened"
	}
	return "Burned"
}

// StageChange describes a finished review's effect, e.g.
// "Guru 1 → Apprentice 3 ↓".
func StageChange(stage, incorrect int) string {
	next := NextStage(stage, incorrect)
	arrow := "↑"
	if next < stage {
		arrow = "↓"
	}
	return fmt.Sprintf("%s → %s %s", StageName(stage), StageName(next), arrow)
}
