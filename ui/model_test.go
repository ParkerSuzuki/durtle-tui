package ui

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/ParkerSuzuki/durtle-tui/dashboard"
	"github.com/ParkerSuzuki/durtle-tui/lessons"
	"github.com/ParkerSuzuki/durtle-tui/review"
	"github.com/ParkerSuzuki/durtle-tui/wanikani"
)

type fakeBackend struct {
	items      []review.Item
	loadErr    error
	submitted  []review.Submission
	submitErr  error
	dash       dashboard.Dashboard
	dashErr    error
	plan       lessons.Plan
	planErr    error
	startErr   error
	startedIDs []int
	settings   lessons.Settings
	saved      []lessons.Settings
	mistakes   []review.Item
}

func (f *fakeBackend) Mistakes(context.Context) ([]review.Item, int, error) {
	return f.mistakes, 0, nil
}

func (f *fakeBackend) Lessons(context.Context) (lessons.Plan, error) { return f.plan, f.planErr }
func (f *fakeBackend) StartLesson(_ context.Context, id int) error {
	f.startedIDs = append(f.startedIDs, id)
	return f.startErr
}
func (f *fakeBackend) Settings() (lessons.Settings, error) { return f.settings, nil }
func (f *fakeBackend) SaveSettings(s lessons.Settings) error {
	f.saved = append(f.saved, s)
	return nil
}

func (f *fakeBackend) Dashboard(context.Context) (dashboard.Dashboard, error) {
	return f.dash, f.dashErr
}

func (f *fakeBackend) Login(context.Context, string) error { return nil }
func (f *fakeBackend) Load(context.Context) ([]review.Item, int, error) {
	return f.items, 0, f.loadErr
}
func (f *fakeBackend) Submit(_ context.Context, s review.Submission) (bool, error) {
	f.submitted = append(f.submitted, s)
	return false, f.submitErr
}

var ground = review.Item{AssignmentID: 1, Type: "radical", Characters: "一", Meanings: []string{"Ground"}}

// step runs one Update and returns the new model and command.
func step(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

// typeAndEnter puts text in the input and presses Enter.
func typeAndEnter(t *testing.T, m Model, text string) (Model, tea.Cmd) {
	t.Helper()
	m.input.SetValue(text)
	return step(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
}

func TestNoTokenShowsOnboarding(t *testing.T) {
	m, _ := step(t, New(&fakeBackend{}, false), loadedMsg{err: wanikani.ErrUnauthorized})
	if m.screen != onboarding {
		t.Errorf("screen = %v, want onboarding", m.screen)
	}
}

func TestReviewFlowSubmits(t *testing.T) {
	fb := &fakeBackend{items: []review.Item{ground}}
	m, _ := step(t, New(fb, false), loadedMsg{items: fb.items})
	if m.screen != reviewing {
		t.Fatalf("screen = %v, want reviewing", m.screen)
	}
	m, _ = typeAndEnter(t, m, "sky")
	if !m.showingAnswer {
		t.Fatal("wrong answer should show the correct answers")
	}
	m, _ = step(t, m, tea.KeyPressMsg{Code: tea.KeyEnter}) // continue
	m, cmd := typeAndEnter(t, m, "ground")
	if m.screen != summary || m.inFlight != 1 || cmd == nil {
		t.Fatalf("screen=%v inFlight=%d cmd=%v", m.screen, m.inFlight, cmd)
	}
	m, _ = step(t, m, cmd()) // run the submit command, feed its message back
	if m.inFlight != 0 || len(fb.submitted) != 1 || fb.submitted[0].IncorrectMeaning != 1 {
		t.Errorf("inFlight=%d submitted=%+v", m.inFlight, fb.submitted)
	}
}

func TestQuitWaitsForSubmits(t *testing.T) {
	fb := &fakeBackend{items: []review.Item{ground}}
	m, _ := step(t, New(fb, false), loadedMsg{items: fb.items})
	m, submit := typeAndEnter(t, m, "ground")
	m, cmd := step(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd != nil || !m.quitting {
		t.Fatalf("quit with a submit in flight: cmd=%v quitting=%v; want to wait", cmd, m.quitting)
	}
	_, cmd = step(t, m, submit())
	if cmd == nil {
		t.Fatal("expected tea.Quit once the submit finished")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("final command should be tea.Quit")
	}
}

func TestOnboardingShowsFullPlaceholder(t *testing.T) {
	m, _ := step(t, New(&fakeBackend{}, false), loadedMsg{err: wanikani.ErrUnauthorized})
	// Strip color codes: the cursor highlight splits "p" from "aste".
	got := regexp.MustCompile("\x1b\\[[0-9;]*m").ReplaceAllString(m.View().Content, "")
	if !strings.Contains(got, "paste your API token") {
		t.Errorf("onboarding view is missing the placeholder:\n%s", got)
	}
}

// A submit that could neither be sent nor saved must be reported as lost,
// with the reason, not blamed on WaniKani.
func TestSaveFailureIsReportedAsLost(t *testing.T) {
	fb := &fakeBackend{items: []review.Item{ground}, submitErr: errors.New("disk full")}
	m, _ := step(t, New(fb, false), loadedMsg{items: fb.items})
	m, submit := typeAndEnter(t, m, "ground")
	m, _ = step(t, m, submit())
	if m.rejected != 0 {
		t.Errorf("rejected = %d, want 0: a local save failure is not a WaniKani refusal", m.rejected)
	}
	if got := m.View().Content; !strings.Contains(got, "disk full") {
		t.Errorf("summary should show the save error:\n%s", got)
	}
}

// Fixing a kana in the middle of a reading answer must convert in place and
// leave the cursor after the fix, not jump to the end.
func TestReadingEditMidAnswer(t *testing.T) {
	woman := review.Item{AssignmentID: 3, Type: "vocabulary", Characters: "女",
		Meanings: []string{"Woman"}, Readings: []string{"おんな"}}
	m, _ := step(t, New(&fakeBackend{}, false), loadedMsg{items: []review.Item{woman}})
	if _, part, _ := m.session.Current(); part == review.Meaning {
		m, _ = typeAndEnter(t, m, "woman")
	}
	m.input.SetValue("おんな")
	m.input.SetCursor(1)
	for _, k := range "ka" {
		m, _ = step(t, m, tea.KeyPressMsg{Code: k, Text: string(k)})
	}
	if got, pos := m.input.Value(), m.input.Position(); got != "おかんな" || pos != 2 {
		t.Errorf("value %q cursor %d, want %q cursor 2", got, pos, "おかんな")
	}
}

// The review screen spans the terminal and follows resizes.
func TestReviewFillsTerminalWidth(t *testing.T) {
	m, _ := step(t, New(&fakeBackend{}, false), loadedMsg{items: []review.Item{ground}})
	for _, width := range []int{80, 120, 60} {
		m, _ = step(t, m, tea.WindowSizeMsg{Width: width, Height: 30})
		lines := strings.Split(m.View().Content, "\n")
		var bar, chars string
		for _, l := range lines {
			if strings.Contains(l, "Radical meaning") {
				bar = l
			}
			if strings.Contains(l, "一") {
				chars = l
			}
		}
		for name, line := range map[string]string{"prompt bar": bar, "character block": chars} {
			if got := lipgloss.Width(line); got != width {
				t.Errorf("width %d: %s line is %d cells wide", width, name, got)
			}
		}
		if got := m.input.Width(); got < width-8 {
			t.Errorf("width %d: input is only %d wide", width, got)
		}
	}
}

func stripANSI(s string) string {
	return regexp.MustCompile("\x1b\\[[0-9;?]*[a-zA-Z]").ReplaceAllString(s, "")
}

// lineIndex returns the index of the first line containing sub, or -1.
func lineIndex(lines []string, sub string) int {
	for i, l := range lines {
		if strings.Contains(l, sub) {
			return i
		}
	}
	return -1
}

func TestCharacterBlockIsTall(t *testing.T) {
	m, _ := step(t, New(&fakeBackend{}, false), loadedMsg{items: []review.Item{ground}})
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 80, Height: 30})
	lines := strings.Split(m.View().Content, "\n")
	top, bar := lineIndex(lines, "done"), lineIndex(lines, "Radical meaning")
	if gap := bar - top - 1; gap < 9 {
		t.Errorf("only %d lines between progress and prompt; want a block at least 7 rows tall plus spacing", gap)
	}
}

func TestAnswerInputIsCentered(t *testing.T) {
	m, _ := step(t, New(&fakeBackend{}, false), loadedMsg{items: []review.Item{ground}})
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 80, Height: 30})
	m.input.SetValue("ground")
	lines := strings.Split(stripANSI(m.View().Content), "\n")
	line := lines[lineIndex(lines, "ground")]
	left := len(line) - len(strings.TrimLeft(line, " "))
	right := 80 - left - lipgloss.Width("ground")
	if d := left - right; d < -3 || d > 3 {
		t.Errorf("answer not centered: %d cells left, %d right\n%q", left, right, line)
	}
}

