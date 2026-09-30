package ui

import (
	"bytes"
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

func TestPlaceholders(t *testing.T) {
	got := Placeholders(42, 3, 2)
	cell := func(r, c int) string { return string([]rune{placeholder, diacritics[r], diacritics[c]}) }
	want := "\x1b[38;5;42m" + cell(0, 0) + cell(0, 1) + cell(0, 2) + "\x1b[39m\n" +
		"\x1b[38;5;42m" + cell(1, 0) + cell(1, 1) + cell(1, 2) + "\x1b[39m"
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	if diacritics[9] != 0x034A || diacritics[19] != 0x0365 {
		t.Fatal("diacritics table typo")
	}
}

func TestTransmitChunks(t *testing.T) {
	s := Transmit(7, 20, 10, bytes.Repeat([]byte{0xff}, 5000)) // 6668 b64 chars -> 2 chunks
	parts := strings.Split(s, "\x1b\\")
	if len(parts) != 3 || !strings.HasPrefix(parts[0], "\x1b_Ga=T,U=1,f=100,i=7,c=20,r=10,q=2,m=1;") || !strings.HasPrefix(parts[1], "\x1b_Gm=0;") {
		t.Fatalf("bad chunking: %q...", s[:80])
	}
}

// Each placeholder must land in one width-1 cell with its diacritics and the
// indexed fg, under both width methods bubbletea may pick.
func TestCellsSurviveUV(t *testing.T) {
	for _, m := range []ansi.Method{ansi.WcWidth, ansi.GraphemeWidth} {
		buf := uv.NewScreenBuffer(20, 10)
		buf.Method = m
		uv.NewStyledString(Placeholders(42, 20, 10)).Draw(buf, buf.Bounds())
		for r := 0; r < 10; r++ {
			for c := 0; c < 20; c++ {
				cl := buf.CellAt(c, r)
				if want := string([]rune{placeholder, diacritics[r], diacritics[c]}); cl.Content != want || cl.Width != 1 {
					t.Fatalf("method %v cell %d,%d = %q w%d", m, r, c, cl.Content, cl.Width)
				}
				if cl.Style.Fg != ansi.IndexedColor(42) {
					t.Fatalf("method %v fg = %#v", m, cl.Style.Fg)
				}
			}
		}
	}
}
