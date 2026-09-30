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
	"github.com/ParkerSuzuki/durtle-tui/lessons"
	"github.com/ParkerSuzuki/durtle-tui/review"
	"github.com/ParkerSuzuki/durtle-tui/wanikani"
)

// Backend is everything the UI needs from the outside world.
type Backend interface {
	Login(ctx context.Context, token string) error
	Load(ctx context.Context) (items []review.Item, skipped int, err error)
	Submit(ctx context.Context, s review.Submission) (pending bool, err error)
	Dashboard(ctx context.Context) (dashboard.Dashboard, error)
	Lessons(ctx context.Context) (lessons.Plan, error)
	StartLesson(ctx context.Context, assignmentID int) error
	Settings() (lessons.Settings, error)
	SaveSettings(s lessons.Settings) error
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
	teaching
	lessonSummary
	settingsScreen
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
	drawBigMsg struct{}
	refreshMsg struct{ at time.Time } // time to reload the dashboard
	lessonsMsg struct {
		plan lessons.Plan
		err  error
	}
	startedMsg struct {
		err error
		run int
	}
	settingsMsg struct {
		s   lessons.Settings
		err error
	}
	savedMsg     struct{ err error }
	submittedMsg struct {
		pending bool
		err     error
		run     int // the session it belongs to; later sessions ignore it
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
	loadingReviews bool      // which load a retry repeats: reviews or the dashboard
	refreshAt      time.Time // when the dashboard reloads itself; stale timers are ignored

	plan            lessons.Plan
	batchStart      int  // index of the first lesson in the current batch
	page            int  // teaching page within the batch
	lessonMode      bool // the reviewing screen is a lesson quiz
	loadingLessons  bool
	started         int // lessons started on WaniKani this session
	startFailed     int
	startErr        error
	settings        lessons.Settings // being edited on the settings screen
	settingRow      int
	height          int  // terminal rows, 0 before the first resize
	scroll          int  // first visible text line on a teaching page
	run             int  // review or lesson session number, for late results
	submitForbidden bool // a review was refused for lack of reviews:create
}

// refreshSlack is how long after a forecast hour the dashboard reloads,
// giving WaniKani a moment to make those reviews available.
const refreshSlack = 5 * time.Second

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

// toDashboard ends the review screens and reloads the dashboard.
func (m Model) toDashboard() (tea.Model, tea.Cmd) {
	m.screen, m.loadingReviews = loading, false
	m.lessonMode, m.loadingLessons = false, false
	m.feedback = ""
	m.input.Reset()
	return m, m.loadDashboard()
}

func (m Model) startReviews() (tea.Model, tea.Cmd) {
	if m.dash.Reviews == 0 {
		return m, nil
	}
	m.screen, m.loadingReviews, m.loadingLessons = loading, true, false
	return m, m.load()
}

func (m Model) loadLessons() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		p, err := m.backend.Lessons(ctx)
		return lessonsMsg{p, err}
	}
}

func (m Model) startLesson(id int) tea.Cmd {
	run := m.run
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		return startedMsg{m.backend.StartLesson(ctx, id), run}
	}
}

func (m Model) startLessons() (tea.Model, tea.Cmd) {
	if m.dash.LessonsToday == 0 {
		return m, nil
	}
	m.screen, m.loadingReviews, m.loadingLessons = loading, false, true
	return m, m.loadLessons()
}

// batch is the lessons being taught or quizzed right now.
func (m Model) batch() []lessons.Lesson {
	end := min(m.batchStart+m.plan.BatchSize, len(m.plan.Lessons))
	return m.plan.Lessons[m.batchStart:end]
}

type pageKind int

const (
	meaningPage pageKind = iota
	readingPage
	contextPage
)

// teachPage is one teaching screen: a lesson in the batch and which page.
type teachPage struct {
	lesson int
	kind   pageKind
}

func (m Model) pages() []teachPage {
	var ps []teachPage
	for i, l := range m.batch() {
		ps = append(ps, teachPage{i, meaningPage})
		if l.HasReading() {
			ps = append(ps, teachPage{i, readingPage})
		}
		if len(l.Sentences) > 0 {
			ps = append(ps, teachPage{i, contextPage})
		}
	}
	return ps
}