// bigModel is a review screen at width 80 with kitty big text on.
func bigModel(t *testing.T, item review.Item) Model {
	t.Helper()
	m, _ := step(t, New(&fakeBackend{}, true), loadedMsg{items: []review.Item{item}})
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 80, Height: 30})
	return m
}

func TestBigCharsSequence(t *testing.T) {
	// charRow must be where the normal layout actually puts the characters.
	plain, _ := step(t, New(&fakeBackend{}, false), loadedMsg{items: []review.Item{ground}})
	plain, _ = step(t, plain, tea.WindowSizeMsg{Width: 80, Height: 30})
	if got := lineIndex(strings.Split(plain.View().Content, "\n"), "一") + 1; got != charRow {
		t.Fatalf("characters are on row %d, charRow = %d", got, charRow)
	}

	m := bigModel(t, ground)
	seq := m.bigCharsSeq()
	if !strings.Contains(seq, "\x1b]66;s=3;一\x07") {
		t.Errorf("no 3x text sizing sequence in %q", seq)
	}
	col := pagePadding + (76-2*3)/2 + 1 // inner width 76, 一 is 2 cells wide at 1x
	if want := fmt.Sprintf("\x1b[%d;%dH", charRow-1, col); !strings.Contains(seq, want) {
		t.Errorf("glyph not placed at row %d col %d: %q", charRow-1, col, seq)
	}
	if strings.Contains(m.View().Content, "一") {
		t.Error("with big text on, the view must leave the block empty for the big glyph")
	}
}

func TestBigCharsFallBackWhenTooWide(t *testing.T) {
	long := review.Item{AssignmentID: 4, Type: "vocabulary", Characters: "一二三四五六七八九十一二三四",
		Meanings: []string{"x"}, Readings: []string{"x"}}
	m := bigModel(t, long) // 28 cells: too wide at 3x (84 cells), fits at 2x (56)
	if !strings.Contains(m.bigCharsSeq(), "s=2;") {
		t.Errorf("28-cell word in a 76-cell block should drop to 2x: %q", m.bigCharsSeq())
	}
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 40, Height: 30})
	if m.bigCharsSeq() != "" || !strings.Contains(m.View().Content, "一二三") {
		t.Error("when no scale fits, draw the characters normally")
	}
}

func TestBigCharsRedrawScheduled(t *testing.T) {
	m := bigModel(t, ground)
	if _, cmd := step(t, m, tea.WindowSizeMsg{Width: 90, Height: 30}); cmd == nil {
		t.Error("a resize must schedule a big-glyph redraw")
	}
	if _, cmd := step(t, m, drawBigMsg{}); cmd == nil {
		t.Error("drawBigMsg must write the glyph")
	}
	plain, _ := step(t, New(&fakeBackend{}, false), loadedMsg{items: []review.Item{ground}})
	if _, cmd := step(t, plain, tea.WindowSizeMsg{Width: 90, Height: 30}); cmd != nil {
		t.Error("without big text, a resize schedules nothing")
	}
}

// schedulesBigRedraw runs cmd (and each command in a batch) for up to 200ms
// and reports whether any of them produced drawBigMsg. Slower commands, like
// the cursor blink, are ignored.
func schedulesBigRedraw(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	found := make(chan bool, 64)
	var run func(tea.Cmd)
	run = func(c tea.Cmd) {
		go func() {
			switch msg := c().(type) {
			case drawBigMsg:
				found <- true
			case tea.BatchMsg:
				for _, sub := range msg {
					if sub != nil {
						run(sub)
					}
				}
			}
		}()
	}
	run(cmd)
	select {
	case <-found:
		return true
	case <-time.After(200 * time.Millisecond):
		return false
	}
}

