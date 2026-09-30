package ui

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
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
	m, _ := step(t, New(&fakeBackend{}), loadedMsg{err: wanikani.ErrUnauthorized})
	if m.screen != onboarding {
		t.Errorf("screen = %v, want onboarding", m.screen)
	}
}

func TestReviewFlowSubmits(t *testing.T) {
	fb := &fakeBackend{items: []review.Item{ground}}
	m, _ := step(t, New(fb), loadedMsg{items: fb.items})
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
	m, _ := step(t, New(fb), loadedMsg{items: fb.items})
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
	m, _ := step(t, New(&fakeBackend{}), loadedMsg{err: wanikani.ErrUnauthorized})
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
	m, _ := step(t, New(fb), loadedMsg{items: fb.items})
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
	m, _ := step(t, New(&fakeBackend{}), loadedMsg{items: []review.Item{woman}})
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
