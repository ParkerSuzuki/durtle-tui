# durtle-tui Milestone 3 (Lessons) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Pick today's lessons by the user's own editable rules, teach them, quiz each batch with the review engine, and start each passed item on WaniKani.

**Architecture:** A new pure package `lessons` owns settings, selection, the "started today" count, mnemonic markup, and the lesson types. `wanikani` gains subject teaching fields and `StartAssignment`. The backend loads settings from `settings.json`, joins `/summary` lesson IDs with the caches, and builds `lessons.Plan`. The UI adds `teaching`, `lessonSummary`, and `settingsScreen`; the quiz is the existing `reviewing` screen with `lessonMode` set.

**Tech Stack:** Go 1.27, Bubble Tea v2, Lip Gloss v2. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-09-30-lessons-design.md` (decisions 25 to 28).

## Global Constraints

- No new modules. No em dashes. `gofmt -w .` and `go vet ./...` before every commit.
- `lessons` imports only the standard library and `review`. No I/O.
- Settings: `daily_cap` 0..100 (default 10), `order` `classic`|`interleaved` (default classic), `types` radical/kanji/vocabulary (at least one on; default all), `batch_size` 3..10 (default: WaniKani `lessons_batch_size`, else 5). File `os.UserConfigDir()/durtle-tui/settings.json`, mode 0600. Missing or corrupt file means defaults (written back).
- `subjectCacheVersion` becomes 4.
- Quiz answers never go to `POST /reviews`; passed items go to `PUT /assignments/<id>/start`.
- Manual checks: tmux capture of the dashboard and settings screen only. Never start a real lesson (memory: no-focus-stealing-tests).
- Explain the Go ideas per task in `docs/learning-go.md` (section 14).

## Review Focus

1. The daily cap already used up (for example by lessons done on the website today): `LessonsToday` is 0 and `l` does nothing. Pinned in Task 3 (`TestLessonsCapCountsToday`) and Task 4 (`TestLessonKeyNeedsLessonsToday`).
2. Turning off the last lesson type on the settings screen must be refused, not accepted or silently reset. Pinned in Task 5 (`TestSettingsScreen`).
3. Quitting (Esc) mid-quiz with a lesson start in flight waits for it. Pinned in Task 4 (`TestQuitWaitsForLessonStart`).
4. An image-only radical lesson when `rsvg-convert` is unavailable is skipped without breaking the batch. Pinned in Task 3 (`TestLessons`, radical with no image and no art).
5. A lesson with no reading page and no sentences (a radical, or kana vocabulary) renders with only a meaning page. Pinned in Task 4 (`TestTeachingPages`).

---

### Task 1: API teaching fields and StartAssignment

**Files:** Modify `wanikani/types.go`, `wanikani/endpoints.go`, `backend.go` (cache version); Test `wanikani/endpoints_test.go`

**Interfaces:**
- Produces: `Subject.MeaningMnemonic, MeaningHint, ReadingMnemonic, ReadingHint string`, `Subject.ContextSentences []ContextSentence` (`En`, `Ja`), `Subject.PartsOfSpeech []string`, `Subject.ComponentSubjectIDs []int`; `User.Preferences.LessonsBatchSize int`; `func (c *Client) StartAssignment(ctx context.Context, assignmentID int) error`.

**Go concepts:** JSON `null` decoding into a plain `string` (stays `""`); nested named structs in JSON; `fmt.Sprintf` for URL paths.

- [ ] **Step 1: Failing tests** (append to `wanikani/endpoints_test.go`)

```go
func TestStartAssignment(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/assignments/42/start" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		var body map[string]map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["assignment"] == nil {
			t.Errorf("body = %v, %v", body, err)
		}
		fmt.Fprint(w, `{}`)
	})
	if err := c.StartAssignment(context.Background(), 42); err != nil {
		t.Fatal(err)
	}
}

func TestSubjectTeachingFields(t *testing.T) {
	var s Subject
	err := json.Unmarshal([]byte(`{"meaning_mnemonic":"m","meaning_hint":null,"reading_mnemonic":"r","reading_hint":"h",
		"context_sentences":[{"en":"Adult.","ja":"大人です。"}],"parts_of_speech":["noun"],"component_subject_ids":[1,2]}`), &s)
	if err != nil {
		t.Fatal(err)
	}
	if s.MeaningMnemonic != "m" || s.MeaningHint != "" || s.ReadingHint != "h" || s.ContextSentences[0].Ja != "大人です。" ||
		s.PartsOfSpeech[0] != "noun" || len(s.ComponentSubjectIDs) != 2 {
		t.Errorf("decoded %+v", s)
	}
	var u User
	if err := json.Unmarshal([]byte(`{"level":3,"preferences":{"lessons_batch_size":4}}`), &u); err != nil || u.Preferences.LessonsBatchSize != 4 {
		t.Errorf("user %+v, %v", u, err)
	}
}
```

- [ ] **Step 2:** `go test ./wanikani/` fails: `c.StartAssignment undefined`.
- [ ] **Step 3: Implement.** In `types.go` add to `Subject`:

```go
	MeaningMnemonic     string            `json:"meaning_mnemonic"`
	MeaningHint         string            `json:"meaning_hint"` // null decodes to ""
	ReadingMnemonic     string            `json:"reading_mnemonic"`
	ReadingHint         string            `json:"reading_hint"`
	ContextSentences    []ContextSentence `json:"context_sentences"`
	PartsOfSpeech       []string          `json:"parts_of_speech"`
	ComponentSubjectIDs []int             `json:"component_subject_ids"`
