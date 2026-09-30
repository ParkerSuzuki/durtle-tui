// durtle-tui is an unofficial third-party terminal client for WaniKani.
package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/ParkerSuzuki/durtle-tui/store"
	"github.com/ParkerSuzuki/durtle-tui/ui"
	"github.com/ParkerSuzuki/durtle-tui/wanikani"
)

func main() {
	dir, err := store.CacheDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "durtle-tui:", err)
		os.Exit(1)
	}
	b := &backend{dir: dir, base: wanikani.BaseURL}
	if _, err := tea.NewProgram(ui.New(b, bigTextSupported())).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "durtle-tui:", err)
		os.Exit(1)
	}
}

// bigTextSupported reports whether the terminal can draw large characters:
// kitty sets KITTY_WINDOW_ID, but tmux inside kitty cannot pass the
// sequence through, so it is off there.
func bigTextSupported() bool {
	return os.Getenv("KITTY_WINDOW_ID") != "" && os.Getenv("TMUX") == ""
}
