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

// bigTextSupported reports whether kitty itself is drawing this process, so
// its text sizing sequence will be honored. kitty sets TERM=xterm-kitty;
// multiplexers in between (herdr, tmux, zellij) set their own TERM but
// inherit KITTY_WINDOW_ID from the outer window, so that variable alone lies.
func bigTextSupported() bool {
	return os.Getenv("TERM") == "xterm-kitty"
}
