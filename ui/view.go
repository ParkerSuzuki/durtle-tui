package ui

import (
	"fmt"
	"image"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/ParkerSuzuki/durtle-tui/dashboard"
	"github.com/ParkerSuzuki/durtle-tui/lessons"
	"github.com/ParkerSuzuki/durtle-tui/review"
)

const (
	// pagePadding is the blank columns left and right of every screen.
	pagePadding = 2
	// blockPadding is the blank rows above and below the characters.
	blockPadding = 3
	// charRow is the 1-based terminal row the characters sit on: the page's
	// top padding, the progress line, a blank line, the block's top padding.
	charRow = 1 + 1 + 1 + blockPadding + 1
)

var (
	typeColors = map[string]string{
		"radical":         "#2A9D8F",
		"kanji":           "#E9A23B",
		"vocabulary":      "#6A994E",
		"kana_vocabulary": "#6A994E",
	}
	meaningBar = lipgloss.NewStyle().Bold(true).Padding(0, 2).
			Foreground(lipgloss.Color("#1D1D1D")).Background(lipgloss.Color("#F4F1DE"))
	readingBar = lipgloss.NewStyle().Bold(true).Padding(0, 2).
			Foreground(lipgloss.Color("#F4F1DE")).Background(lipgloss.Color("#3D405B"))
	dim   = lipgloss.NewStyle().Faint(true)
	page  = lipgloss.NewStyle().Padding(1, pagePadding)
	title = lipgloss.NewStyle().Bold(true)
)

func (m Model) View() tea.View {
	var body string
	switch m.screen {
	case loading:
		body = "Syncing with WaniKani..."
	case onboarding:
		body = m.onboardingView()
	case reviewing:
		body = m.reviewView()
	case summary:
		body = m.summaryView()
	case home:
		body = m.homeView()
	case teaching, itemInfo:
		body = m.teachingView()
	case lessonSummary:
		body = m.lessonSummaryView()
	case settingsScreen:
		body = m.settingsView()
	case failed:
		what := "sync with WaniKani"
		switch m.what {
		case loadLessons:
			what = "load lessons"
		case loadReviews:
			what = "load reviews"
		case loadMistakes:
			what = "load recent mistakes"
		}
		body = fmt.Sprintf("Could not %s:\n\n%v\n\n%s", what, m.err, dim.Render("Enter to retry, Esc to quit"))
	}
	if m.quitting {
		body += "\n\n" + dim.Render(fmt.Sprintf("Finishing %d submission(s) before quitting... (again to quit now; they are saved)", m.inFlight))
	}
	v := tea.NewView(page.Render(body))
	v.AltScreen = true
	return v
}

func (m Model) onboardingView() string {
	lines := []string{
		title.Render("durtle-tui"),
		dim.Render("An unofficial third-party app for WaniKani."),
		"",
		"To start, create a personal access token at:",
		"  https://www.wanikani.com/settings/personal_access_tokens",
		"Tick the \"reviews:create\" permission, then paste the token here.",
		"",
		m.input.View(),
	}
	if m.err != nil {
		lines = append(lines, "", "That token did not work: "+m.err.Error())
	}
	return strings.Join(lines, "\n")
}

func (m Model) reviewView() string {
	item, part, _ := m.session.Current()
	done := len(m.session.Results())
	chars := m.itemBlock(item)
	bar := meaningBar
	if part == review.Reading {
		bar = readingBar
	}
	if w := m.innerWidth(); w > 0 {
		bar = bar.Width(w).Align(lipgloss.Center)
	}
	prompt := bar.Render(fmt.Sprintf("%s %s", typeLabel(item.Type), part))
	reviewHint := ":q dashboard   esc quit"
	if m.showingAnswer {
		reviewHint = "enter continue   f item info   esc quit"
	}
	return strings.Join([]string{
		dim.Render(fmt.Sprintf("%d / %d done   %s correct", done, m.session.Total(), percentCorrect(m.session.Results()))),
		"",
		chars,
		"",
		prompt,
		m.answerView(),
		"",
		m.feedback,
		"",
		dim.Render(reviewHint),
	}, "\n")
}