// Typing never touches the character block, so it must not trigger a
// big-glyph redraw: each redraw makes kitty repaint three full rows plus the
// scaled text, which lagged typing on a throttled CPU.
func TestTypingDoesNotRedrawBigChars(t *testing.T) {
	kanji := review.Item{AssignmentID: 5, Type: "kanji", Characters: "大",
		Meanings: []string{"Big"}, Readings: []string{"たい"}}
	m, _ := step(t, New(&fakeBackend{}, true), loadedMsg{items: []review.Item{ground, kanji}})
	m, cmd := step(t, m, tea.WindowSizeMsg{Width: 80, Height: 30})
	if !schedulesBigRedraw(cmd) {
		t.Fatal("first frame must draw the big characters")
	}
	m, cmd = step(t, m, tea.KeyPressMsg{Code: 'a', Text: "a"})
	if schedulesBigRedraw(cmd) {
		t.Error("a keystroke scheduled a big-glyph redraw")
	}
	m.input.Reset()
	for i := 0; i < 2; i++ { // answer until the next item appears
		it, p, _ := m.session.Current()
		ans := it.Meanings[0]
		if p == review.Reading {
			ans = it.Readings[0]
		}
		before := it.AssignmentID
		m, cmd = typeAndEnter(t, m, ans)
		if next, _, ok := m.session.Current(); ok && next.AssignmentID != before {
			if !schedulesBigRedraw(cmd) {
				t.Error("a new item must redraw the big characters")
			}
			return
		}
	}
}

func TestHalfBlocks(t *testing.T) {
	// 3 wide, 4 tall: column 0 all ink, column 1 top half, column 2 bottom half.
	img := image.NewAlpha(image.Rect(0, 0, 3, 4))
	for y := 0; y < 4; y++ {
		img.SetAlpha(0, y, color.Alpha{255})
	}
	img.SetAlpha(1, 0, color.Alpha{255}) // top pixel of the first text row
	img.SetAlpha(2, 3, color.Alpha{255}) // bottom pixel of the second
	want := []string{"█▀ ", "█ ▄"}
	if got := halfBlocks(img); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("halfBlocks = %q, want %q", got, want)
	}
}

func TestImageRadicalReview(t *testing.T) {
	img := image.NewAlpha(image.Rect(0, 0, 20, 20))
	for x := 0; x < 20; x++ {
		img.SetAlpha(x, 8, color.Alpha{255}) // even row: top half of text row 4
	}
	beggar := review.Item{AssignmentID: 9, Type: "radical", Meanings: []string{"Beggar"}, Image: img}
	for _, big := range []bool{false, true} {
		m, _ := step(t, New(&fakeBackend{}, big), loadedMsg{items: []review.Item{beggar}})
		m, _ = step(t, m, tea.WindowSizeMsg{Width: 80, Height: 40})
		lines := strings.Split(stripANSI(m.View().Content), "\n")
		top, bar := lineIndex(lines, "done"), lineIndex(lines, "Radical meaning")
		if gap := bar - top - 1; gap != 10+2 {
			t.Errorf("big=%v: %d lines between progress and prompt, want the 10-row image plus 2 blank lines", big, gap)
		}
		if lineIndex(lines, "▀▀▀▀▀▀▀▀▀▀") < 0 {
			t.Errorf("big=%v: the image is not drawn", big)
		}
		if m.bigCharsSeq() != "" {
			t.Errorf("big=%v: image radicals must not use kitty big text", big)
		}
	}
}

func TestSkippedRadicalsNoted(t *testing.T) {
	m, _ := step(t, New(&fakeBackend{}, false), loadedMsg{items: nil, skipped: 2})
	if got := m.View().Content; !strings.Contains(got, "2 radicals") || !strings.Contains(got, "rsvg-convert") {
		t.Errorf("summary should say 2 radicals were skipped and why:\n%s", got)
	}
}

var sampleDash = dashboard.Dashboard{Level: 12, Lessons: 60, LessonsToday: 5, Reviews: 67,
	Today: dashboard.Answers{Correct: 853, Incorrect: 147}, Yesterday: dashboard.Answers{Correct: 848, Incorrect: 152},
	Forecast: []dashboard.Hour{{At: time.Date(2026, 9, 30, 15, 0, 0, 0, time.Local), Added: 12, Total: 79}},
	Progress: dashboard.Progress{Radicals: 10, RadicalsPassed: 9, Kanji: 33, KanjiPassed: 21, KanjiNeeded: 30},
	SRS:      dashboard.SRS{88, 143, 97, 201, 12}}

func TestDashboardShowsPanels(t *testing.T) {
	for _, width := range []int{100, 30} { // 30: narrow terminals must not panic
		m, _ := step(t, New(&fakeBackend{}, false), tea.WindowSizeMsg{Width: width, Height: 40})
		m, _ = step(t, m, dashboardMsg{d: sampleDash})
		got := stripANSI(m.View().Content)
		for _, want := range []string{"Level 12", "Lessons", "Reviews", "67", "21 / 33", "30 needed", "15:00", "+12", "79", "Apprentice", "143", "Burned"} {
			if want == "30 needed" && width < 61 {
				continue // narrow terminals drop the level-up suffix so nothing wraps
			}
			if !strings.Contains(got, want) {
				t.Errorf("width %d: dashboard missing %q", width, want)
			}
		}
	}
}

func TestEmptyDashboard(t *testing.T) {
	m, _ := step(t, New(&fakeBackend{}, false), tea.WindowSizeMsg{Width: 80, Height: 40})
	m, _ = step(t, m, dashboardMsg{d: dashboard.Dashboard{Level: 1}})
	if got := stripANSI(m.View().Content); !strings.Contains(got, "none in the next 24 hours") {
		t.Errorf("empty forecast not explained:\n%s", got)
	}
}

func TestStartReviewsFromDashboard(t *testing.T) {
	fb := &fakeBackend{items: []review.Item{ground}}
	m, _ := step(t, New(fb, false), dashboardMsg{d: dashboard.Dashboard{Reviews: 0}})
	if next, cmd := step(t, m, tea.KeyPressMsg{Code: 'r', Text: "r"}); cmd != nil || next.screen != home {
		t.Errorf("r with nothing due: screen %v, cmd %v; want to stay on the dashboard", next.screen, cmd)
	}
	m, _ = step(t, m, dashboardMsg{d: dashboard.Dashboard{Reviews: 1}})
	m, cmd := step(t, m, tea.KeyPressMsg{Code: 'r', Text: "r"})
	if m.screen != loading || cmd == nil {
		t.Fatalf("r with reviews due: screen %v, cmd %v", m.screen, cmd)
	}
	m, _ = step(t, m, cmd())
	if m.screen != reviewing {
		t.Fatalf("screen = %v, want reviewing", m.screen)
	}
	m, _ = step(t, m, tea.KeyPressMsg{Code: 'r', Text: "r"})
	if m.input.Value() != "r" {
		t.Errorf("r during reviews must be typed, got %q", m.input.Value())
	}
}