```

and add:

```go
type ContextSentence struct {
	En string `json:"en"`
	Ja string `json:"ja"`
}

type Preferences struct {
	LessonsBatchSize int `json:"lessons_batch_size"`
}
```

with `Preferences Preferences \`json:"preferences"\`` added to `User`. In `endpoints.go` (import `fmt`):

```go
// StartAssignment starts a lesson: the item moves to Apprentice 1 and
// enters the review queue. Needs the assignments:start token permission.
func (c *Client) StartAssignment(ctx context.Context, assignmentID int) error {
	body := map[string]any{"assignment": map[string]any{}}
	return c.do(ctx, http.MethodPut, fmt.Sprintf("%sassignments/%d/start", c.base, assignmentID), body, nil)
}
```

In `backend.go` set `subjectCacheVersion = 4` and extend its comment: "4 for the teaching fields".
- [ ] **Step 4:** `go test ./...` passes.
- [ ] **Step 5:** Commit `wanikani: teaching fields, batch size preference, StartAssignment`.

---

### Task 2: Package lessons

**Files:** Create `lessons/lessons.go`, `lessons/markup.go`, `lessons/lessons_test.go`

**Interfaces:**
- Consumes: `review.Item`.
- Produces:
  ```go
  type Order string; const Classic Order = "classic"; const Interleaved Order = "interleaved"
  type Types struct{ Radical, Kanji, Vocabulary bool }   // json: radical, kanji, vocabulary
  type Settings struct{ DailyCap int; Order Order; Types Types; BatchSize int } // json: daily_cap, order, types, batch_size
  func Default(waniKaniBatch int) Settings
  func (s Settings) Clamp() Settings
  type Candidate struct{ AssignmentID, SubjectID, Level int; Type string }
  func Pick(available []Candidate, startedToday int, s Settings) []Candidate
  func StartedToday(now time.Time, started []time.Time) int
  type Lesson struct{ review.Item; MeaningMnemonic, MeaningHint, ReadingMnemonic, ReadingHint string;
      PartsOfSpeech []string; KanjiReadings []KanjiReading; Components []Component; Sentences []Sentence }
  type KanjiReading struct{ Reading, Type string; Accepted bool }
  type Component struct{ Characters, Meaning, Type string }
  type Sentence struct{ Ja, En string }
  type Plan struct{ Lessons []Lesson; BatchSize int }
  type Span struct{ Text, Tag string }
  func Markup(s string) []Span
  ```

**Go concepts:** struct embedding (`Lesson` embeds `review.Item`, so `l.Meanings` and `l.HasReading()` work directly); string-typed enums; `cmp.Compare` and `cmp.Or` for multi-key sorting (Go 1.22); comparing structs with `==` to detect "all false"; a hand-written scanner with `strings.IndexByte`.

- [ ] **Step 1: Failing tests** (`lessons/lessons_test.go`)

```go
package lessons

import (
	"testing"
	"time"
)

func ids(cs []Candidate) []int {
	var out []int
	for _, c := range cs {
		out = append(out, c.SubjectID)
	}
	return out
}

func equal(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

var avail = []Candidate{
	{SubjectID: 30, Level: 2, Type: "vocabulary"},
	{SubjectID: 10, Level: 1, Type: "vocabulary"},
	{SubjectID: 5, Level: 1, Type: "kanji"},
	{SubjectID: 6, Level: 1, Type: "kanji"},
	{SubjectID: 1, Level: 1, Type: "radical"},
	{SubjectID: 11, Level: 1, Type: "kana_vocabulary"},
	{SubjectID: 20, Level: 2, Type: "radical"},
}

func TestPick(t *testing.T) {
	all := Default(5)
	all.DailyCap = 100
	noVocab := all
	noVocab.Types.Vocabulary = false
	inter := all
	inter.Order = Interleaved
	tests := []struct {
		name    string
		s       Settings
		started int
		want    []int
	}{
		{"classic: level, then radical, kanji, vocab, then id", all, 0, []int{1, 5, 6, 10, 11, 20, 30}},
		{"type filter drops vocabulary and kana vocabulary", noVocab, 0, []int{1, 5, 6, 20}},
		{"interleaved deals by type, skipping types that ran out", inter, 0, []int{1, 5, 10, 20, 6, 11, 30}},
		{"cap minus lessons already started today", Settings{DailyCap: 3, Order: Classic, Types: all.Types}, 1, []int{1, 5}},
		{"cap used up", Settings{DailyCap: 2, Order: Classic, Types: all.Types}, 5, nil},
		{"cap zero", Settings{DailyCap: 0, Order: Classic, Types: all.Types}, 0, nil},
	}
	for _, tt := range tests {
		if got := ids(Pick(avail, tt.started, tt.s)); !equal(got, tt.want) {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestStartedToday(t *testing.T) {
	loc := time.FixedZone("test", -6*3600)
	now := time.Date(2026, 9, 30, 0, 30, 0, 0, loc)
	started := []time.Time{
		time.Date(2026, 9, 29, 23, 59, 0, 0, loc), // yesterday
		time.Date(2026, 9, 30, 0, 0, 0, 0, loc),   // exactly midnight: today
		time.Date(2026, 9, 30, 6, 10, 0, 0, time.UTC), // 00:10 local
	}
	if got := StartedToday(now, started); got != 2 {
		t.Errorf("StartedToday = %d, want 2", got)
	}
}

func TestDefaultAndClamp(t *testing.T) {
	if d := Default(0); d.BatchSize != 5 || d.DailyCap != 10 || d.Order != Classic || d.Types != (Types{true, true, true}) {
		t.Errorf("Default(0) = %+v", d)
	}
	if d := Default(4); d.BatchSize != 4 {
		t.Errorf("Default(4).BatchSize = %d", d.BatchSize)
	}
	got := Settings{DailyCap: 500, Order: "weird", BatchSize: 1}.Clamp()
	if got.DailyCap != 100 || got.Order != Classic || got.BatchSize != 3 || got.Types != (Types{true, true, true}) {
		t.Errorf("Clamp = %+v", got)
	}
}

func TestMarkup(t *testing.T) {
	got := Markup("The <radical>ground</radical> reads <reading>じ</reading>, <b>not</b> <ja>地</ja>.")
	want := []Span{{"The ", ""}, {"ground", "radical"}, {" reads ", ""}, {"じ", "reading"}, {", ", ""},
		{"not", ""}, {" ", ""}, {"地", "ja"}, {".", ""}}
	if len(got) != len(want) {
		t.Fatalf("Markup = %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("span %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if got := Markup("a < b"); len(got) != 1 || got[0].Text != "a < b" {
		t.Errorf("a lone < must stay text: %+v", got)
	}
}
```