// itemBlock is the colored block showing an item's characters (or its
// half-block image), spanning the terminal with the content centered.
func (m Model) itemBlock(item review.Item) string {
	charStyle := lipgloss.NewStyle().Bold(true).Padding(blockPadding, 4).
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color(typeColors[item.Type]))
	w := m.innerWidth()
	if w == 0 {
		return charStyle.Render(item.Characters)
	}
	// Styles are values, so these calls change local copies, not shared styles.
	shown, style := item.Characters, charStyle
	switch {
	case item.Image != nil:
		// The picture fills the block: 10 rows, no top or bottom padding.
		// Kitty draws the sharp PNG over placeholder cells; elsewhere half-blocks.
		shown = strings.Join(halfBlocks(item.Image), "\n")
		if id, ok := m.imageIDs[item.AssignmentID]; ok {
			shown = Placeholders(id, radicalCols, radicalRows)
		}
		style = style.Padding(0, 4)
	case m.fitScale(shown, w) > 0:
		shown = " " // leave the row empty; bigCharsSeq draws on top
	}
	return style.Width(w).Align(lipgloss.Center).Render(shown)
}

// currentItem is the item on screen: the one being reviewed or quizzed, or
// the one being taught.
func (m Model) currentItem() (review.Item, bool) {
	switch m.screen {
	case reviewing:
		it, _, ok := m.session.Current()
		return it, ok
	case teaching, itemInfo:
		ps := m.pages()
		if len(ps) == 0 {
			return review.Item{}, false
		}
		return m.batch()[ps[m.page].lesson].Item, true
	}
	return review.Item{}, false
}

// bigGlyph is text kitty draws at scale over a colored area that the view
// left empty (see bigCharsSeq).
type bigGlyph struct {
	row         int // 1-based terminal row of the glyph's top
	left, width int // 1-based column and width of the colored area
	scale       int
	text, bg    string
}

// fitScale is the largest kitty text scale (3, then 2) at which text fits in
// width cells with a little margin, or 0 to draw it at normal size.
func (m Model) fitScale(text string, width int) int {
	if !m.bigText || text == "" { // "" is an image-only radical
		return 0
	}
	for _, s := range []int{3, 2} {
		if lipgloss.Width(text)*s <= width-4 {
			return s
		}
	}
	return 0
}

// bigGlyphs lists what the current screen wants drawn large: the review
// characters, or the dashboard's two counts.
func (m Model) bigGlyphs() []bigGlyph {
	w := m.innerWidth()
	switch m.screen {
	case reviewing, teaching, itemInfo:
		item, ok := m.currentItem()
		if !ok {
			return nil
		}
		if s := m.fitScale(item.Characters, w); s > 0 {
			return []bigGlyph{{row: charRow - (s-1)/2, left: pagePadding + 1, width: w,
				scale: s, text: item.Characters, bg: typeColors[item.Type]}}
		}
	case home:
		tw := tileWidth(w)
		var gs []bigGlyph
		for i, t := range m.tiles() {
			if s := m.fitScale(t.count, tw); s > 0 {
				gs = append(gs, bigGlyph{row: tileNumberRow - (s-1)/2, left: pagePadding + 1 + i*(tw+tileGap),
					width: tw, scale: s, text: t.count, bg: t.color})
			}
		}
		return gs
	}
	return nil
}

// bigKey identifies what is drawn large. The glyphs only need redrawing
// when this changes: a new item, new counts, a resize, or a new screen.
func (m Model) bigKey() string {
	gs := m.bigGlyphs()
	if len(gs) == 0 {
		return ""
	}
	return fmt.Sprintf("%v", gs)
}

// bigCharsSeq returns the raw escape sequence that draws bigGlyphs, or ""
// when there are none. It saves the cursor; for each glyph repaints the rows
// it covers with the area's color (so a shorter text leaves no ghosts) and
// writes the scaled text with OSC 66; then restores the cursor.
// Protocol: https://sw.kovidgoyal.net/kitty/text-sizing-protocol/
func (m Model) bigCharsSeq() string {
	gs := m.bigGlyphs()
	if len(gs) == 0 {
		return "" // not a screen with big text, or nothing fits
	}
	var sb strings.Builder
	sb.WriteString("\x1b7")
	for _, g := range gs {
		var r, gr, b uint8
		fmt.Sscanf(g.bg, "#%02x%02x%02x", &r, &gr, &b)
		bg := fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r, gr, b)
		for row := g.row; row < g.row+g.scale; row++ {
			fmt.Fprintf(&sb, "\x1b[%d;%dH%s%s", row, g.left, bg, strings.Repeat(" ", g.width))
		}
		col := g.left + (g.width-lipgloss.Width(g.text)*g.scale)/2
		fmt.Fprintf(&sb, "\x1b[%d;%dH%s\x1b[1;38;2;255;255;255m\x1b]66;s=%d;%s\x07\x1b[0m",
			g.row, col, bg, g.scale, g.text)
	}
	sb.WriteString("\x1b8")
	return sb.String()
}

