package ui

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/ParkerSuzuki/durtle-tui/review"
	"github.com/ParkerSuzuki/durtle-tui/wanikani"
)

type fakeBackend struct {
	items     []review.Item
	loadErr   error
	submitted []review.Submission
	submitErr error
}

func (f *fakeBackend) Login(context.Context, string) error { return nil }
func (f *fakeBackend) Load(context.Context) ([]review.Item, error) {
	return f.items, f.loadErr
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