- [ ] **Step 2:** `go test ./lessons/` fails: `undefined: Candidate`.
- [ ] **Step 3: Implement** `lessons/lessons.go`:

```go
// Package lessons decides which lessons to teach today, from the user's own
// rules, and holds what a lesson shows. It does no I/O.
package lessons

import (
	"cmp"
	"slices"
	"time"

	"github.com/ParkerSuzuki/durtle-tui/review"
)

// Order is how today's lessons are arranged.
type Order string

const (
	Classic     Order = "classic"     // level, then radicals, kanji, vocabulary
	Interleaved Order = "interleaved" // the classic order dealt round-robin by type
)

// Types says which subject types to learn. Vocabulary includes kana vocabulary.
type Types struct {
	Radical    bool `json:"radical"`
	Kanji      bool `json:"kanji"`
	Vocabulary bool `json:"vocabulary"`
}

// Settings are the user's lesson rules, saved in settings.json.
type Settings struct {
	DailyCap  int   `json:"daily_cap"`
	Order     Order `json:"order"`
	Types     Types `json:"types"`
	BatchSize int   `json:"batch_size"`
}

const (
	MaxDailyCap  = 100
	MinBatchSize = 3
	MaxBatchSize = 10
)

// Default is the starting settings, seeded with the user's WaniKani batch
// size (0 when unknown).
func Default(waniKaniBatch int) Settings {
	if waniKaniBatch == 0 {
		waniKaniBatch = 5
	}
	return Settings{DailyCap: 10, Order: Classic, Types: Types{true, true, true}, BatchSize: waniKaniBatch}.Clamp()
}

// Clamp puts every setting back in range.
func (s Settings) Clamp() Settings {
	s.DailyCap = min(max(s.DailyCap, 0), MaxDailyCap)
	s.BatchSize = min(max(s.BatchSize, MinBatchSize), MaxBatchSize)
	if s.Order != Interleaved {
		s.Order = Classic
	}
	if s.Types == (Types{}) {
		s.Types = Types{true, true, true}
	}
	return s
}

func (s Settings) wants(subjectType string) bool {
	switch rank(subjectType) {
	case 0:
		return s.Types.Radical
	case 1:
		return s.Types.Kanji
	}
	return s.Types.Vocabulary
}

// rank orders types the way WaniKani teaches them within a level.
func rank(subjectType string) int {
	switch subjectType {
	case "radical":
		return 0
	case "kanji":
		return 1
	}
	return 2 // vocabulary, kana_vocabulary
}

// Candidate is a lesson available now.
type Candidate struct {
	AssignmentID, SubjectID, Level int
	Type                           string
}

// Pick returns today's lessons: available ones of the wanted types, in the
// chosen order, limited to what the daily cap has left.
func Pick(available []Candidate, startedToday int, s Settings) []Candidate {
	var picked []Candidate
	for _, c := range available {
		if s.wants(c.Type) {
			picked = append(picked, c)
		}
	}
	slices.SortFunc(picked, func(a, b Candidate) int {
		return cmp.Or(cmp.Compare(a.Level, b.Level), cmp.Compare(rank(a.Type), rank(b.Type)), cmp.Compare(a.SubjectID, b.SubjectID))
	})
	if s.Order == Interleaved {
		picked = interleave(picked)
	}
	n := min(len(picked), max(s.DailyCap-startedToday, 0))
	if n == 0 {
		return nil
	}
	return picked[:n]
}

// interleave deals classic-ordered lessons round-robin by type: radical,
// kanji, vocabulary, repeat, skipping types that ran out.
func interleave(sorted []Candidate) []Candidate {
	var queues [3][]Candidate
	for _, c := range sorted {
		r := rank(c.Type)
		queues[r] = append(queues[r], c)
	}
	out := make([]Candidate, 0, len(sorted))
	for len(out) < len(sorted) {
		for r := range queues {
			if len(queues[r]) > 0 {
				out = append(out, queues[r][0])
				queues[r] = queues[r][1:]
			}
		}
	}
	return out
}

// StartedToday counts start times at or after local midnight of now's day
// (in now's time zone), from any client, website included.
func StartedToday(now time.Time, started []time.Time) int {
	y, m, d := now.Date()
	midnight := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	n := 0
	for _, t := range started {
		if !t.Before(midnight) {
			n++
		}
	}
	return n
}

// Lesson is one item to teach: its quiz item plus teaching content.
type Lesson struct {
	review.Item
	MeaningMnemonic, MeaningHint string
	ReadingMnemonic, ReadingHint string
	PartsOfSpeech                []string
	KanjiReadings                []KanjiReading // kanji only
	Components                   []Component    // radicals of a kanji, kanji of a vocabulary word
	Sentences                    []Sentence     // vocabulary only, at most 3
}

type KanjiReading struct {
	Reading, Type string // Type: onyomi, kunyomi, nanori
	Accepted      bool
}

type Component struct {
	Characters, Meaning, Type string // Characters is "" for image-only radicals
}

type Sentence struct{ Ja, En string }

// Plan is today's lessons and how many to teach before each quiz.
type Plan struct {
	Lessons   []Lesson
	BatchSize int
}
```