// halfBlocks draws img with ▀ ▄ █, two pixel rows per text row. A pixel
// counts as ink when it is mostly opaque: radical images are dark strokes on
// a transparent background.
func halfBlocks(img image.Image) []string {
	b := img.Bounds()
	ink := func(x, y int) int {
		if y >= b.Max.Y {
			return 0
		}
		if _, _, _, a := img.At(x, y).RGBA(); a > 0x8000 {
			return 1
		}
		return 0
	}
	glyphs := [4]string{" ", "▄", "▀", "█"} // index: top*2 + bottom
	var rows []string
	for y := b.Min.Y; y < b.Max.Y; y += 2 {
		var sb strings.Builder
		for x := b.Min.X; x < b.Max.X; x++ {
			sb.WriteString(glyphs[ink(x, y)*2+ink(x, y+1)])
		}
		rows = append(rows, sb.String())
	}
	return rows
}

// answerView centers the typed answer under the prompt bar. The input is
// sized to its text (plus one cell for the cursor) so centering the field
// centers the text, and it grows from the middle as you type.
func (m Model) answerView() string {
	in := m.input // a copy: View must not change the model
	in.Prompt = ""
	in.SetWidth(lipgloss.Width(in.Value()) + 1)
	w := m.innerWidth()
	if w == 0 {
		return in.View()
	}
	return lipgloss.PlaceHorizontal(w, lipgloss.Center, in.View())
}

func (m Model) summaryView() string {
	results := m.session.Results()
	lines := []string{
		title.Render("Session complete"),
		fmt.Sprintf("%d reviewed, %s correct", len(results), percentCorrect(results)),
	}
	if m.practice {
		lines = []string{
			title.Render("Practice done"),
			fmt.Sprintf("%d items, %s correct. Nothing was sent to WaniKani.", len(results), percentCorrect(results)),
		}
	}
	var missed []string
	for _, r := range results {
		if r.Submission.IncorrectMeaning+r.Submission.IncorrectReading > 0 {
			name := r.Item.Characters
			if name == "" {
				name = "(" + r.Item.Meanings[0] + " radical)"
			}
			missed = append(missed, name)
		}
	}
	if len(missed) > 0 {
		lines = append(lines, "Missed: "+strings.Join(missed, "  "))
	}
	if m.inFlight > 0 {
		lines = append(lines, fmt.Sprintf("Sending %d...", m.inFlight))
	}
	if m.submitForbidden {
		lines = append(lines, "", "Your API token lacks the reviews:create permission, so answers were saved, not sent.",
			"Make a token with it at https://www.wanikani.com/settings/personal_access_tokens,",
			"then press t to enter it. The saved answers are sent after that.")
	} else if m.pending > 0 {
		lines = append(lines, fmt.Sprintf("%d saved offline; they will be sent next launch.", m.pending))
	}
	if m.rejected > 0 {
		lines = append(lines, fmt.Sprintf("%d refused by WaniKani (probably already reviewed elsewhere).", m.rejected))
	}
	if m.skipped > 0 {
		lines = append(lines, fmt.Sprintf("%d radicals with no character were skipped: they need rsvg-convert to draw (review them on the website).", m.skipped))
	}
	if m.lost > 0 {
		lines = append(lines, fmt.Sprintf("%d could not be sent or saved (%v). Redo them on the website.", m.lost, m.lostErr))
	}
	lines = append(lines, "", dim.Render("Enter for the dashboard, Esc to quit"))
	return strings.Join(lines, "\n")
}

