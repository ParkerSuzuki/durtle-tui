package review

import "testing"

func TestNextStage(t *testing.T) {
	tests := []struct {
		stage, incorrect, want int
	}{
		{1, 0, 2},
		{4, 0, 5}, // Apprentice 4 -> Guru 1
		{8, 0, 9}, // Enlightened -> Burned
		{4, 1, 3}, // below Guru: down ceil(1/2) = 1
		{4, 2, 3}, // ceil(2/2) = 1
		{4, 3, 2}, // ceil(3/2) = 2
		{5, 1, 3}, // Guru and above: penalty doubles
		{8, 3, 4}, // ceil(3/2) * 2 = 4
		{2, 9, 1}, // never below Apprentice 1
		{6, 4, 2},
	}
	for _, tt := range tests {
		if got := NextStage(tt.stage, tt.incorrect); got != tt.want {
			t.Errorf("NextStage(%d, %d) = %d, want %d", tt.stage, tt.incorrect, got, tt.want)
		}
	}
}

func TestStageName(t *testing.T) {
	for stage, want := range map[int]string{1: "Apprentice 1", 4: "Apprentice 4", 5: "Guru 1", 6: "Guru 2",
		7: "Master", 8: "Enlightened", 9: "Burned"} {
		if got := StageName(stage); got != want {
			t.Errorf("StageName(%d) = %q, want %q", stage, got, want)
		}
	}
}