and `lessons/markup.go`:

```go
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
```

- [ ] **Step 4:** `go test ./lessons/ -v` passes. (Note: `"a < b"` has no `>`, so it stays one span.)
- [ ] **Step 5:** Commit `lessons: settings, pick, started today, markup`.

---

### Task 3: Backend: settings, lessons, start, dashboard count

**Files:** Modify `store/store.go`, `dashboard/dashboard.go`, `backend.go`; Test `backend_test.go`, `store/store_test.go`

**Interfaces:**
- Consumes: Tasks 1-2.
- Produces: `store.ConfigFile(name string) (string, error)`; `dashboard.Dashboard.LessonsToday int`; backend methods `Lessons(ctx) (lessons.Plan, error)`, `StartLesson(ctx, assignmentID int) error`, `Settings() (lessons.Settings, error)`, `SaveSettings(lessons.Settings) error`.

**Go concepts:** reusing one generic cache for a second purpose; keyed composite literals for types from another package (`go vet` rejects unkeyed ones); building a lookup map (subject ID to assignment ID) before joining.

- [ ] **Step 1: Failing tests.** Append to `store/store_test.go`:

```go
func TestConfigFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	got, err := ConfigFile("settings.json")
	if err != nil || got != filepath.Join(dir, "durtle-tui", "settings.json") {
		t.Errorf("ConfigFile = %q, %v", got, err)
	}
}
```

Append to `backend_test.go` (add import `github.com/ParkerSuzuki/durtle-tui/lessons`):

