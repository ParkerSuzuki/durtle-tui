// Package ui is durtle-tui's Bubble Tea interface.
package ui

import (
	"context"
	"errors"
	"math/rand/v2"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/ParkerSuzuki/durtle-tui/review"
	"github.com/ParkerSuzuki/durtle-tui/wanikani"
)

// Backend is everything the UI needs from the outside world.
type Backend interface {
	Login(ctx context.Context, token string) error
	Load(ctx context.Context) ([]review.Item, error)
	Submit(ctx context.Context, s review.Submission) (pending bool, err error)
}

type screen int

const (
	loading screen = iota
	onboarding
	reviewing
	summary
	failed
)

type (
	loadedMsg struct {
		items []review.Item
		err   error
	}
	loginMsg     struct{ err error }
	submittedMsg struct {
		pending bool
		err     error
	}
)

// Model is the whole UI state. Update returns a changed copy.
type Model struct {
	backend       Backend
	screen        screen
	input         textinput.Model
	session       *review.Session
	feedback      string
	showingAnswer bool // a wrong answer is on screen; Enter continues
	err           error
	inFlight      int // submits not yet finished
	pending       int // saved to retry next launch
	rejected      int // refused by WaniKani
	quitting      bool
	width         int
}

func New(b Backend) Model {
	in := textinput.New()
	in.Focus()
	return Model{backend: b, screen: loading, input: in}
}

func (m Model) Init() tea.Cmd { return m.load() }

func (m Model) load() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		items, err := m.backend.Load(ctx)
		return loadedMsg{items, err}
	}
}

func (m Model) login(token string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return loginMsg{m.backend.Login(ctx, token)}
	}
}

func (m Model) submit(s review.Submission) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		pending, err := m.backend.Submit(ctx, s)
		return submittedMsg{pending, err}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil
	case loadedMsg:
		return m.loaded(msg)
	case loginMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.screen = loading
		m.input.EchoMode = textinput.EchoNormal
		m.input.Placeholder = ""
		m.input.Reset()
		return m, m.load()
	case submittedMsg:
		m.inFlight--
		switch {
		case msg.err != nil:
			m.rejected++
		case msg.pending:
			m.pending++
		}
		if m.quitting && m.inFlight == 0 {
			return m, tea.Quit
		}
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			return m.quit()
		case "enter":
			return m.enter()
		}
		if m.showingAnswer {
			return m, nil // ignore typing while the correction is shown
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.screen == reviewing {
		if _, part, ok := m.session.Current(); ok && part == review.Reading {
			m.input.SetValue(review.ToHiragana(m.input.Value(), false))
			m.input.CursorEnd()
		}
	}
	return m, cmd
}

func (m Model) loaded(msg loadedMsg) (tea.Model, tea.Cmd) {
	switch {
	case errors.Is(msg.err, wanikani.ErrUnauthorized):
		m.screen = onboarding
		m.input.EchoMode = textinput.EchoPassword
		m.input.Placeholder = "paste your API token"
		m.input.Reset()
		return m, nil
	case msg.err != nil:
		m.screen = failed
		m.err = msg.err
		return m, nil
	}
	m.session = review.NewSession(msg.items, rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())))
	m.screen = reviewing
	if _, _, ok := m.session.Current(); !ok {
		m.screen = summary
	}
	return m, nil
}

func (m Model) enter() (tea.Model, tea.Cmd) {
	switch m.screen {
	case onboarding:
		if tok := strings.TrimSpace(m.input.Value()); tok != "" {
			m.err = nil
			return m, m.login(tok)
		}
	case failed:
		m.screen = loading
		m.err = nil
		return m, m.load()
	case summary:
		return m.quit()
	case reviewing:
		return m.answer()
	}
	return m, nil
}

func (m Model) answer() (tea.Model, tea.Cmd) {
	if m.showingAnswer {
		m.showingAnswer = false
		m.feedback = ""
		m.input.Reset()
		return m, nil
	}
	value := strings.TrimSpace(m.input.Value())
	if value == "" {
		return m, nil
	}
	item, part, _ := m.session.Current()
	grade, sub := m.session.Answer(value)
	switch grade.Verdict {
	case review.Warn:
		m.feedback = grade.Hint
		return m, nil
	case review.Wrong:
		m.showingAnswer = true
		m.feedback = "Wrong. Accepted: " + strings.Join(accepted(item, part), ", ")
		return m, nil
	case review.CorrectTypo:
		m.feedback = "Correct (typo accepted): " + accepted(item, part)[0]
	case review.Correct:
		m.feedback = "Correct"
	}
	m.input.Reset()
	var cmd tea.Cmd
	if sub != nil {
		m.inFlight++
		cmd = m.submit(*sub)
	}
	if _, _, ok := m.session.Current(); !ok {
		m.screen = summary
	}
	return m, cmd
}

func accepted(it review.Item, p review.Part) []string {
	if p == review.Reading {
		return it.Readings
	}
	return it.Meanings
}

func (m Model) quit() (tea.Model, tea.Cmd) {
	if m.inFlight == 0 {
		return m, tea.Quit
	}
	m.quitting = true
	return m, nil
}