func typeLabel(t string) string {
	switch t {
	case "radical":
		return "Radical"
	case "kanji":
		return "Kanji"
	}
	return "Vocabulary"
}

func percentCorrect(results []review.Result) string {
	if len(results) == 0 {
		return "0%"
	}
	clean := 0
	for _, r := range results {
		if r.Submission.IncorrectMeaning+r.Submission.IncorrectReading == 0 {
			clean++
		}
	}
	return fmt.Sprintf("%d%%", clean*100/len(results))
}

const (
	srsColor        = "#A8DADC"
	maxForecastRows = 8
	tileGap         = 2
	// tileRow is the 1-based terminal row where the count tiles start: the
	// page's top padding, the title line, a blank line.
	tileRow = 1 + 1 + 1 + 1
	// tileNumberRow is the row a count sits on: below the label and a blank
	// row, in the middle of the tile's three lower rows.
	tileNumberRow = tileRow + 3
)

func (m Model) homeView() string {
	d, p := m.dash, m.dash.Progress
	// The kanji line is the widest: an 18-cell label, the bar, the counts
	// ("  123 / 456"), and "   (411 needed to level up)" when there is room.
	// On narrow terminals the suffix goes, so nothing wraps.
	const labelW, countsW, neededW = 18 + 1, 11, 26
	needed := fmt.Sprintf("   (%d needed to level up)", p.KanjiNeeded)
	barW := m.innerWidth() - (labelW + countsW + neededW)
	if barW < 5 {
		needed, barW = "", m.innerWidth()-(labelW+countsW)
	}
	barW = max(barW, 5)
	lines := []string{
		title.Render("durtle-tui") + dim.Render(fmt.Sprintf("   Level %d", d.Level)),
		"",
		m.tilesView(),
		"",
		fmt.Sprintf("%-18s %s  %d / %d%s", fmt.Sprintf("Level %d kanji", d.Level),
			progressBar(p.KanjiPassed, p.Kanji, barW, typeColors["kanji"]), p.KanjiPassed, p.Kanji, needed),
		fmt.Sprintf("%-18s %s  %d / %d", fmt.Sprintf("Level %d radicals", d.Level),
			progressBar(p.RadicalsPassed, p.Radicals, barW, typeColors["radical"]), p.RadicalsPassed, p.Radicals),
		"",
	}
	lines = append(lines, accuracyLines(d.Today, d.Yesterday, barW)...)
	lines = append(lines, "", fmt.Sprintf("%-18s %d in the last 24 hours", "Recent mistakes", d.Mistakes))
	lines = append(lines, "", "Upcoming reviews")
	lines = append(lines, forecastLines(d.Forecast, barW)...)
	lines = append(lines, "")
	lines = append(lines, srsLines(d.SRS, barW)...)
	var hint []string
	if d.Reviews > 0 {
		hint = append(hint, "r reviews")
	}
	if d.LessonsToday > 0 {
		hint = append(hint, "l lessons")
	}
	if d.Mistakes > 0 {
		hint = append(hint, "m mistakes")
	}
	hint = append(hint, "s settings", "q quit")
	lines = append(lines, "", dim.Render(strings.Join(hint, "   ")))
	return strings.Join(lines, "\n")
}

// tile is one of the dashboard's count tiles.
type tile struct{ label, count, note, color string }

func (m Model) tiles() []tile {
	return []tile{
		{"Lessons", fmt.Sprint(m.dash.LessonsToday), fmt.Sprintf("of %d available", m.dash.Lessons), typeColors["radical"]},
		{"Reviews", fmt.Sprint(m.dash.Reviews), "", typeColors["kanji"]},
	}
}

// tileWidth splits the inner width between two tiles and the gap.
func tileWidth(inner int) int { return max((inner-tileGap)/2, 8) }