```go
// lessonServer serves one radical, one kanji built from it, one vocabulary
// word, and one image-only radical, all available as lessons now.
func lessonServer(t *testing.T, startedToday bool) *backend {
	t.Helper()
	started := `null`
	if startedToday {
		started = fmt.Sprintf("%q", time.Now().UTC().Format(time.RFC3339))
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":{"level":1,"preferences":{"lessons_batch_size":4}}}`)
	})
	mux.HandleFunc("/summary", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"data":{"lessons":[{"available_at":%q,"subject_ids":[3,2,1,4]}],"reviews":[]}}`,
			time.Now().Add(-time.Minute).UTC().Format(time.RFC3339))
	})
	mux.HandleFunc("/subjects", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"pages":{"next_url":null},"data":[
			{"id":1,"object":"radical","data":{"level":1,"characters":"一","meanings":[{"meaning":"Ground","primary":true,"accepted_answer":true}],"meaning_mnemonic":"a <radical>line</radical>"}},
			{"id":2,"object":"kanji","data":{"level":1,"characters":"二","component_subject_ids":[1],
				"meanings":[{"meaning":"Two","primary":true,"accepted_answer":true}],
				"readings":[{"reading":"に","primary":true,"accepted_answer":true,"type":"onyomi"},{"reading":"ふた","accepted_answer":false,"type":"kunyomi"}]}},
			{"id":3,"object":"vocabulary","data":{"level":1,"characters":"二つ","component_subject_ids":[2],"parts_of_speech":["numeral"],
				"meanings":[{"meaning":"Two Things","primary":true,"accepted_answer":true}],
				"readings":[{"reading":"ふたつ","primary":true,"accepted_answer":true}],
				"context_sentences":[{"en":"a","ja":"あ"},{"en":"b","ja":"い"},{"en":"c","ja":"う"},{"en":"d","ja":"え"}]}},
			{"id":4,"object":"radical","data":{"level":1,"characters":null,"meanings":[{"meaning":"Beggar","primary":true,"accepted_answer":true}]}},
			{"id":5,"object":"kanji","data":{"level":1,"characters":"三"}}]}`)
	})
	mux.HandleFunc("/assignments", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"pages":{"next_url":null},"data":[
			{"id":10,"object":"assignment","data":{"subject_id":1}},
			{"id":20,"object":"assignment","data":{"subject_id":2}},
			{"id":30,"object":"assignment","data":{"subject_id":3}},
			{"id":40,"object":"assignment","data":{"subject_id":4}},
			{"id":50,"object":"assignment","data":{"subject_id":5,"srs_stage":1,"started_at":%s}}]}`, started)
	})
	mux.HandleFunc("/study_materials", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"pages":{"next_url":null},"data":[]}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	b := &backend{dir: t.TempDir(), base: srv.URL + "/"}
	b.client = wanikani.NewClient(b.base, "tok")
	return b
}

func TestLessons(t *testing.T) {
	b := lessonServer(t, false)
	plan, err := b.Lessons(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if plan.BatchSize != 4 {
		t.Errorf("batch size %d, want 4 from the WaniKani preference", plan.BatchSize)
	}
	var chars []string
	for _, l := range plan.Lessons {
		chars = append(chars, l.Characters)
	}
	// The image-only radical has no art here (no image URL), so it is skipped.
	if strings.Join(chars, ",") != "一,二,二つ" {
		t.Fatalf("lessons = %v, want 一,二,二つ", chars)
	}
	kanji, vocab := plan.Lessons[1], plan.Lessons[2]
	if len(kanji.KanjiReadings) != 2 || len(kanji.Components) != 1 || kanji.Components[0].Meaning != "Ground" {
		t.Errorf("kanji lesson = %+v", kanji)
	}
	if len(vocab.Sentences) != 3 || vocab.PartsOfSpeech[0] != "numeral" || plan.Lessons[0].MeaningMnemonic == "" {
		t.Errorf("vocab lesson = %+v", vocab)
	}
	if s, err := b.Settings(); err != nil || s.BatchSize != 4 || s.DailyCap != 10 {
		t.Errorf("settings.json not written with defaults: %+v, %v", s, err)
	}
}

func TestLessonsCapCountsToday(t *testing.T) {
	b := lessonServer(t, true) // one lesson (subject 5) already started today
	if err := b.SaveSettings(lessons.Settings{DailyCap: 2, Order: lessons.Classic,
		Types: lessons.Types{Radical: true, Kanji: true, Vocabulary: true}, BatchSize: 3}); err != nil {
		t.Fatal(err)
	}
	plan, err := b.Lessons(context.Background())
	if err != nil || len(plan.Lessons) != 1 {
		t.Fatalf("got %d lessons (%v); cap 2 minus 1 started today leaves 1", len(plan.Lessons), err)
	}
	d, err := b.Dashboard(context.Background())
	if err != nil || d.LessonsToday != 1 || d.Lessons != 4 {
		t.Errorf("dashboard lessons today %d of %d (%v), want 1 of 4", d.LessonsToday, d.Lessons, err)
	}
}

func TestStartLesson(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.Method + " " + r.URL.Path
		w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	b := &backend{dir: t.TempDir(), base: srv.URL + "/"}
	b.client = wanikani.NewClient(b.base, "tok")
	if err := b.StartLesson(context.Background(), 7); err != nil || path != "PUT /assignments/7/start" {
		t.Errorf("StartLesson: %q, %v", path, err)
	}
}
```

- [ ] **Step 2:** `go test ./...` fails to compile (`b.Lessons undefined`, `ConfigFile undefined`).
- [ ] **Step 3: Implement.**
  - `store/store.go`: replace `tokenPath` with a general helper and use it:

```go
// ConfigFile is the path of a durtle-tui config file, e.g.
// ~/.config/durtle-tui/settings.json on Linux.
func ConfigFile(name string) (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, appName, name), nil
}

func tokenPath() (string, error) { return ConfigFile("token") }
```

  - `dashboard/dashboard.go`: add `LessonsToday int // lessons the daily cap still allows now` to `Dashboard` (set by the backend).
  - `backend.go` (imports `lessons`): add

