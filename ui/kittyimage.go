package ui

// Kitty graphics with Unicode placeholders (decision 30): the PNG is sent to
// kitty once with tea.Raw, and the view draws it with ordinary text cells, so
// Bubble Tea's renderer needs no timing tricks.
// Protocol: https://sw.kovidgoyal.net/kitty/graphics-protocol/#unicode-placeholders

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// diacritics maps a row/column number to its combining mark (first 32 of 297).
// Source: https://sw.kovidgoyal.net/kitty/_downloads/f0a0de9ec8d9ff4456206db8e0814937/rowcolumn-diacritics.txt
// (same as https://raw.githubusercontent.com/kovidgoyal/kitty/master/gen/rowcolumn-diacritics.txt)
var diacritics = []rune{
	0x0305, 0x030D, 0x030E, 0x0310, 0x0312, 0x033D, 0x033E, 0x033F,
	0x0346, 0x034A, 0x034B, 0x034C, 0x0350, 0x0351, 0x0352, 0x0357,
	0x035B, 0x0363, 0x0364, 0x0365, 0x0366, 0x0367, 0x0368, 0x0369,
	0x036A, 0x036B, 0x036C, 0x036D, 0x036E, 0x036F, 0x0483, 0x0484,
}

const placeholder = '\U0010EEEE'

// Placeholders returns rows x cols placeholder cells for image id (1..255),
// the id carried as a 256-color foreground so it survives an ANSI256 profile.
// Every cell carries both diacritics: no reliance on kitty's left-neighbor
// inheritance, which a diffing renderer could break by skipping cells.
func Placeholders(id uint8, cols, rows int) string {
	if rows > len(diacritics) || cols > len(diacritics) {
		panic("placeholder area larger than embedded diacritics table")
	}
	var b strings.Builder
	for r := 0; r < rows; r++ {
		fmt.Fprintf(&b, "\x1b[38;5;%dm", id)
		for c := 0; c < cols; c++ {
			b.WriteRune(placeholder)
			b.WriteRune(diacritics[r])
			b.WriteRune(diacritics[c])
		}
		b.WriteString("\x1b[39m")
		if r < rows-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// Transmit returns the APC sequence(s) that upload png as image id and create
// a virtual placement of cols x rows cells (a=T,U=1), chunked at 4096 bytes.
func Transmit(id uint8, cols, rows int, png []byte) string {
	data := base64.StdEncoding.EncodeToString(png)
	var b strings.Builder
	first := true
	for len(data) > 0 {
		n := min(4096, len(data))
		chunk := data[:n]
		data = data[n:]
		more := 0
		if len(data) > 0 {
			more = 1
		}
		if first {
			fmt.Fprintf(&b, "\x1b_Ga=T,U=1,f=100,i=%d,c=%d,r=%d,q=2,m=%d;%s\x1b\\", id, cols, rows, more, chunk)
			first = false
		} else {
			fmt.Fprintf(&b, "\x1b_Gm=%d;%s\x1b\\", more, chunk)
		}
	}
	return b.String()
}