// tilesView draws the Lessons and Reviews tiles side by side: label on top,
// count on tileNumberRow. With kitty big text the count row is left empty
// and bigCharsSeq draws the count over it.
func (m Model) tilesView() string {
	tw := tileWidth(m.innerWidth())
	var parts []string
	for i, t := range m.tiles() {
		count := t.count
		if m.fitScale(count, tw) > 0 {
			count = " "
		}
		body := strings.Join([]string{t.label, "", "", count, "", t.note}, "\n")
		if i > 0 {
			parts = append(parts, strings.Repeat(" ", tileGap))
		}
		parts = append(parts, lipgloss.NewStyle().Bold(true).Width(tw).Align(lipgloss.Center).
			Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color(t.color)).Render(body))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

// progressBar draws done out of total as width cells: filled in color, the rest dim.
func progressBar(done, total, width int, color string) string {
	filled := 0
	if total > 0 {
		filled = min(done*width/total, width)
	}
	if done > 0 {
		filled = max(filled, 1) // any progress shows
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(strings.Repeat("█", filled)) +
		dim.Render(strings.Repeat("░", width-filled))
}

// scaledBar draws n as a bar where most fills width; any n > 0 gets at least one cell.
func scaledBar(n, most, width int) string {
	cells := 0
	if most > 0 {
		cells = n * width / most
	}
	if n > 0 {
		cells = max(cells, 1)
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(srsColor)).Render(strings.Repeat("█", cells))
}

func forecastLines(hours []dashboard.Hour, width int) []string {
	if len(hours) == 0 {
		return []string{dim.Render("  none in the next 24 hours")}
	}
	hours = hours[:min(len(hours), maxForecastRows)]
	most := 0
	for _, h := range hours {
		most = max(most, h.Added)
	}
	var lines []string
	for _, h := range hours {
		lines = append(lines, fmt.Sprintf("  %-9s %+5d %5d  %s",
			h.At.Local().Format("Mon 15:04"), h.Added, h.Total, scaledBar(h.Added, most, width)))
	}
	return lines
}

func srsLines(srs dashboard.SRS, width int) []string {
	most := 0
	for _, n := range srs {
		most = max(most, n)
	}
	var lines []string
	for i, n := range srs {
		lines = append(lines, fmt.Sprintf("%-12s %5d  %s", dashboard.StageNames[i], n, scaledBar(n, most, width)))
	}
	return lines
}

var tagStyles = map[string]lipgloss.Style{
	"radical":    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(typeColors["radical"])),
	"kanji":      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(typeColors["kanji"])),
	"vocabulary": lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(typeColors["vocabulary"])),
	"meaning":    lipgloss.NewStyle().Bold(true),
	"reading":    lipgloss.NewStyle().Bold(true),
}

// markup renders WaniKani mnemonic markup in our colors.
func markup(s string) string {
	var sb strings.Builder
	for _, sp := range lessons.Markup(s) {
		if st, ok := tagStyles[sp.Tag]; ok {
			sb.WriteString(st.Render(sp.Text))
		} else {
			sb.WriteString(sp.Text)
		}
	}
	return sb.String()
}

var readingLabels = map[string]string{"onyomi": "On'yomi", "kunyomi": "Kun'yomi", "nanori": "Nanori"}

// teachBody is the current teaching page's text, wrapped to the screen, and
// how many of its lines fit under the fixed parts of the page. With an
// unknown height (before the first resize) everything fits.
func (m Model) teachBody() (lines []string, avail int) {
	p := m.pages()[m.page]
	l := m.batch()[p.lesson]
	var body []string
	switch p.kind {
	case meaningPage:
		body = meaningLines(l)
	case readingPage:
		body = readingLines(l)
	case contextPage:
		body = contextLines(l)
	case allPage:
		body = allLines(l)
	}
	text := strings.Join(body, "\n")
	if w := m.innerWidth(); w > 0 {
		text = lipgloss.NewStyle().Width(w).Render(text)
	}
	lines = strings.Split(text, "\n")
	if m.height == 0 {
		return lines, len(lines)
	}
	// page padding (2), header, blank, block, blank, bar, blank, then after
	// the text: blank, hint.
	fixed := 2 + 1 + 1 + lipgloss.Height(m.itemBlock(l.Item)) + 1 + 1 + 1 + 1 + 1
	return lines, max(m.height-fixed, 2)
}