```go
// loadSettings reads settings.json, clamped. A missing or corrupt file is
// replaced by defaults seeded with the user's WaniKani batch size.
func (b *backend) loadSettings(waniKaniBatch int) (lessons.Settings, error) {
	path, err := store.ConfigFile("settings.json")
	if err != nil {
		return lessons.Settings{}, err
	}
	var s lessons.Settings
	if err := store.ReadJSON(path, &s); err != nil || s == (lessons.Settings{}) {
		s = lessons.Default(waniKaniBatch)
		return s, store.WriteJSON(path, s)
	}
	return s.Clamp(), nil
}

// Settings returns the lesson rules for the settings screen.
func (b *backend) Settings() (lessons.Settings, error) { return b.loadSettings(0) }

// SaveSettings stores the lesson rules, clamped.
func (b *backend) SaveSettings(s lessons.Settings) error {
	path, err := store.ConfigFile("settings.json")
	if err != nil {
		return err
	}
	return store.WriteJSON(path, s.Clamp())
}

// lessonCandidates lists lessons available now: the summary's lesson
// subject IDs joined with their assignments and subjects, minus hidden ones.
func lessonCandidates(now time.Time, sum wanikani.Summary,
	assignments map[int]wanikani.Resource[wanikani.Assignment],
	subjects map[int]wanikani.Resource[wanikani.Subject]) []lessons.Candidate {
	bySubject := map[int]int{} // subject ID -> assignment ID
	for _, a := range assignments {
		bySubject[a.Data.SubjectID] = a.ID
	}
	var out []lessons.Candidate
	for _, e := range sum.Lessons {
		if e.AvailableAt.After(now) {
			continue
		}
		for _, id := range e.SubjectIDs {
			s, ok := subjects[id]
			aid, has := bySubject[id]
			if !ok || !has || s.Data.HiddenAt != nil {
				continue
			}
			out = append(out, lessons.Candidate{AssignmentID: aid, SubjectID: id, Level: s.Data.Level, Type: s.Object})
		}
	}
	return out
}

// startTimes lists when every started assignment was started.
func startTimes(assignments map[int]wanikani.Resource[wanikani.Assignment]) []time.Time {
	var out []time.Time
	for _, a := range assignments {
		if a.Data.StartedAt != nil {
			out = append(out, *a.Data.StartedAt)
		}
	}
	return out
}

// Lessons picks today's lessons by the user's settings and adds teaching
// content. Image-only radicals that cannot be drawn are left out.
func (b *backend) Lessons(ctx context.Context) (lessons.Plan, error) {
	var none lessons.Plan
	if err := b.connect(); err != nil {
		return none, err
	}
	subjects, err := b.syncSubjects(ctx)
	if err != nil {
		return none, err
	}
	assignments, err := b.syncAssignments(ctx)
	if err != nil {
		return none, err
	}
	synonyms, err := b.syncSynonyms(ctx)
	if err != nil {
		return none, err
	}
	user, err := b.client.User(ctx)
	if err != nil {
		return none, err
	}
	sum, err := b.client.Summary(ctx)
	if err != nil {
		return none, err
	}
	settings, err := b.loadSettings(user.Preferences.LessonsBatchSize)
	if err != nil {
		return none, err
	}
	now := time.Now()
	picked := lessons.Pick(lessonCandidates(now, sum, assignments, subjects), lessons.StartedToday(now, startTimes(assignments)), settings)
	due := make([]wanikani.Resource[wanikani.Assignment], 0, len(picked))
	for _, c := range picked {
		due = append(due, assignments[c.AssignmentID])
	}
	art := func(s wanikani.Resource[wanikani.Subject]) image.Image { return b.radicalImage(ctx, s) }
	items, _ := buildItems(due, subjects, synonyms, art)
	return lessons.Plan{Lessons: buildLessons(items, assignments, subjects), BatchSize: settings.BatchSize}, nil
}

// buildLessons adds each item's teaching content from its subject.
func buildLessons(items []review.Item, assignments map[int]wanikani.Resource[wanikani.Assignment],
	subjects map[int]wanikani.Resource[wanikani.Subject]) []lessons.Lesson {
	out := make([]lessons.Lesson, 0, len(items))
	for _, it := range items {
		s := subjects[assignments[it.AssignmentID].Data.SubjectID].Data
		l := lessons.Lesson{Item: it, MeaningMnemonic: s.MeaningMnemonic, MeaningHint: s.MeaningHint,
			ReadingMnemonic: s.ReadingMnemonic, ReadingHint: s.ReadingHint, PartsOfSpeech: s.PartsOfSpeech}
		if it.Type == "kanji" {
			for _, r := range s.Readings {
				l.KanjiReadings = append(l.KanjiReadings, lessons.KanjiReading{Reading: r.Reading, Type: r.Type, Accepted: r.AcceptedAnswer})
			}
		}
		for _, id := range s.ComponentSubjectIDs {
			c, ok := subjects[id]
			if !ok {
				continue
			}
			comp := lessons.Component{Type: c.Object}
			if c.Data.Characters != nil {
				comp.Characters = *c.Data.Characters
			}
			for _, m := range c.Data.Meanings {
				if m.Primary {
					comp.Meaning = m.Meaning
				}
			}
			l.Components = append(l.Components, comp)
		}
		for _, cs := range s.ContextSentences[:min(len(s.ContextSentences), 3)] {
			l.Sentences = append(l.Sentences, lessons.Sentence{Ja: cs.Ja, En: cs.En})
		}
		out = append(out, l)
	}
	return out
}

// StartLesson starts one lesson on WaniKani.
func (b *backend) StartLesson(ctx context.Context, assignmentID int) error {
	if err := b.connect(); err != nil {
		return err
	}
	return b.client.StartAssignment(ctx, assignmentID)
}
```

  - In `Dashboard`, after fetching `sum`, add before building:

```go
	settings, err := b.loadSettings(user.Preferences.LessonsBatchSize)
	if err != nil {
		return none, err
	}
	now := time.Now()
	d := dashboard.Build(now, user.Level, sum, assignments, subjects)
	d.LessonsToday = len(lessons.Pick(lessonCandidates(now, sum, assignments, subjects),
		lessons.StartedToday(now, startTimes(assignments)), settings))
	return d, nil
```

  and delete the old `return dashboard.Build(...)` line.
