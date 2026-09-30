package lessons

import "strings"

// Span is a run of mnemonic text and the WaniKani tag around it ("" for none).
type Span struct{ Text, Tag string }

var knownTags = map[string]bool{"radical": true, "kanji": true, "vocabulary": true, "meaning": true, "reading": true, "ja": true}

// Markup splits WaniKani mnemonic markup into spans. Known tags become the
// span's Tag; unknown tags are dropped and their text kept; a "<" with no
// closing ">" is ordinary text.
func Markup(s string) []Span {
	var spans []Span
	add := func(text, tag string) {
		if text != "" {
			spans = append(spans, Span{text, tag})
		}
	}
	tag := ""
	for s != "" {
		i := strings.IndexByte(s, '<')
		if i < 0 {
			add(s, tag)
			break
		}
		j := strings.IndexByte(s[i:], '>')
		if j < 0 {
			add(s, tag)
			break
		}
		add(s[:i], tag)
		name := s[i+1 : i+j]
		s = s[i+j+1:]
		switch {
		case strings.HasPrefix(name, "/"):
			tag = ""
		case knownTags[name]:
			tag = name
		}
	}
	return spans
}