func (m Model) teachingView() string {
	ps := m.pages()
	p := ps[m.page]
	l := m.batch()[p.lesson]
	name, bar := "meaning", meaningBar
	switch p.kind {
	case readingPage:
		name, bar = "reading", readingBar
	case contextPage:
		name = "context"
	case allPage:
		name = "info"
	}
	if w := m.innerWidth(); w > 0 {
		bar = bar.Width(w).Align(lipgloss.Center)
	}
	header := fmt.Sprintf("Lesson %d of %d   page %d of %d", m.batchStart+p.lesson+1, len(m.plan.Lessons), m.page+1, len(ps))
	hint := "← → pages   ↑↓ scroll   enter next   q dashboard   esc quit"
	if m.screen == itemInfo {
		header = fmt.Sprintf("Item info   page %d of %d", m.page+1, len(ps))
		hint = "← → pages   ↑↓ scroll   f back"
	}
	// Bubble Tea drops the top of a frame taller than the screen, which would
	// push the block out from under the big glyph, so long text scrolls.
	lines, avail := m.teachBody()
	shown := lines
	if len(lines) > avail {
		start := min(m.scroll, len(lines)-avail)
		if start+avail < len(lines) {
			shown = append(slices.Clone(lines[start:start+avail-1]), dim.Render("↓ more (↑↓ scroll)"))
		} else {
			shown = lines[start:]
		}
	}
	return strings.Join([]string{
		dim.Render(header),
		"",
		m.itemBlock(l.Item),
		"",
		bar.Render(typeLabel(l.Type) + " " + name),
		"",
		strings.Join(shown, "\n"),
		"",
		dim.Render(hint),
	}, "\n")
}

func meaningLines(l lessons.Lesson) []string {
	lines := []string{title.Render(l.Meanings[0])}
	if len(l.Meanings) > 1 {
		lines = append(lines, dim.Render("also: "+strings.Join(l.Meanings[1:], ", ")))
	}
	if len(l.PartsOfSpeech) > 0 {
		lines = append(lines, dim.Render(strings.Join(l.PartsOfSpeech, ", ")))
	}
	if len(l.Components) > 0 {
		var parts []string
		for _, c := range l.Components {
			st := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(typeColors[c.Type]))
			parts = append(parts, strings.TrimSpace(st.Render(c.Characters)+" "+c.Meaning))
		}
		lines = append(lines, "", "Made of: "+strings.Join(parts, ", "))
	}
	if l.MeaningMnemonic != "" {
		lines = append(lines, "", markup(l.MeaningMnemonic))
	}
	if l.MeaningHint != "" {
		lines = append(lines, "", dim.Render("Hint: ")+markup(l.MeaningHint))
	}
	return lines
}

func readingLines(l lessons.Lesson) []string {
	var lines []string
	if len(l.KanjiReadings) > 0 {
		for _, kind := range []string{"onyomi", "kunyomi", "nanori"} {
			var rs []string
			for _, r := range l.KanjiReadings {
				if r.Type != kind {
					continue
				}
				if r.Accepted {
					rs = append(rs, title.Render(r.Reading))
				} else {
					rs = append(rs, dim.Render(r.Reading))
				}
			}
			if len(rs) > 0 {
				lines = append(lines, fmt.Sprintf("%-9s %s", readingLabels[kind], strings.Join(rs, "、")))
			}
		}
	} else {
		lines = append(lines, title.Render(strings.Join(l.Readings, "、")))
	}
	if l.ReadingMnemonic != "" {
		lines = append(lines, "", markup(l.ReadingMnemonic))
	}
	if l.ReadingHint != "" {
		lines = append(lines, "", dim.Render("Hint: ")+markup(l.ReadingHint))
	}
	return lines
}

// readingHeading titles the reading section of the one-page item info;
// openInfo scrolls to it after a missed reading.
const readingHeading = "Reading"

// allLines stacks every section for the one-page item info layout.
func allLines(l lessons.Lesson) []string {
	lines := append([]string{title.Render("Meaning")}, meaningLines(l)...)
	if l.HasReading() {
		lines = append(lines, "", title.Render(readingHeading))
		lines = append(lines, readingLines(l)...)
	}
	if len(l.Sentences) > 0 {
		lines = append(lines, "", title.Render("Context"))
		lines = append(lines, contextLines(l)...)
	}
	return lines
}

func contextLines(l lessons.Lesson) []string {
	var lines []string
	for _, s := range l.Sentences {
		lines = append(lines, s.Ja, dim.Render(s.En), "")
	}
	return lines
}