- [ ] **Step 4:** `go test -race ./...` passes. Existing `TestDashboard` must still pass: it now also writes `settings.json`, so add `t.Setenv("XDG_CONFIG_HOME", t.TempDir())` at its top if it fails on the real config dir.
- [ ] **Step 5:** Commit `backend: lesson settings, lesson plan, start, dashboard lesson count`.

---

### Task 4: UI: lessons from the dashboard, teaching, quiz, summary

**Files:** Modify `ui/model.go`, `ui/view.go`, `ui/model_test.go`

**Interfaces:**
- Consumes: `lessons.Plan`, `lessons.Lesson` (+ `KanjiReading`, `Component`, `Sentence`), `lessons.Markup`, `dashboard.Dashboard.LessonsToday`.
- Produces: `Backend` gains `Lessons(ctx) (lessons.Plan, error)`, `StartLesson(ctx, int) error`, `Settings() (lessons.Settings, error)`, `SaveSettings(lessons.Settings) error`; screens `teaching`, `lessonSummary`, `settingsScreen`; messages `lessonsMsg`, `startedMsg`, `settingsMsg`, `savedMsg`.

**Go concepts:** reusing a state machine by adding one flag (`lessonMode`) instead of a copy; small named types for UI state (`pageKind`, `teachPage`); embedded-field promotion across packages (`l.Meanings` on a `lessons.Lesson`).

- [ ] **Step 1: Failing tests.** In `ui/model_test.go`: import `lessons`; extend `fakeBackend` with fields `plan lessons.Plan; planErr, startErr error; startedIDs []int; settings lessons.Settings; saved []lessons.Settings` and methods:

```go
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
```

