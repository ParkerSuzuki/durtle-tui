package main

import "testing"

// Big text needs kitty itself on the other end. Multiplexers such as herdr
// and tmux inherit KITTY_WINDOW_ID from the outer kitty but set their own
// TERM, and they drop kitty's text sizing sequence.
func TestBigTextSupported(t *testing.T) {
	for _, tt := range []struct {
		name, term, kittyID string
		want                bool
	}{
		{"kitty", "xterm-kitty", "1", true},
		{"herdr inside kitty", "xterm-256color", "1", false},
		{"tmux inside kitty", "tmux-256color", "1", false},
		{"other terminal", "xterm-256color", "", false},
	} {
		t.Setenv("TERM", tt.term)
		t.Setenv("KITTY_WINDOW_ID", tt.kittyID)
		t.Setenv("TMUX", "")
		if got := bigTextSupported(); got != tt.want {
			t.Errorf("%s: bigTextSupported() = %v, want %v", tt.name, got, tt.want)
		}
	}
}
