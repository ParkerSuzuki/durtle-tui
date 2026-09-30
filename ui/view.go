package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/ParkerSuzuki/durtle-tui/review"
)

// pagePadding is the blank columns left and right of every screen.
const pagePadding = 2

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
	case failed:
		body = fmt.Sprintf("Could not load reviews:\n\n%v\n\n%s", m.err, dim.Render("Enter to retry, Esc to quit"))
	}
	if m.quitting {
		body += "\n\n" + dim.Render(fmt.Sprintf("Finishing %d submission(s) before quitting...", m.inFlight))
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
	charStyle := lipgloss.NewStyle().Bold(true).Padding(3, 4).
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color(typeColors[item.Type]))
	chars := charStyle.Render(item.Characters)
	bar := meaningBar
	if part == review.Reading {
		bar = readingBar
	}
	if w := m.innerWidth(); w > 0 {
		// Span the terminal, content centered. Styles are values, so these
		// calls change local copies, not the shared package-level styles.
		chars = charStyle.Width(w).Align(lipgloss.Center).Render(item.Characters)
		bar = bar.Width(w).Align(lipgloss.Center)
	}
	prompt := bar.Render(fmt.Sprintf("%s %s", typeLabel(item.Type), part))
	return strings.Join([]string{
		dim.Render(fmt.Sprintf("%d / %d done   %s correct", done, m.session.Total(), percentCorrect(m.session.Results()))),
		"",
		chars,
		"",
		prompt,
		m.answerView(),
		"",
		m.feedback,
	}, "\n")
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
	var missed []string
	for _, r := range results {
		if r.Submission.IncorrectMeaning+r.Submission.IncorrectReading > 0 {
			missed = append(missed, r.Item.Characters)
		}
	}
	if len(missed) > 0 {
		lines = append(lines, "Missed: "+strings.Join(missed, "  "))
	}
	if m.inFlight > 0 {
		lines = append(lines, fmt.Sprintf("Sending %d...", m.inFlight))
	}
	if m.pending > 0 {
		lines = append(lines, fmt.Sprintf("%d saved offline; they will be sent next launch.", m.pending))
	}
	if m.rejected > 0 {
		lines = append(lines, fmt.Sprintf("%d refused by WaniKani (probably already reviewed elsewhere).", m.rejected))
	}
	if m.lost > 0 {
		lines = append(lines, fmt.Sprintf("%d could not be sent or saved (%v). Redo them on the website.", m.lost, m.lostErr))
	}
	lines = append(lines, "", dim.Render("Enter or Esc to quit"))
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