Change `sampleDash` to `Lessons: 60, LessonsToday: 5` (tiles now show today's count) and append:

```go
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
```

- [ ] **Step 2:** `go test ./ui/` fails to compile (`lessonsMsg`, `teaching`, ... undefined).
- [ ] **Step 3: Implement `ui/model.go`.**
  - Import `lessons`. Extend `Backend`:

```go
	Lessons(ctx context.Context) (lessons.Plan, error)
	StartLesson(ctx context.Context, assignmentID int) error
	Settings() (lessons.Settings, error)
	SaveSettings(s lessons.Settings) error
```

  - Add screens after `home`: `teaching`, `lessonSummary`, `settingsScreen`.
  - Add messages: `lessonsMsg struct{ plan lessons.Plan; err error }`, `startedMsg struct{ err error }`, `settingsMsg struct{ s lessons.Settings; err error }`, `savedMsg struct{ err error }`.
  - Add fields: `plan lessons.Plan`, `batchStart int // index of the first lesson in the current batch`, `page int // teaching page within the batch`, `lessonMode bool // the reviewing screen is a lesson quiz`, `loadingLessons bool`, `started, startFailed int`, `startErr error`, `settings lessons.Settings`, `settingRow int`.
  - Add:

```go
func (m Model) loadLessons() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		p, err := m.backend.Lessons(ctx)
		return lessonsMsg{p, err}
	}
}

func (m Model) startLesson(id int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		return startedMsg{m.backend.StartLesson(ctx, id)}
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
```

  - In `toDashboard` also clear `m.lessonMode, m.loadingLessons = false, false`.
  - In `update`, add cases:

```go
	case lessonsMsg:
		if msg.err != nil {
			return m.loadFailed(msg.err)
		}
		m.plan, m.batchStart, m.page, m.bigDrawn = msg.plan, 0, 0, ""
		m.started, m.startFailed, m.startErr = 0, 0, nil
		m.screen = teaching
		if len(m.plan.Lessons) == 0 {
			m.screen = lessonSummary
		}
		return m, nil
	case startedMsg:
		m.inFlight--
		if msg.err != nil {
			m.startFailed++
			m.startErr = msg.err
		} else {
			m.started++
		}
		if m.quitting && m.inFlight == 0 {
			return m, tea.Quit
		}
		return m, nil
```

  - In the `tea.KeyPressMsg` case: on `home` add `case "l": return m.startLessons()`. Before the generic key switch add:

```go
		if m.screen == teaching {
			last := len(m.pages()) - 1
			switch msg.String() {
			case "right", "enter":
				if m.page < last {
					m.page++
					return m, nil
				}
				return m.quiz()
			case "left":
				m.page = max(m.page-1, 0)
				return m, nil
			case "q":
				return m.toDashboard()
			case "esc", "ctrl+c":
				return m.quit()
			}
			return m, nil
		}
		if m.screen == lessonSummary && msg.String() == "t" && m.startForbidden() {
			return m.loadFailed(wanikani.ErrUnauthorized) // token entry
		}
```

  - `enter()`: add `case lessonSummary: return m.toDashboard()`; in `case failed:` retry `m.loadLessons()` when `m.loadingLessons`.
  - `answer()`: replace the submit block and session-end check with:

```go
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
```

  - Add:

```go
// startForbidden reports whether a lesson start failed for lack of the
// assignments:start token permission.
func (m Model) startForbidden() bool {
	var apiErr *wanikani.APIError
	return errors.As(m.startErr, &apiErr) && apiErr.Status == 403
}
```

- [ ] **Step 4: Implement `ui/view.go`.**
  - Extract the block code at the top of `reviewView` into `func (m Model) itemBlock(item review.Item) string` (unchanged logic) and call it from `reviewView`.
  - Add `func (m Model) currentItem() (review.Item, bool)`: `reviewing` returns `m.session.Current()`; `teaching` returns the lesson for `m.pages()[m.page]`; otherwise false. In `bigGlyphs`, change `case reviewing:` to `case reviewing, teaching:` and get the item from `currentItem()`.
  - In `View`: `case teaching: body = m.teachingView()`, `case lessonSummary: body = m.lessonSummaryView()`; failed text says "load lessons" when `m.loadingLessons`.
  - Lessons tile: `tiles()` Lessons count is `m.dash.LessonsToday`, with a note `fmt.Sprintf("of %d available", m.dash.Lessons)` as the tile's last line (`tile` gains `note string`; body `[label, "", "", count, "", note]`).
  - Home hint: build it from parts: `r reviews` when `Reviews > 0`, `l lessons` when `LessonsToday > 0`, then `s settings   q quit`.
  - Add:

```go
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

func (m Model) teachingView() string {
	ps := m.pages()
	p := ps[m.page]
	l := m.batch()[p.lesson]
	var name string
	var body []string
	bar := meaningBar
	switch p.kind {
	case meaningPage:
		name, body = "meaning", meaningLines(l)
	case readingPage:
		name, body, bar = "reading", readingLines(l), readingBar
	case contextPage:
		name, body = "context", contextLines(l)
	}
	text := strings.Join(body, "\n")
	if w := m.innerWidth(); w > 0 {
		bar = bar.Width(w).Align(lipgloss.Center)
		text = lipgloss.NewStyle().Width(w).Render(text)
	}
	return strings.Join([]string{
		dim.Render(fmt.Sprintf("Lesson %d of %d   page %d of %d",
			m.batchStart+p.lesson+1, len(m.plan.Lessons), m.page+1, len(ps))),
		"",
		m.itemBlock(l.Item),
		"",
		bar.Render(typeLabel(l.Type) + " " + name),
		"",
		text,
		"",
		dim.Render("← → pages   enter next   q dashboard   esc quit"),
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
```

  (`view.go` imports gain `errors` only if used there; `startForbidden` lives in `model.go`, so only `lessons` is new.)
- [ ] **Step 5:** `go test -race ./...` passes, including the updated `sampleDash` tests.
- [ ] **Step 6:** Commit `ui: lessons from the dashboard: teaching, batch quiz, lesson summary`.

---

### Task 5: UI settings screen

**Files:** Modify `ui/model.go`, `ui/view.go`, `ui/model_test.go`

**Interfaces:** Consumes `Backend.Settings`, `Backend.SaveSettings`, `lessons.Settings`.

**Go concepts:** a pointer into a struct field (`s := &m.settings`) to edit in place; saving a snapshot by value into a closure.

- [ ] **Step 1: Failing test** (append):

```go
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
```

- [ ] **Step 2:** fails: `settingsScreen` shows nothing / `s` does nothing.
- [ ] **Step 3: Implement.** In `model.go`:

```go
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
```

  On `home` add `case "s": return m, m.loadSettings()`. Before the teaching block add `if m.screen == settingsScreen { return m.settingsKey(msg.String()) }`. In `update` add:

```go
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
```

  In `view.go`, `case settingsScreen: body = m.settingsView()` and:

```go
func (m Model) settingsView() string {
	s := m.settings
	check := func(on bool) string {
		if on {
			return "[x]"
		}
		return "[ ]"
	}
	rows := [settingRows][2]string{
		{"Daily lesson cap", fmt.Sprintf("‹ %d ›", s.DailyCap)},
		{"Order", fmt.Sprintf("‹ %s ›", s.Order)},
		{"Radicals", check(s.Types.Radical)},
		{"Kanji", check(s.Types.Kanji)},
		{"Vocabulary", check(s.Types.Vocabulary)},
		{"Batch size", fmt.Sprintf("‹ %d ›", s.BatchSize)},
	}
	lines := []string{title.Render("Lesson settings"), ""}
	for i, r := range rows {
		cursor := "  "
		if i == m.settingRow {
			cursor = "> "
		}
		lines = append(lines, fmt.Sprintf("%s%-18s %s", cursor, r[0], r[1]))
	}
	lines = append(lines, "", dim.Render("↑↓ choose   ←→ change   space toggle   esc save"))
	return strings.Join(lines, "\n")
}
```

- [ ] **Step 4:** `go test -race ./...` passes.
- [ ] **Step 5:** Commit `ui: lesson settings screen`.

---

### Task 6: Real run (read-only) and docs

- [ ] **Step 1:** Build, run in tmux at 100x40, wait for the dashboard, capture it (Lessons tile shows today's count "of N available"), press `s`, capture the settings screen, press `ctrl+c`. Never press `l` against the real account.
- [ ] **Step 2:** README status mentions lessons and settings; `TODO.md` marks milestone 3 done and removes the "Lessons" item; `docs/learning-go.md` section 14 (struct embedding, `cmp.Or` multi-key sort, a hand-written markup scanner, reusing a state machine with a mode flag, copying state into a command closure).
- [ ] **Step 3:** Commit `docs: milestone 3 lessons`.