// quiz starts the back-to-back quiz on the current batch.
func (m Model) quiz() (tea.Model, tea.Cmd) {
	var items []review.Item
	for _, l := range m.batch() {
		items = append(items, l.Item)
	}
	m.session = review.NewSession(items, rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())))
	m.screen, m.lessonMode, m.feedback, m.bigDrawn = reviewing, true, "", ""
	m.input.Reset()
	return m, nil
}

// nextBatch moves on after a quiz: teach the next batch, or show the summary.
func (m Model) nextBatch() Model {
	m.batchStart += m.plan.BatchSize
	m.lessonMode, m.bigDrawn = false, ""
	if m.batchStart >= len(m.plan.Lessons) {
		m.screen = lessonSummary
		return m
	}
	m.screen, m.page = teaching, 0
	return m
}

// startForbidden reports whether a lesson start failed for lack of the
// assignments:start token permission.
func (m Model) startForbidden() bool {
	var apiErr *wanikani.APIError
	return errors.As(m.startErr, &apiErr) && apiErr.Status == 403
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
	run := m.run
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		pending, err := m.backend.Submit(ctx, s)
		return submittedMsg{pending, err, run}
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
		m.width, m.height = msg.Width, msg.Height
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
		m.loadingReviews, m.loadingLessons = false, false
		return m, m.loadDashboard()
	case dashboardMsg:
		if msg.err != nil {
			return m.loadFailed(msg.err)
		}
		m.dash = msg.d
		m.screen = home
		if len(msg.d.Forecast) == 0 {
			return m, nil
		}
		// Reload when the next reviews become available, so a dashboard left
		// open never shows stale counts.
		at := msg.d.Forecast[0].At.Add(refreshSlack)
		m.refreshAt = at
		return m, tea.Tick(time.Until(at), func(time.Time) tea.Msg { return refreshMsg{at} })
	case refreshMsg:
		if !msg.at.Equal(m.refreshAt) || m.screen != home || m.quitting {
			return m, nil // replaced by a newer refresh, or we moved on
		}
		return m, m.loadDashboard() // stays on the dashboard while it reloads
	case lessonsMsg:
		if msg.err != nil {
			return m.loadFailed(msg.err)
		}
		m.plan, m.batchStart, m.page, m.scroll, m.bigDrawn = msg.plan, 0, 0, 0, ""
		m.run++
		m.started, m.startFailed, m.startErr = 0, 0, nil
		m.screen = teaching
		if len(m.plan.Lessons) == 0 {
			m.screen = lessonSummary
		}
		return m, nil
	case startedMsg:
		m.inFlight--
		if msg.run == m.run && errors.Is(msg.err, wanikani.ErrUnauthorized) {
			return m.loadFailed(msg.err) // token entry; unstarted items stay lessons
		}
		if msg.run != m.run {
			if m.quitting && m.inFlight == 0 {
				return m, tea.Quit
			}
			return m, nil
		}
		if msg.err != nil {
			m.startFailed++
			m.startErr = msg.err
			if m.startForbidden() && (m.lessonMode || m.screen == teaching) {
				// Nothing can be started with this token: stop now and explain,
				// rather than after every remaining batch.
				m.screen, m.lessonMode = lessonSummary, false
			}
		} else {
			m.started++
		}
		if m.quitting && m.inFlight == 0 {
			return m, tea.Quit
		}
		return m, nil
	case settingsMsg:
		if msg.err != nil {
			return m.loadFailed(msg.err)
		}
		m.settings, m.settingRow, m.screen = msg.s, 0, settingsScreen
		return m, nil
	case savedMsg:
		if msg.err != nil {
			return m.loadFailed(msg.err)
		}
		return m.toDashboard() // the lesson count depends on the settings
	case submittedMsg:
		m.inFlight--
		if msg.run != m.run {
			if m.quitting && m.inFlight == 0 {
				return m, tea.Quit
			}
			return m, nil // from an earlier session: its summary is gone
		}
		var apiErr *wanikani.APIError
		switch {
		case errors.As(msg.err, &apiErr) && apiErr.Status == 403 && msg.pending:
			m.pending++
			m.submitForbidden = true
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
			if m.quitting { // waiting for submits; starting a session now could be cut off
				switch msg.String() {
				case "q", "esc", "ctrl+c":
					return m.quit()
				}
				return m, nil
			}
			switch msg.String() {
			case "r", "enter":
				return m.startReviews()
			case "l":
				return m.startLessons()
			case "s":
				return m, m.loadSettings()
			case "q", "esc", "ctrl+c":
				return m.quit()
			}
			return m, nil
		}
		if m.screen == settingsScreen {
			return m.settingsKey(msg.String())
		}
		if m.screen == teaching {
			last := len(m.pages()) - 1
			switch msg.String() {
			case "right", "enter":
				m.scroll = 0
				if m.page < last {
					m.page++
					return m, nil
				}
				return m.quiz()
			case "left":
				m.page, m.scroll = max(m.page-1, 0), 0
				return m, nil
			case "down":
				lines, avail := m.teachBody()
				m.scroll = min(m.scroll+1, max(len(lines)-avail, 0))
				return m, nil
			case "up":
				m.scroll = max(m.scroll-1, 0)
				return m, nil
			case "q":
				return m.toDashboard()
			case "esc", "ctrl+c":
				return m.quit()
			}
			return m, nil
		}
		if m.screen == summary && msg.String() == "t" && m.submitForbidden {
			return m.loadFailed(wanikani.ErrUnauthorized) // token entry
		}
		if m.screen == lessonSummary && msg.String() == "t" && m.startForbidden() {
			return m.loadFailed(wanikani.ErrUnauthorized) // token entry
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
	m.run++
	m.pending, m.rejected, m.lost, m.lostErr, m.submitForbidden = 0, 0, 0, nil, false // per-session counts
	m.bigDrawn = ""                                                                   // a new session always draws its first item
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
		if m.loadingLessons {
			return m, m.loadLessons()
		}
		if m.loadingReviews {
			return m, m.load()
		}
		return m, m.loadDashboard()
	case lessonSummary:
		if m.inFlight > 0 {
			return m, nil // wait: the dashboard's lesson count needs these starts
		}
		return m.toDashboard()
	case summary:
		return m.toDashboard()
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
	switch value {
	case "":
		return m, nil
	case ":q", ":wq": // vim habit: leave for the dashboard
		// Finished items are already submitted; the one in progress stays due.
		return m.toDashboard()
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
		if m.lessonMode {
			cmd = m.startLesson(sub.AssignmentID) // lessons start; they are never reviews
		} else {
			cmd = m.submit(*sub)
		}
	}
	if _, _, ok := m.session.Current(); !ok {
		if m.lessonMode {
			m = m.nextBatch()
		} else {
			m.screen = summary
		}
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
	// A second quit stops waiting: answers still sending are already saved in
	// pending.json (write-ahead) and go out on the next launch.
	if m.inFlight == 0 || m.quitting {
		return m, tea.Quit
	}
	m.quitting = true
	return m, nil
}

const settingRows = 6 // daily cap, order, radicals, kanji, vocabulary, batch size

func (m Model) loadSettings() tea.Cmd {
	return func() tea.Msg {
		s, err := m.backend.Settings()
		return settingsMsg{s, err}
	}
}

func (m Model) saveSettings() tea.Cmd {
	s := m.settings // a copy: the command runs later, on another goroutine
	return func() tea.Msg { return savedMsg{m.backend.SaveSettings(s)} }
}

func (m Model) settingsKey(key string) (tea.Model, tea.Cmd) {
	s := &m.settings
	prev := *s
	switch key {
	case "up":
		m.settingRow = (m.settingRow + settingRows - 1) % settingRows
	case "down":
		m.settingRow = (m.settingRow + 1) % settingRows
	case "left", "right", "space":
		d := 1
		if key == "left" {
			d = -1
		}
		switch m.settingRow {
		case 0:
			s.DailyCap += d
		case 1:
			if s.Order == lessons.Classic {
				s.Order = lessons.Interleaved
			} else {
				s.Order = lessons.Classic
			}
		case 2:
			s.Types.Radical = !s.Types.Radical
		case 3:
			s.Types.Kanji = !s.Types.Kanji
		case 4:
			s.Types.Vocabulary = !s.Types.Vocabulary
		case 5:
			s.BatchSize += d
		}
		if s.Types == (lessons.Types{}) {
			*s = prev // at least one type stays on
		}
		*s = s.Clamp()
	case "esc", "enter":
		return m, m.saveSettings()
	case "ctrl+c":
		return m.quit()
	}
	return m, nil
}