func (m Model) lessonSummaryView() string {
	lines := []string{title.Render("Lessons done")}
	if len(m.plan.Lessons) == 0 {
		lines = append(lines, "No lessons left today with your settings (s on the dashboard).")
	} else {
		lines = append(lines, fmt.Sprintf("%d started on WaniKani: they are in your reviews now.", m.started))
	}
	if m.inFlight > 0 {
		lines = append(lines, fmt.Sprintf("Starting %d...", m.inFlight))
	}
	switch {
	case m.startFailed > 0 && m.startForbidden():
		lines = append(lines, "",
			fmt.Sprintf("%d could not be started: your API token lacks the assignments:start permission.", m.startFailed),
			"Make a token with it at https://www.wanikani.com/settings/personal_access_tokens,",
			"then press t to enter it. The items stay in your lessons.")
	case m.startFailed > 0:
		lines = append(lines, "", fmt.Sprintf("%d could not be started (%v). They stay in your lessons.", m.startFailed, m.startErr))
	}
	lines = append(lines, "", dim.Render("Enter for the dashboard, Esc to quit"))
	return strings.Join(lines, "\n")
}

func (m Model) settingsView() string {
	s := m.settings
	check := func(on bool) string {
		if on {
			return "[x]"
		}
		return "[ ]"
	}
	layout := "pages"
	if s.InfoLayout == lessons.InfoSingle {
		layout = "one page"
	}
	rows := [settingRows][2]string{
		{"Daily lesson cap", fmt.Sprintf("‹ %d ›", s.DailyCap)},
		{"Order", fmt.Sprintf("‹ %s ›", s.Order)},
		{"Radicals", check(s.Types.Radical)},
		{"Kanji", check(s.Types.Kanji)},
		{"Vocabulary", check(s.Types.Vocabulary)},
		{"Batch size", fmt.Sprintf("‹ %d ›", s.BatchSize)},
		{"Item info", "‹ " + layout + " ›"},
	}
	lines := []string{title.Render("Settings"), ""}
	for i, r := range rows {
		cursor := "  "
		if i == m.settingRow {
			cursor = "> "
		}
		lines = append(lines, fmt.Sprintf("%s%-18s %s", cursor, r[0], r[1]))
	}
	if m.settingsNote != "" {
		lines = append(lines, "", m.settingsNote)
	}
	lines = append(lines, "", dim.Render("↑↓ choose   ←→ change   space toggle   esc save"))
	return strings.Join(lines, "\n")
}

// accuracyLines is the review accuracy panel: today and yesterday, and a
// gauge from 50% to 100% with the Learning Zone (85-95%) shaded.
func accuracyLines(today, yesterday dashboard.Answers, width int) []string {
	pct := func(a dashboard.Answers) string {
		if a.Total() == 0 {
			return "-"
		}
		return fmt.Sprintf("%.1f%%", a.Percent())
	}
	head := fmt.Sprintf("%-18s Today %s (%d)   Yesterday %s", "Correct answers", pct(today), today.Total(), pct(yesterday))
	zone := dashboard.ZoneOf(today)
	if zone == dashboard.NoData {
		return []string{head, fmt.Sprintf("%-18s %s", "", dim.Render("no reviews yet today"))}
	}
	cell := func(p float64) int { return min(max(int((p-50)/50*float64(width)), 0), width-1) }
	lo, hi, at := cell(dashboard.ZoneLow), cell(dashboard.ZoneHigh), cell(today.Percent())
	zoneStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(srsColor))
	var bar strings.Builder
	for i := range width {
		if i >= lo && i <= hi {
			bar.WriteString(zoneStyle.Render("▓"))
		} else {
			bar.WriteString(dim.Render("░"))
		}
	}
	label := map[dashboard.Zone]string{
		dashboard.InZone:    "in the Learning Zone (85-95%)",
		dashboard.BelowZone: "below the zone: go easier on lessons",
		dashboard.AboveZone: "above the zone: room for more lessons",
	}[zone]
	return []string{
		head,
		fmt.Sprintf("%-18s %s", "", bar.String()),
		fmt.Sprintf("%-18s %s", "", strings.Repeat(" ", at)+"▲"),
		fmt.Sprintf("%-18s %s", "", label),
	}
}