func TestSummaryReturnsToDashboard(t *testing.T) {
	fb := &fakeBackend{items: []review.Item{ground}, dash: dashboard.Dashboard{Level: 3}}
	m, _ := step(t, New(fb, false), loadedMsg{items: fb.items})
	m, submit := typeAndEnter(t, m, "ground") // summary, one submit in flight
	m, cmd := step(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.screen != loading || cmd == nil {
		t.Fatalf("Enter on summary: screen %v, cmd %v", m.screen, cmd)
	}
	m, _ = step(t, m, cmd())
	if m.screen != home || m.dash.Level != 3 {
		t.Fatalf("screen %v, level %d; want the dashboard", m.screen, m.dash.Level)
	}
	m, cmd = step(t, m, tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd != nil || !m.quitting {
		t.Fatal("q with a submit in flight must wait for it")
	}
	if _, cmd = step(t, m, submit()); cmd == nil {
		t.Fatal("expected quit once the submit finished")
	}
}

// No dashboard line may be wider than the terminal once there is room for it.
func TestDashboardFitsWidth(t *testing.T) {
	for _, width := range []int{60, 70, 80, 100, 140} {
		m, _ := step(t, New(&fakeBackend{}, false), tea.WindowSizeMsg{Width: width, Height: 40})
		m, _ = step(t, m, dashboardMsg{d: sampleDash})
		for _, line := range strings.Split(m.View().Content, "\n") {
			if w := lipgloss.Width(line); w > width {
				t.Errorf("width %d: line is %d cells: %q", width, w, stripANSI(line))
			}
		}
	}
}

// Left open, the dashboard reloads itself when the next forecast hour
// arrives, without leaving the home screen. Stale timers do nothing.
func TestDashboardRefreshesAtNextForecastHour(t *testing.T) {
	next := time.Now().Add(time.Hour).Truncate(time.Hour)
	fb := &fakeBackend{dash: dashboard.Dashboard{Level: 4}}
	m, cmd := step(t, New(fb, false), dashboardMsg{d: dashboard.Dashboard{Level: 3,
		Forecast: []dashboard.Hour{{At: next, Added: 5, Total: 5}}}})
	if cmd == nil || !m.refreshAt.Equal(next.Add(refreshSlack)) {
		t.Fatalf("refresh not scheduled: cmd %v, refreshAt %v", cmd, m.refreshAt)
	}
	if _, stale := step(t, m, refreshMsg{at: next.Add(-time.Hour)}); stale != nil {
		t.Error("an outdated refresh timer must not reload")
	}
	m, cmd = step(t, m, refreshMsg{at: m.refreshAt})
	if cmd == nil || m.screen != home {
		t.Fatalf("refresh: cmd %v, screen %v; want a background reload on the dashboard", cmd, m.screen)
	}
	if m, _ = step(t, m, cmd()); m.dash.Level != 4 {
		t.Errorf("dashboard not reloaded: level %d", m.dash.Level)
	}
}

// A new session must redraw the big glyph even when its first item matches
// the last one drawn in the previous session.
func TestNewSessionRedrawsBigChars(t *testing.T) {
	m := bigModel(t, ground)
	m, _ = typeAndEnter(t, m, "ground") // session over
	if _, cmd := step(t, m, loadedMsg{items: []review.Item{ground}}); !schedulesBigRedraw(cmd) {
		t.Error("the next session's first item was not drawn")
	}
}

// While waiting to quit, the dashboard ignores keys: starting a session
// then would let the pending quit cut it off.
func TestQuittingIgnoresStartReviews(t *testing.T) {
	fb := &fakeBackend{items: []review.Item{ground}, dash: dashboard.Dashboard{Reviews: 1}}
	m, _ := step(t, New(fb, false), loadedMsg{items: fb.items})
	m, _ = typeAndEnter(t, m, "ground") // one submit in flight
	m, cmd := step(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = step(t, m, cmd()) // dashboard
	m, _ = step(t, m, tea.KeyPressMsg{Code: 'q', Text: "q"})
	if m, cmd = step(t, m, tea.KeyPressMsg{Code: 'r', Text: "r"}); cmd != nil || m.screen != home {
		t.Errorf("r while quitting: screen %v, cmd %v; want it ignored", m.screen, cmd)
	}
}

// The counts sit in colored tiles. Without kitty they are in the view on
// tileNumberRow; with kitty the view leaves them out and they are drawn at 3x.
func TestCountTiles(t *testing.T) {
	plain, _ := step(t, New(&fakeBackend{}, false), tea.WindowSizeMsg{Width: 100, Height: 40})
	plain, _ = step(t, plain, dashboardMsg{d: sampleDash})
	lines := strings.Split(stripANSI(plain.View().Content), "\n")
	if got := lineIndex(lines, "67") + 1; got != tileNumberRow {
		t.Errorf("review count on row %d, want tileNumberRow %d", got, tileNumberRow)
	}
	if lineIndex(lines, "Lessons") < 0 || lineIndex(lines, "Reviews") < 0 {
		t.Error("tile labels missing")
	}

	big, _ := step(t, New(&fakeBackend{}, true), tea.WindowSizeMsg{Width: 100, Height: 40})
	big, cmd := step(t, big, dashboardMsg{d: sampleDash})
	if !schedulesBigRedraw(cmd) {
		t.Error("the dashboard must draw its big counts")
	}
	if strings.Contains(stripANSI(big.View().Content), "67") {
		t.Error("with big text on, the view must leave the count row empty")
	}
	seq := big.bigCharsSeq()
	for _, want := range []string{"\x1b]66;s=3;5\x07", "\x1b]66;s=3;67\x07"} {
		if !strings.Contains(seq, want) {
			t.Errorf("missing %q in %q", want, seq)
		}
	}
	if want := fmt.Sprintf("\x1b[%d;", tileNumberRow-1); !strings.Contains(seq, want) {
		t.Errorf("big counts not placed at row %d: %q", tileNumberRow-1, seq)
	}
}

// :q and :wq leave a session for the dashboard, from either answer box, and
// never submit the half-answered item.
func TestVimQuitToDashboard(t *testing.T) {
	woman := review.Item{AssignmentID: 3, Type: "vocabulary", Characters: "女",
		Meanings: []string{"Woman"}, Readings: []string{"おんな"}}
	for _, cmdText := range []string{":q", ":wq"} {
		fb := &fakeBackend{items: []review.Item{woman}, dash: dashboard.Dashboard{Level: 7}}
		m, _ := step(t, New(fb, false), loadedMsg{items: fb.items})
		if _, part, _ := m.session.Current(); part == review.Meaning {
			m, _ = typeAndEnter(t, m, "woman") // half-answered: reading still to go
		}
		for _, k := range cmdText { // typed key by key, through the kana conversion
			m, _ = step(t, m, tea.KeyPressMsg{Code: k, Text: string(k)})
		}
		m, cmd := step(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
		if m.screen != loading || cmd == nil {
			t.Fatalf("%s: screen %v, cmd %v; want to load the dashboard", cmdText, m.screen, cmd)
		}
		if m, _ = step(t, m, cmd()); m.screen != home || m.dash.Level != 7 {
			t.Errorf("%s: screen %v; want the dashboard", cmdText, m.screen)
		}
		if len(fb.submitted) != 0 {
			t.Errorf("%s: the half-answered item was submitted: %+v", cmdText, fb.submitted)
		}
	}
}

// Anything but exactly :q or :wq is an answer.
func TestNotQuiteVimQuitIsAnAnswer(t *testing.T) {
	m, _ := step(t, New(&fakeBackend{}, false), loadedMsg{items: []review.Item{ground}})
	m, _ = typeAndEnter(t, m, ":qq")
	if m.screen != reviewing || !m.showingAnswer {
		t.Errorf("\":qq\" should be graded as a wrong answer; screen %v", m.screen)
	}
}

func TestReviewShowsQuitHint(t *testing.T) {
	m, _ := step(t, New(&fakeBackend{}, false), loadedMsg{items: []review.Item{ground}})
	if got := stripANSI(m.View().Content); !strings.Contains(got, ":q dashboard") {
		t.Errorf("review screen has no :q hint:\n%s", got)
	}
}

func TestFailedScreenSaysWhatFailed(t *testing.T) {
	m, _ := step(t, New(&fakeBackend{}, false), dashboardMsg{err: errors.New("boom")})
	if got := stripANSI(m.View().Content); !strings.Contains(got, "Could not sync") {
		t.Errorf("dashboard failure should say sync failed:\n%s", got)
	}
}

func TestProgressBarShowsAnyProgress(t *testing.T) {
	if got := stripANSI(progressBar(1, 32, 20, "#E9A23B")); !strings.HasPrefix(got, "█") {
		t.Errorf("1 of 32 drew no filled cell: %q", got)
	}
}

var (
	lessonKanji = lessons.Lesson{
		Item: review.Item{AssignmentID: 21, Type: "kanji", Characters: "二",
			Meanings: []string{"Two"}, Readings: []string{"に"}, OtherReadings: []string{"ふた"}, ReadingKind: "on'yomi"},
		MeaningMnemonic: "two <radical>lines</radical>",
		KanjiReadings:   []lessons.KanjiReading{{Reading: "に", Type: "onyomi", Accepted: true}, {Reading: "ふた", Type: "kunyomi"}},
		Components:      []lessons.Component{{Characters: "一", Meaning: "Ground", Type: "radical"}},
	}
	lessonRadical = lessons.Lesson{Item: review.Item{AssignmentID: 11, Type: "radical", Characters: "一", Meanings: []string{"Ground"}}}
)

// lessonModel is the dashboard with lessons due, then l pressed and loaded.
func lessonModel(t *testing.T, fb *fakeBackend) Model {
	t.Helper()
	m, _ := step(t, New(fb, false), dashboardMsg{d: dashboard.Dashboard{LessonsToday: len(fb.plan.Lessons)}})
	m, cmd := step(t, m, tea.KeyPressMsg{Code: 'l', Text: "l"})
	if cmd == nil {
		t.Fatal("l did not load lessons")
	}
	m, _ = step(t, m, cmd())
	return m
}

func TestLessonKeyNeedsLessonsToday(t *testing.T) {
	m, _ := step(t, New(&fakeBackend{}, false), dashboardMsg{d: dashboard.Dashboard{Lessons: 60, LessonsToday: 0}})
	if next, cmd := step(t, m, tea.KeyPressMsg{Code: 'l', Text: "l"}); cmd != nil || next.screen != home {
		t.Errorf("l with the cap used up: screen %v, cmd %v", next.screen, cmd)
	}
}

func TestTeachingPages(t *testing.T) {
	fb := &fakeBackend{plan: lessons.Plan{Lessons: []lessons.Lesson{lessonRadical, lessonKanji}, BatchSize: 3}}
	m := lessonModel(t, fb)
	if m.screen != teaching || len(m.pages()) != 3 { // radical: meaning; kanji: meaning, reading
		t.Fatalf("screen %v, pages %d; want teaching with 3 pages", m.screen, len(m.pages()))
	}
	got := stripANSI(m.View().Content)
	if !strings.Contains(got, "Ground") {
		t.Errorf("first page should teach the radical:\n%s", got)
	}
	m, _ = step(t, m, tea.KeyPressMsg{Code: tea.KeyRight})
	if got := stripANSI(m.View().Content); !strings.Contains(got, "lines") || !strings.Contains(got, "一 Ground") {
		t.Errorf("kanji meaning page should show the mnemonic and components:\n%s", got)
	}
	m, _ = step(t, m, tea.KeyPressMsg{Code: tea.KeyRight})
	if got := stripANSI(m.View().Content); !strings.Contains(got, "On'yomi") || !strings.Contains(got, "ふた") {
		t.Errorf("reading page:\n%s", got)
	}
	m, _ = step(t, m, tea.KeyPressMsg{Code: tea.KeyLeft})
	if m.page != 1 {
		t.Errorf("left: page %d, want 1", m.page)
	}
	m, _ = step(t, m, tea.KeyPressMsg{Code: tea.KeyRight})
	m, _ = step(t, m, tea.KeyPressMsg{Code: tea.KeyEnter}) // last page: quiz
	if m.screen != reviewing || !m.lessonMode || m.session.Total() != 2 {
		t.Fatalf("screen %v lessonMode %v; want the quiz", m.screen, m.lessonMode)
	}
}

// answerAll answers every quiz question correctly and returns the commands.
func answerAll(t *testing.T, m Model) (Model, []tea.Cmd) {
	t.Helper()
	var cmds []tea.Cmd
	for m.screen == reviewing {
		it, part, _ := m.session.Current()
		ans := it.Meanings[0]
		if part == review.Reading {
			ans = it.Readings[0]
		}
		var cmd tea.Cmd
		m, cmd = typeAndEnter(t, m, ans)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return m, cmds
}

func TestLessonQuizStartsNotSubmits(t *testing.T) {
	fb := &fakeBackend{plan: lessons.Plan{Lessons: []lessons.Lesson{lessonRadical, lessonKanji, lessonRadical, lessonKanji}, BatchSize: 3}}
	fb.plan.Lessons[2].AssignmentID, fb.plan.Lessons[3].AssignmentID = 12, 22
	m := lessonModel(t, fb)
	for range m.pages() {
		m, _ = step(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	}
	m, cmds := answerAll(t, m)
	for _, c := range cmds {
		m, _ = step(t, m, c())
	}
	if m.screen != teaching || m.batchStart != 3 {
		t.Fatalf("after batch 1: screen %v batchStart %d; want teaching batch 2", m.screen, m.batchStart)
	}
	for range m.pages() {
		m, _ = step(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	}
	m, cmds = answerAll(t, m)
	for _, c := range cmds {
		m, _ = step(t, m, c())
	}
	if m.screen != lessonSummary || m.started != 4 || len(fb.startedIDs) != 4 || len(fb.submitted) != 0 {
		t.Errorf("screen %v started %d startedIDs %v submitted %v", m.screen, m.started, fb.startedIDs, fb.submitted)
	}
}

func TestStart403ExplainsToken(t *testing.T) {
	fb := &fakeBackend{plan: lessons.Plan{Lessons: []lessons.Lesson{lessonRadical}, BatchSize: 3},
		startErr: &wanikani.APIError{Status: 403}}
	m := lessonModel(t, fb)
	m, _ = step(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m, cmds := answerAll(t, m)
	m, _ = step(t, m, cmds[0]())
	got := stripANSI(m.View().Content)
	if !strings.Contains(got, "assignments:start") {
		t.Errorf("403 should explain the missing permission:\n%s", got)
	}
	if m, _ = step(t, m, tea.KeyPressMsg{Code: 't', Text: "t"}); m.screen != onboarding {
		t.Errorf("t should open token entry, screen %v", m.screen)
	}
}

func TestQuitFromTeachingStartsNothing(t *testing.T) {
	fb := &fakeBackend{plan: lessons.Plan{Lessons: []lessons.Lesson{lessonKanji}, BatchSize: 3}}
	m := lessonModel(t, fb)
	m, cmd := step(t, m, tea.KeyPressMsg{Code: 'q', Text: "q"})
	if m.screen != loading || cmd == nil || len(fb.startedIDs) != 0 {
		t.Errorf("q while teaching: screen %v, started %v", m.screen, fb.startedIDs)
	}
}

func TestQuitWaitsForLessonStart(t *testing.T) {
	fb := &fakeBackend{plan: lessons.Plan{Lessons: []lessons.Lesson{lessonRadical}, BatchSize: 3}}
	m := lessonModel(t, fb)
	m, _ = step(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m, cmds := answerAll(t, m) // lesson summary, one start in flight
	m, cmd := step(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd != nil || !m.quitting {
		t.Fatal("esc with a lesson start in flight must wait")
	}
	if _, cmd = step(t, m, cmds[0]()); cmd == nil {
		t.Fatal("expected quit once the start finished")
	}
}

func TestSettingsScreen(t *testing.T) {
	fb := &fakeBackend{settings: lessons.Default(3)}
	m, _ := step(t, New(fb, false), dashboardMsg{d: dashboard.Dashboard{}})
	m, cmd := step(t, m, tea.KeyPressMsg{Code: 's', Text: "s"})
	m, _ = step(t, m, cmd())
	if m.screen != settingsScreen {
		t.Fatalf("screen %v, want settings", m.screen)
	}
	key := func(code rune) { m, _ = step(t, m, tea.KeyPressMsg{Code: code}) }
	for range 9 {
		key(tea.KeyLeft) // daily cap 10 -> 1
	}
	key(tea.KeyDown)
	key(tea.KeyRight) // order -> interleaved
	key(tea.KeyDown)
	key(tea.KeySpace) // radicals off
	key(tea.KeyDown)
	key(tea.KeySpace) // kanji off
	key(tea.KeyDown)
	key(tea.KeySpace) // vocabulary: refused, it is the last type on
	if got := stripANSI(m.View().Content); !strings.Contains(got, "‹ 1 ›") || !strings.Contains(got, "interleaved") {
		t.Errorf("settings view:\n%s", got)
	}
	m, cmd = step(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m, _ = step(t, m, cmd()) // saved
	want := lessons.Settings{DailyCap: 1, Order: lessons.Interleaved, Types: lessons.Types{Vocabulary: true}, BatchSize: 3}
	if len(fb.saved) != 1 || fb.saved[0] != want {
		t.Errorf("saved %+v, want %+v", fb.saved, want)
	}
	if m.screen != loading {
		t.Errorf("after saving: screen %v, want the dashboard reloading", m.screen)
	}
}

// A long lesson must fit the screen (Bubble Tea drops the top lines of an
// oversized frame, which moves the block under the big glyph) and scroll.
func TestTeachingFitsScreenAndScrolls(t *testing.T) {
	long := lessonKanji
	long.MeaningMnemonic = strings.Repeat("A very long mnemonic sentence. ", 80) + "THE END"
	fb := &fakeBackend{plan: lessons.Plan{Lessons: []lessons.Lesson{long}, BatchSize: 3}}
	m := lessonModel(t, fb)
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 80, Height: 30})
	lines := strings.Split(stripANSI(m.View().Content), "\n")
	if len(lines) > 30 {
		t.Errorf("teaching view is %d lines on a 30-line screen", len(lines))
	}
	if got := lineIndex(lines, "Lesson 1 of 1"); got != 1 {
		t.Errorf("header on line %d, want 1 (the top must not scroll away)", got)
	}
	if strings.Contains(stripANSI(m.View().Content), "THE END") {
		t.Fatal("the end of a long mnemonic should start out of view")
	}
	for range 40 {
		m, _ = step(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if !strings.Contains(stripANSI(m.View().Content), "THE END") {
		t.Error("scrolling down should reach the end of the mnemonic")
	}
}

// Leaving lessons through token entry must not make a later review load
// failure look like a lesson failure.
func TestLessonLoadFlagDoesNotLeak(t *testing.T) {
	fb := &fakeBackend{planErr: wanikani.ErrUnauthorized, items: []review.Item{ground}}
	m, _ := step(t, New(fb, false), dashboardMsg{d: dashboard.Dashboard{LessonsToday: 1, Reviews: 1}})
	m, cmd := step(t, m, tea.KeyPressMsg{Code: 'l', Text: "l"})
	if m, _ = step(t, m, cmd()); m.screen != onboarding {
		t.Fatalf("screen %v, want onboarding after a 401", m.screen)
	}
	m, _ = step(t, m, loginMsg{})
	m, _ = step(t, m, dashboardMsg{d: dashboard.Dashboard{Reviews: 1}})
	m, _ = step(t, m, tea.KeyPressMsg{Code: 'r', Text: "r"})
	m, _ = step(t, m, loadedMsg{err: errors.New("boom")})
	if got := stripANSI(m.View().Content); !strings.Contains(got, "load reviews") {
		t.Errorf("failure text:\n%s", got)
	}
	if _, cmd = step(t, m, tea.KeyPressMsg{Code: tea.KeyEnter}); cmd == nil {
		t.Fatal("no retry")
	} else if _, ok := cmd().(loadedMsg); !ok {
		t.Error("retry must reload reviews, not lessons")
	}
}

// Without assignments:start, the first refused start ends the lesson
// session right away instead of after every batch.
func TestStart403StopsLessonsEarly(t *testing.T) {
	fb := &fakeBackend{plan: lessons.Plan{Lessons: []lessons.Lesson{lessonRadical, lessonKanji, lessonRadical, lessonKanji}, BatchSize: 3},
		startErr: &wanikani.APIError{Status: 403}}
	fb.plan.Lessons[2].AssignmentID, fb.plan.Lessons[3].AssignmentID = 12, 22
	m := lessonModel(t, fb)
	for range m.pages() {
		m, _ = step(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	}
	var start tea.Cmd
	for start == nil && m.screen == reviewing {
		it, part, _ := m.session.Current()
		ans := it.Meanings[0]
		if part == review.Reading {
			ans = it.Readings[0]
		}
		m, start = typeAndEnter(t, m, ans)
	}
	if m, _ = step(t, m, start()); m.screen != lessonSummary || m.lessonMode {
		t.Errorf("after a 403: screen %v, lessonMode %v; want the lesson summary now", m.screen, m.lessonMode)
	}
}

func TestAccuracyPanel(t *testing.T) {
	m, _ := step(t, New(&fakeBackend{}, false), tea.WindowSizeMsg{Width: 100, Height: 40})
	m, _ = step(t, m, dashboardMsg{d: sampleDash})
	got := stripANSI(m.View().Content)
	for _, want := range []string{"Correct answers", "Today 85.3%", "(1000)", "Yesterday 84.8%", "Learning Zone"} {
		if !strings.Contains(got, want) {
			t.Errorf("accuracy panel missing %q:\n%s", want, got)
		}
	}
	m, _ = step(t, m, dashboardMsg{d: dashboard.Dashboard{Level: 1}})
	if got := stripANSI(m.View().Content); !strings.Contains(got, "no reviews yet today") {
		t.Errorf("empty accuracy not explained:\n%s", got)
	}
}

// A second quit key stops waiting: answers still sending are already saved
// in pending.json and go out on the next launch.
func TestSecondQuitStopsWaiting(t *testing.T) {
	fb := &fakeBackend{items: []review.Item{ground}}
	m, _ := step(t, New(fb, false), loadedMsg{items: fb.items})
	m, _ = typeAndEnter(t, m, "ground") // one submit in flight
	m, cmd := step(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd != nil || !m.quitting {
		t.Fatal("first esc should wait")
	}
	if _, cmd = step(t, m, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}); cmd == nil {
		t.Fatal("second quit key should quit now")
	} else if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("want tea.Quit")
	}
	// Same from the dashboard, where keys are otherwise ignored while quitting.
	fb.dash = dashboard.Dashboard{}
	m, _ = step(t, New(fb, false), loadedMsg{items: fb.items})
	m, _ = typeAndEnter(t, m, "ground")
	m, cmd = step(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = step(t, m, cmd())
	m, _ = step(t, m, tea.KeyPressMsg{Code: 'q', Text: "q"})
	if _, cmd = step(t, m, tea.KeyPressMsg{Code: 'q', Text: "q"}); cmd == nil {
		t.Error("second q on the dashboard should quit now")
	}
}

// A review refused with 403 (token lacks reviews:create) is kept for later
// and explained, with t to enter a better token.
func TestReview403ExplainsToken(t *testing.T) {
	fb := &fakeBackend{items: []review.Item{ground}}
	m, _ := step(t, New(fb, false), loadedMsg{items: fb.items})
	m, _ = typeAndEnter(t, m, "ground") // summary
	m, _ = step(t, m, submittedMsg{pending: true, err: &wanikani.APIError{Status: 403}, run: m.run})
	got := stripANSI(m.View().Content)
	if m.rejected != 0 || m.pending != 1 || !strings.Contains(got, "reviews:create") {
		t.Errorf("rejected %d pending %d; summary:\n%s", m.rejected, m.pending, got)
	}
	if m, _ = step(t, m, tea.KeyPressMsg{Code: 't', Text: "t"}); m.screen != onboarding {
		t.Errorf("t should open token entry, screen %v", m.screen)
	}
}

func TestLessonStart401GoesToTokenEntry(t *testing.T) {
	fb := &fakeBackend{plan: lessons.Plan{Lessons: []lessons.Lesson{lessonRadical}, BatchSize: 3}, startErr: wanikani.ErrUnauthorized}
	m := lessonModel(t, fb)
	m, _ = step(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m, cmds := answerAll(t, m)
	if m, _ = step(t, m, cmds[0]()); m.screen != onboarding || m.inFlight != 0 {
		t.Errorf("after a 401 on start: screen %v inFlight %d; want token entry", m.screen, m.inFlight)
	}
}

func TestLessonSummaryWaitsForStarts(t *testing.T) {
	fb := &fakeBackend{plan: lessons.Plan{Lessons: []lessons.Lesson{lessonRadical}, BatchSize: 3}}
	m := lessonModel(t, fb)
	m, _ = step(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m, cmds := answerAll(t, m) // lesson summary, one start in flight
	if next, cmd := step(t, m, tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil || next.screen != lessonSummary {
		t.Errorf("Enter while starting: screen %v; want to wait", next.screen)
	}
	m, _ = step(t, m, cmds[0]())
	if m, _ = step(t, m, tea.KeyPressMsg{Code: tea.KeyEnter}); m.screen != loading {
		t.Errorf("Enter after starts landed: screen %v; want the dashboard loading", m.screen)
	}
}

// A result from an earlier session must not count toward the current one.
func TestLateResultIgnoredByNextSession(t *testing.T) {
	fb := &fakeBackend{items: []review.Item{ground}, submitErr: errors.New("disk full")}
	m, _ := step(t, New(fb, false), loadedMsg{items: fb.items})
	m, late := typeAndEnter(t, m, "ground")
	m, _ = step(t, m, loadedMsg{items: fb.items}) // next session starts
	if m, _ = step(t, m, late()); m.lost != 0 || m.inFlight != 0 {
		t.Errorf("late result: lost %d inFlight %d; want 0 and 0", m.lost, m.inFlight)
	}
}

// Only Enter starts the quiz: → on the last page is a stray key, not a choice.
func TestRightOnLastPageDoesNotStartQuiz(t *testing.T) {
	m := lessonModel(t, &fakeBackend{plan: lessons.Plan{Lessons: []lessons.Lesson{lessonRadical}, BatchSize: 3}})
	if m, _ = step(t, m, tea.KeyPressMsg{Code: tea.KeyRight}); m.screen != teaching {
		t.Fatalf("→ on the last page: screen %v, want to stay teaching", m.screen)
	}
	if m, _ = step(t, m, tea.KeyPressMsg{Code: tea.KeyEnter}); m.screen != reviewing {
		t.Errorf("Enter on the last page: screen %v, want the quiz", m.screen)
	}
}

// Space toggles, it does not count; refusing to turn off the last type says why.
func TestSettingsSpaceAndRefusal(t *testing.T) {
	fb := &fakeBackend{settings: lessons.Settings{DailyCap: 5, Order: lessons.Classic, Types: lessons.Types{Kanji: true}, BatchSize: 3}}
	m, _ := step(t, New(fb, false), dashboardMsg{d: dashboard.Dashboard{}})
	m, cmd := step(t, m, tea.KeyPressMsg{Code: 's', Text: "s"})
	m, _ = step(t, m, cmd())
	if m, _ = step(t, m, tea.KeyPressMsg{Code: tea.KeySpace}); m.settings.DailyCap != 5 {
		t.Errorf("space on the cap row changed it to %d", m.settings.DailyCap)
	}
	for range 3 {
		m, _ = step(t, m, tea.KeyPressMsg{Code: tea.KeyDown}) // kanji row
	}
	m, _ = step(t, m, tea.KeyPressMsg{Code: tea.KeySpace})
	if got := stripANSI(m.View().Content); !m.settings.Types.Kanji || !strings.Contains(got, "At least one type") {
		t.Errorf("turning off the last type should be refused with a reason:\n%s", got)
	}
}

// In kitty, an image-only radical is uploaded once and drawn with Unicode
// placeholders inside the block; elsewhere it stays half-block art.
func TestKittyRadicalImage(t *testing.T) {
	img := image.NewAlpha(image.Rect(0, 0, 20, 20))
	beggar := review.Item{AssignmentID: 9, Type: "radical", Meanings: []string{"Beggar"}, Image: img, PNG: []byte("png-bytes")}

	m, _ := step(t, New(&fakeBackend{}, true), tea.WindowSizeMsg{Width: 80, Height: 40})
	m, cmd := step(t, m, loadedMsg{items: []review.Item{beggar}})
	if !sendsRaw(cmd, "\x1b_Ga=T,U=1,f=100,i=") {
		t.Error("the radical's PNG was not uploaded to kitty")
	}
	view := m.View().Content
	if !strings.Contains(view, "\U0010EEEE") {
		t.Error("kitty view should draw the radical with image placeholders")
	}
	if !strings.Contains(view, "\x1b[38;5;1m\U0010EEEE") {
		t.Error("the image id (foreground color 1) must survive the block's styling")
	}
	for _, line := range strings.Split(view, "\n") {
		if w := lipgloss.Width(line); w > 80 {
			t.Errorf("line is %d cells wide on an 80-column screen", w)
		}
	}

	plain, _ := step(t, New(&fakeBackend{}, false), tea.WindowSizeMsg{Width: 80, Height: 40})
	plain, cmd = step(t, plain, loadedMsg{items: []review.Item{beggar}})
	if sendsRaw(cmd, "\x1b_G") || strings.Contains(plain.View().Content, "\U0010EEEE") {
		t.Error("outside kitty: no upload, no placeholders")
	}
}

// sendsRaw runs cmd (and batches) briefly and reports whether any raw
// terminal output starts with prefix.
func sendsRaw(cmd tea.Cmd, prefix string) bool {
	if cmd == nil {
		return false
	}
	found := make(chan bool, 64)
	var run func(tea.Cmd)
	run = func(c tea.Cmd) {
		go func() {
			switch msg := c().(type) {
			case tea.RawMsg:
				if s, ok := msg.Msg.(string); ok && strings.HasPrefix(s, prefix) {
					found <- true
				}
			case tea.BatchMsg:
				for _, sub := range msg {
					if sub != nil {
						run(sub)
					}
				}
			}
		}()
	}
	run(cmd)
	select {
	case <-found:
		return true
	case <-time.After(200 * time.Millisecond):
		return false
	}
}

// Recent mistakes: m on the dashboard starts a practice quiz that sends
// nothing to WaniKani and returns to the dashboard.
func TestPracticeRecentMistakes(t *testing.T) {
	fb := &fakeBackend{mistakes: []review.Item{ground}}
	m, _ := step(t, New(fb, false), tea.WindowSizeMsg{Width: 100, Height: 40})
	m, _ = step(t, m, dashboardMsg{d: dashboard.Dashboard{Mistakes: 0}})
	if next, cmd := step(t, m, tea.KeyPressMsg{Code: 'm', Text: "m"}); cmd != nil || next.screen != home {
		t.Errorf("m with no mistakes: screen %v, cmd %v", next.screen, cmd)
	}
	m, _ = step(t, m, dashboardMsg{d: dashboard.Dashboard{Mistakes: 1}})
	if got := stripANSI(m.View().Content); !strings.Contains(got, "Recent mistakes") || !strings.Contains(got, "m mistakes") {
		t.Errorf("dashboard should show the mistakes line and key:\n%s", got)
	}
	m, cmd := step(t, m, tea.KeyPressMsg{Code: 'm', Text: "m"})
	if m.screen != loading || cmd == nil {
		t.Fatalf("m: screen %v, cmd %v", m.screen, cmd)
	}
	if m, _ = step(t, m, cmd()); m.screen != reviewing || !m.practice {
		t.Fatalf("screen %v practice %v; want a practice quiz", m.screen, m.practice)
	}
	m, _ = typeAndEnter(t, m, "sky") // wrong
	m, _ = step(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m, cmd = typeAndEnter(t, m, "ground")
	if cmd != nil || m.inFlight != 0 || len(fb.submitted) != 0 {
		t.Errorf("practice sent something: cmd %v, inFlight %d, submitted %v", cmd, m.inFlight, fb.submitted)
	}
	got := stripANSI(m.View().Content)
	if m.screen != summary || !strings.Contains(got, "Practice done") || !strings.Contains(got, "Nothing was sent") {
		t.Errorf("practice summary:\n%s", got)
	}
	if m, cmd = step(t, m, tea.KeyPressMsg{Code: tea.KeyEnter}); m.screen != loading || cmd == nil || m.practice {
		t.Errorf("Enter after practice: screen %v practice %v; want the dashboard loading", m.screen, m.practice)
	}
}
