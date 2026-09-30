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
	"github.com/ParkerSuzuki/durtle-tui/dashboard"
	"github.com/ParkerSuzuki/durtle-tui/review"
	"github.com/ParkerSuzuki/durtle-tui/wanikani"
)

// Backend is everything the UI needs from the outside world.
type Backend interface {
	Login(ctx context.Context, token string) error
	Load(ctx context.Context) (items []review.Item, skipped int, err error)
	Submit(ctx context.Context, s review.Submission) (pending bool, err error)
	Dashboard(ctx context.Context) (dashboard.Dashboard, error)
}

// inputWidth fits a WaniKani token (36 characters) with room to spare.
const inputWidth = 48

type screen int

const (
	loading screen = iota
	onboarding
	reviewing
	summary
	failed
	home // the dashboard
)

type (
	loadedMsg struct {
		items   []review.Item
		skipped int // image-only radicals that could not be drawn
		err     error
	}
	loginMsg     struct{ err error }
	dashboardMsg struct {
		d   dashboard.Dashboard
		err error
	}
	drawBigMsg   struct{}
	submittedMsg struct {
		pending bool
		err     error
	}
)

// Model is the whole UI state. Update returns a changed copy.
type Model struct {
	backend        Backend
	screen         screen
	input          textinput.Model
	session        *review.Session
	feedback       string
	showingAnswer  bool // a wrong answer is on screen; Enter continues
	err            error
	inFlight       int   // submits not yet finished
	pending        int   // saved to retry next launch
	rejected       int   // refused by WaniKani
	lost           int   // could not be sent or saved
	lostErr        error // why the last one was lost
	skipped        int   // image-only radicals left for the website
	quitting       bool
	width          int
	bigText        bool   // draw characters with kitty's text sizing protocol
	bigDrawn       string // bigKey of the last big-glyph draw
	dash           dashboard.Dashboard
	loadingReviews bool // which load a retry repeats: reviews or the dashboard
}

// New builds the UI. bigText turns on large characters, which only kitty
// can draw (see bigCharsSeq); other terminals get the normal layout.
func New(b Backend, bigText bool) Model {
	in := textinput.New()
	in.SetWidth(inputWidth) // without a width, the placeholder is cut to one character
	in.Focus()
	return Model{backend: b, screen: loading, input: in, bigText: bigText}
}

func (m Model) Init() tea.Cmd { return m.loadDashboard() }

func (m Model) loadDashboard() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		d, err := m.backend.Dashboard(ctx)
		return dashboardMsg{d, err}
	}
}

// loadFailed sends a load error to onboarding (bad or missing token) or to
// the failed screen.
func (m Model) loadFailed(err error) (tea.Model, tea.Cmd) {
	if errors.Is(err, wanikani.ErrUnauthorized) {
		m.screen = onboarding
		m.input.EchoMode = textinput.EchoPassword
		m.input.Placeholder = "paste your API token"
		m.input.Reset()
		return m, nil
	}
	m.screen = failed
	m.err = err
	return m, nil
}

func (m Model) startReviews() (tea.Model, tea.Cmd) {
	if m.dash.Reviews == 0 {
		return m, nil
	}
	m.screen, m.loadingReviews = loading, true
	return m, m.load()
}

func (m Model) load() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		items, skipped, err := m.backend.Load(ctx)
		return loadedMsg{items, skipped, err}
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

// bigTextDelay is how long after a frame the big characters are drawn.
const bigTextDelay = 40 * time.Millisecond

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(drawBigMsg); ok {
		if seq := m.bigCharsSeq(); seq != "" {
			return m, tea.Raw(seq)
		}
		return m, nil
	}
	next, cmd := m.update(msg)
	nm := next.(Model)
	if key := nm.bigKey(); nm.bigText && key != "" && key != nm.bigDrawn {
		// ponytail: timing hack. Bubble Tea's renderer drops kitty's text
		// sizing escape, so the big characters are written straight to the
		// terminal a moment after the frame that changed the block. Only
		// then: the renderer leaves unchanged rows alone, and redrawing on
		// every keystroke made kitty repaint the glyph constantly and lag
		// typing. If the renderer ever repaints the block on its own, the
		// glyph vanishes until the next item or resize.
		// Upgrade path: renderer support for OSC 66, if Bubble Tea adds it.
		nm.bigDrawn = key
		redraw := tea.Tick(bigTextDelay, func(time.Time) tea.Msg { return drawBigMsg{} })
		cmd = tea.Batch(cmd, redraw)
	}
	return nm, cmd
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		// The text area is the inner width minus the "> " prompt and the cursor.
		m.input.SetWidth(max(m.innerWidth()-3, 10))
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
		m.loadingReviews = false
		return m, m.loadDashboard()
	case dashboardMsg:
		if msg.err != nil {
			return m.loadFailed(msg.err)
		}
		m.dash = msg.d
		m.screen = home
		return m, nil
	case submittedMsg:
		m.inFlight--
		var apiErr *wanikani.APIError
		switch {
		case errors.As(msg.err, &apiErr):
			m.rejected++
		case msg.err != nil:
			m.lost++
			m.lostErr = msg.err
		case msg.pending:
			m.pending++
		}
		if m.quitting && m.inFlight == 0 {
			return m, tea.Quit
		}
		return m, nil
	case tea.KeyPressMsg:
		if m.screen == home {
			switch msg.String() {
			case "r", "enter":
				return m.startReviews()
			case "q", "esc", "ctrl+c":
				return m.quit()
			}
			return m, nil
		}
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
			m.input = convertBeforeCursor(m.input)
		}
	}
	return m, cmd
}

func (m Model) loaded(msg loadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m.loadFailed(msg.err)
	}
	m.pending, m.rejected, m.lost, m.lostErr = 0, 0, 0, nil // per-session counts
	m.skipped = msg.skipped
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
		m.screen, m.err = loading, nil
		if m.loadingReviews {
			return m, m.load()
		}
		return m, m.loadDashboard()
	case summary:
		m.screen, m.loadingReviews = loading, false
		return m, m.loadDashboard()
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

// convertBeforeCursor turns romaji to the left of the cursor into kana and
// keeps the cursor just after it, so editing mid-answer works. Text after
// the cursor is already converted and is left alone.
func convertBeforeCursor(in textinput.Model) textinput.Model {
	runes := []rune(in.Value())
	pos := min(in.Position(), len(runes))
	typed := string(runes[:pos])
	kana := review.ToHiragana(typed, false)
	if kana == typed {
		return in
	}
	in.SetValue(kana + string(runes[pos:]))
	in.SetCursor(len([]rune(kana)))
	return in
}

func accepted(it review.Item, p review.Part) []string {
	if p == review.Reading {
		return it.Readings
	}
	return it.Meanings
}

// innerWidth is the terminal width minus the page padding, or 0 before the
// first resize message arrives.
func (m Model) innerWidth() int { return max(m.width-2*pagePadding, 0) }

func (m Model) quit() (tea.Model, tea.Cmd) {
	if m.inFlight == 0 {
		return m, tea.Quit
	}
	m.quitting = true
	return m, nil
}
