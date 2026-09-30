# durtle-tui Milestone 2 (Dashboard) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** durtle-tui opens on a dashboard (available now, review forecast, level progress, SRS breakdown); reviews start from it and return to it.

**Architecture:** `wanikani` gains `Summary` and `Assignments(since)`. The backend caches assignments with a new generic `syncResources[T]` that also replaces the subject sync. A new pure package `dashboard` computes the four panels. `ui` gets a `home` screen and a `Backend.Dashboard` call.

**Tech Stack:** Go 1.27, Bubble Tea v2, Lip Gloss v2 (already in go.mod). No new dependencies.

**Spec:** `docs/superpowers/specs/2026-09-30-dashboard-design.md` (decisions 21, 22 in `docs/decisions.md`).

## Global Constraints

- No new modules. No em dashes anywhere. `gofmt -w .` and `go vet ./...` before every commit.
- `dashboard` imports only the standard library and `wanikani` (types only). No I/O.
- `subjectCacheVersion` becomes 3; `assignmentCacheVersion` starts at 1.
- SRS groups: Apprentice 1-4, Guru 5-6, Master 7, Enlightened 8, Burned 9. Only started assignments (stage > 0) count. Hidden subjects are excluded everywhere.
- `KanjiNeeded` = ceil(0.9 x kanji at level) = `(n*9 + 9) / 10`.
- SRS and forecast bars use `#A8DADC`; progress bars use the kanji and radical colors from decision 16.
- Visual checks go through tmux capture only; never send answers or Enter to the real account (memory: no-focus-stealing-tests).
- Working agreement: explain the Go concepts per task in `docs/learning-go.md` (section 12).

## Review Focus

1. A summary entry exactly at `now` counts as available, not as forecast. Pinned in Task 2 (`TestAvailableAndForecast`).
2. A level with zero kanji (or an empty dashboard) must render without dividing by zero. Pinned in Task 4 (`TestEmptyDashboard`).
3. Returning to the dashboard with a submit in flight, then quitting, must still wait for the submit. Pinned in Task 4 (`TestSummaryReturnsToDashboard`).
4. A narrow terminal (30 columns) must render the dashboard without panicking. Pinned in Task 4 (`TestDashboardShowsPanels`).
5. An assignment cache from an older format must trigger a full resync. Covered by the shared `syncResources` and pinned by the existing `TestOldSubjectCacheForcesFullSync` plus Task 3's incremental check (`TestDashboard`).

---

### Task 1: API types and endpoints

**Files:** Modify `wanikani/types.go`, `wanikani/endpoints.go`; Test `wanikani/endpoints_test.go`

**Interfaces:**
- Produces: `Subject.Level int`, `Subject.HiddenAt *time.Time`; `Assignment.SRSStage int`, `Assignment.StartedAt, PassedAt *time.Time`, `Assignment.Hidden bool`; `type Summary struct{ Lessons, Reviews []SummaryEntry }`; `type SummaryEntry struct{ AvailableAt time.Time; SubjectIDs []int }`; `func (c *Client) Summary(ctx) (Summary, error)`; `func (c *Client) Assignments(ctx, since time.Time) ([]Resource[Assignment], error)`.

**Go concepts:** reusing a generic helper for a new endpoint; `*time.Time` for nullable timestamps; JSON decoding of RFC 3339 with fractional seconds.

- [ ] **Step 1: Failing tests** (append to `wanikani/endpoints_test.go`)

```go
func TestSummary(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/summary" {
			t.Errorf("path = %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"object":"report","data":{"lessons":[{"available_at":"2026-09-30T16:00:00.000000Z","subject_ids":[1,2]}],"reviews":[{"available_at":"2026-09-30T16:00:00.000000Z","subject_ids":[3]},{"available_at":"2026-09-30T17:00:00.000000Z","subject_ids":[]}]}}`)
	})
	s, err := c.Summary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Lessons) != 1 || len(s.Lessons[0].SubjectIDs) != 2 || len(s.Reviews) != 2 || s.Reviews[0].SubjectIDs[0] != 3 {
		t.Errorf("got %+v", s)
	}
}

func TestAssignmentsSince(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/assignments" || r.URL.Query().Get("updated_after") == "" {
			t.Errorf("request = %s", r.URL)
		}
		fmt.Fprint(w, `{"pages":{"next_url":null},"data":[{"id":5,"object":"assignment","data":{"subject_id":1,"srs_stage":5,"started_at":"2026-01-01T00:00:00Z","passed_at":"2026-02-01T00:00:00Z","hidden":false}}]}`)
	})
	got, err := c.Assignments(context.Background(), time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || len(got) != 1 {
		t.Fatalf("got %v, %v", got, err)
	}
	if a := got[0].Data; a.SRSStage != 5 || a.StartedAt == nil || a.PassedAt == nil {
		t.Errorf("decoded %+v", a)
	}
}

func TestSubjectLevelAndHidden(t *testing.T) {
	var s Subject
	if err := json.Unmarshal([]byte(`{"level":12,"hidden_at":"2026-01-01T00:00:00Z"}`), &s); err != nil {
		t.Fatal(err)
	}
	if s.Level != 12 || s.HiddenAt == nil {
		t.Errorf("decoded %+v", s)
	}
}
```

- [ ] **Step 2:** `go test ./wanikani/` fails: `c.Summary undefined`.
- [ ] **Step 3: Implement.** In `types.go`, add to `Subject`: `Level int \`json:"level"\`` and `HiddenAt *time.Time \`json:"hidden_at"\``. Replace `Assignment` with:

```go
type Assignment struct {
	SubjectID   int        `json:"subject_id"`
	SubjectType string     `json:"subject_type"`
	SRSStage    int        `json:"srs_stage"` // 0 lesson not done, 1-4 apprentice, 5-6 guru, 7 master, 8 enlightened, 9 burned
	StartedAt   *time.Time `json:"started_at"`
	PassedAt    *time.Time `json:"passed_at"`
	Hidden      bool       `json:"hidden"`
}

// Summary is what is available now and over the next day, grouped by hour.
type Summary struct {
	Lessons []SummaryEntry `json:"lessons"`
	Reviews []SummaryEntry `json:"reviews"`
}

type SummaryEntry struct {
	AvailableAt time.Time `json:"available_at"`
	SubjectIDs  []int     `json:"subject_ids"`
}
```

In `endpoints.go`:

```go
// Summary fetches lessons and reviews available now and per hour for the next day.
func (c *Client) Summary(ctx context.Context) (Summary, error) {
	var r Resource[Summary]
	err := c.do(ctx, http.MethodGet, c.base+"summary", nil, &r)
	return r.Data, err
}

// Assignments fetches assignments changed after t (all of them if t is zero).
func (c *Client) Assignments(ctx context.Context, t time.Time) ([]Resource[Assignment], error) {
	return getAll[Assignment](ctx, c, updatedAfter("assignments", t))
}
```

- [ ] **Step 4:** `go test ./wanikani/` passes.
- [ ] **Step 5:** Commit `wanikani: summary, assignments since, subject level`.

---

### Task 2: Package dashboard

**Files:** Create `dashboard/dashboard.go`, `dashboard/dashboard_test.go`

**Interfaces:**
- Consumes: Task 1 types.
- Produces:
  ```go
  type Hour struct { At time.Time; Added, Total int }
  type Progress struct { Radicals, RadicalsPassed, Kanji, KanjiPassed, KanjiNeeded int }
  type SRS [5]int
  var StageNames = [5]string{"Apprentice", "Guru", "Master", "Enlightened", "Burned"}
  type Dashboard struct { Level, Lessons, Reviews int; Forecast []Hour; Progress Progress; SRS SRS }
  func Build(now time.Time, level int, sum wanikani.Summary,
      assignments map[int]wanikani.Resource[wanikani.Assignment],
      subjects map[int]wanikani.Resource[wanikani.Subject]) Dashboard
  ```

**Go concepts:** a pure package as a seam; `sort.Slice` / `slices.SortFunc`; comparing arrays and structs with `==`; integer ceiling division.

- [ ] **Step 1: Failing tests** (`dashboard/dashboard_test.go`)

```go
package dashboard

import (
	"testing"
	"time"

	"github.com/ParkerSuzuki/durtle-tui/wanikani"
)

var now = time.Date(2026, 9, 30, 14, 30, 0, 0, time.UTC)

// hour is the start of the hour n hours from now's hour.
func hour(n int) time.Time { return now.Truncate(time.Hour).Add(time.Duration(n) * time.Hour) }

func entry(at time.Time, ids ...int) wanikani.SummaryEntry {
	return wanikani.SummaryEntry{AvailableAt: at, SubjectIDs: ids}
}

func TestAvailableAndForecast(t *testing.T) {
	sum := wanikani.Summary{
		Lessons: []wanikani.SummaryEntry{entry(hour(0), 1, 2, 3)},
		Reviews: []wanikani.SummaryEntry{
			entry(hour(0), 1, 2), // 14:00, before now: available
			entry(now, 3),        // exactly now: available, not forecast
			entry(hour(5), 7),    // out of order on purpose
			entry(hour(2), 4, 5),
			entry(hour(1)),        // empty hour: left out
			entry(hour(25), 9),    // more than 24 hours away: left out
		},
	}
	d := Build(now, 1, sum, nil, nil)
	if d.Lessons != 3 || d.Reviews != 3 {
		t.Errorf("lessons %d reviews %d, want 3 and 3", d.Lessons, d.Reviews)
	}
	want := []Hour{{hour(2), 2, 5}, {hour(5), 1, 6}}
	if len(d.Forecast) != len(want) {
		t.Fatalf("forecast = %+v, want %+v", d.Forecast, want)
	}
	for i := range want {
		if d.Forecast[i] != want[i] {
			t.Errorf("forecast[%d] = %+v, want %+v", i, d.Forecast[i], want[i])
		}
	}
}

func TestProgressAndSRS(t *testing.T) {
	subjects := map[int]wanikani.Resource[wanikani.Subject]{}
	add := func(id int, object string, level int, hidden bool) {
		s := wanikani.Subject{Level: level}
		if hidden {
			s.HiddenAt = &now
		}
		subjects[id] = wanikani.Resource[wanikani.Subject]{ID: id, Object: object, Data: s}
	}
	for i := 0; i < 33; i++ {
		add(100+i, "kanji", 12, false)
	}
	add(200, "radical", 12, false)
	add(201, "radical", 12, false)
	add(300, "kanji", 12, true) // hidden: ignored everywhere
	add(400, "kanji", 11, false)

	assignments := map[int]wanikani.Resource[wanikani.Assignment]{}
	assign := func(id, subject, stage int, passed bool) {
		a := wanikani.Assignment{SubjectID: subject, SRSStage: stage}
		if stage > 0 {
			a.StartedAt = &now
		}
		if passed {
			a.PassedAt = &now
		}
		assignments[id] = wanikani.Resource[wanikani.Assignment]{ID: id, Data: a}
	}
	for i := 0; i < 21; i++ {
		assign(1000+i, 100+i, 5, true) // 21 level-12 kanji at Guru, passed
	}
	assign(2000, 200, 1, false) // radical in Apprentice
	assign(2001, 201, 9, true)  // radical burned
	assign(3000, 300, 8, true)  // hidden subject
	assign(4000, 400, 7, true)  // level-11 kanji at Master: counts in SRS only
	assign(5000, 132, 0, false) // lesson not done yet: not in SRS

	d := Build(now, 12, wanikani.Summary{}, assignments, subjects)
	if want := (Progress{Radicals: 2, RadicalsPassed: 1, Kanji: 33, KanjiPassed: 21, KanjiNeeded: 30}); d.Progress != want {
		t.Errorf("progress = %+v, want %+v", d.Progress, want)
	}
	if want := (SRS{1, 21, 1, 0, 1}); d.SRS != want {
		t.Errorf("SRS = %v, want %v", d.SRS, want)
	}
	if d.Level != 12 {
		t.Errorf("level = %d", d.Level)
	}
}

func TestKanjiNeeded(t *testing.T) {
	for n, want := range map[int]int{0: 0, 10: 9, 33: 30, 34: 31, 40: 36} {
		if got := kanjiNeeded(n); got != want {
			t.Errorf("kanjiNeeded(%d) = %d, want %d", n, got, want)
		}
	}
}
```

- [ ] **Step 2:** `go test ./dashboard/` fails: `undefined: Build`.
- [ ] **Step 3: Implement** (`dashboard/dashboard.go`)

```go
// Package dashboard turns WaniKani data into the numbers the home screen
// shows. It does no I/O, so every panel is unit tested.
package dashboard

import (
	"slices"
	"time"

	"github.com/ParkerSuzuki/durtle-tui/wanikani"
)

// Hour is one hour of the review forecast.
type Hour struct {
	At    time.Time
	Added int // reviews that become available at At
	Total int // everything available by then, including what is available now
}

// Progress is how far along the current level is.
type Progress struct {
	Radicals, RadicalsPassed int
	Kanji, KanjiPassed       int
	KanjiNeeded              int // passing this many kanji levels you up
}

// SRS counts started items per stage group, in StageNames order.
type SRS [5]int

var StageNames = [5]string{"Apprentice", "Guru", "Master", "Enlightened", "Burned"}

// Dashboard is everything the home screen shows.
type Dashboard struct {
	Level, Lessons, Reviews int
	Forecast                []Hour
	Progress                Progress
	SRS                     SRS
}

// Build computes the dashboard at time now.
func Build(now time.Time, level int, sum wanikani.Summary,
	assignments map[int]wanikani.Resource[wanikani.Assignment],
	subjects map[int]wanikani.Resource[wanikani.Subject]) Dashboard {
	d := Dashboard{Level: level}
	for _, e := range sum.Lessons {
		if !e.AvailableAt.After(now) {
			d.Lessons += len(e.SubjectIDs)
		}
	}
	var upcoming []wanikani.SummaryEntry
	for _, e := range sum.Reviews {
		switch {
		case !e.AvailableAt.After(now):
			d.Reviews += len(e.SubjectIDs)
		case len(e.SubjectIDs) > 0 && !e.AvailableAt.After(now.Add(24*time.Hour)):
			upcoming = append(upcoming, e)
		}
	}
	slices.SortFunc(upcoming, func(a, b wanikani.SummaryEntry) int { return a.AvailableAt.Compare(b.AvailableAt) })
	total := d.Reviews
	for _, e := range upcoming {
		total += len(e.SubjectIDs)
		d.Forecast = append(d.Forecast, Hour{At: e.AvailableAt, Added: len(e.SubjectIDs), Total: total})
	}

	passed := map[int]bool{} // by subject ID
	for _, a := range assignments {
		s, ok := subjects[a.Data.SubjectID]
		if !ok || s.Data.HiddenAt != nil || a.Data.Hidden {
			continue
		}
		if a.Data.PassedAt != nil {
			passed[s.ID] = true
		}
		if a.Data.StartedAt != nil && a.Data.SRSStage > 0 {
			d.SRS[stageGroup(a.Data.SRSStage)]++
		}
	}
	for _, s := range subjects {
		if s.Data.Level != level || s.Data.HiddenAt != nil {
			continue
		}
		switch s.Object {
		case "radical":
			d.Progress.Radicals++
			if passed[s.ID] {
				d.Progress.RadicalsPassed++
			}
		case "kanji":
			d.Progress.Kanji++
			if passed[s.ID] {
				d.Progress.KanjiPassed++
			}
		}
	}
	d.Progress.KanjiNeeded = kanjiNeeded(d.Progress.Kanji)
	return d
}

// stageGroup maps an SRS stage (1-9) to its index in SRS.
func stageGroup(stage int) int {
	switch {
	case stage <= 4:
		return 0
	case stage <= 6:
		return 1
	default:
		return stage - 5 // 7 master, 8 enlightened, 9 burned
	}
}

// kanjiNeeded is 90% of n rounded up, in integer math: ceil(9n/10).
func kanjiNeeded(n int) int { return (n*9 + 9) / 10 }
```

- [ ] **Step 4:** `go test ./dashboard/ -v` passes.
- [ ] **Step 5:** Commit `dashboard: compute panels from summary, assignments, subjects`.

---

### Task 3: Backend: generic cache sync, assignments, Dashboard

**Files:** Modify `backend.go`, `backend_test.go`

**Interfaces:**
- Consumes: Tasks 1-2.
- Produces: `func (b *backend) Dashboard(ctx context.Context) (dashboard.Dashboard, error)`; `syncResources[T any](path string, version int, fetch func(since time.Time) ([]wanikani.Resource[T], error)) (map[int]wanikani.Resource[T], error)`; `func (b *backend) connect() error`.

**Go concepts:** a generic function with type inference from a function argument; closures adapting a method to a function type; extracting a helper (`connect`) shared by two callers.

- [ ] **Step 1: Failing test** (append to `backend_test.go`; add imports `fmt`, `time`)

```go
func TestDashboard(t *testing.T) {
	var assignmentQueries []string
	mux := http.NewServeMux()
	mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":{"level":1}}`)
	})
	mux.HandleFunc("/summary", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"data":{"lessons":[],"reviews":[{"available_at":%q,"subject_ids":[1]}]}}`,
			time.Now().Add(-time.Minute).UTC().Format(time.RFC3339))
	})
	mux.HandleFunc("/subjects", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"pages":{"next_url":null},"data":[{"id":1,"object":"kanji","data":{"level":1,"characters":"一"}}]}`)
	})
	mux.HandleFunc("/assignments", func(w http.ResponseWriter, r *http.Request) {
		assignmentQueries = append(assignmentQueries, r.URL.RawQuery)
		fmt.Fprint(w, `{"pages":{"next_url":null},"data":[{"id":9,"object":"assignment","data":{"subject_id":1,"srs_stage":5,"started_at":"2026-01-01T00:00:00Z","passed_at":"2026-01-02T00:00:00Z"}}]}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	b := &backend{dir: t.TempDir(), base: srv.URL + "/"}
	b.client = wanikani.NewClient(b.base, "tok")

	d, err := b.Dashboard(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if d.Level != 1 || d.Reviews != 1 || d.Progress.KanjiPassed != 1 || d.SRS[1] != 1 {
		t.Errorf("dashboard = %+v", d)
	}
	if _, err := b.Dashboard(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(assignmentQueries) != 2 || strings.Contains(assignmentQueries[0], "updated_after") ||
		!strings.Contains(assignmentQueries[1], "updated_after") {
		t.Errorf("assignment requests = %q; want a full sync, then an incremental one", assignmentQueries)
	}
}
```

- [ ] **Step 2:** `go test .` fails: `b.Dashboard undefined`.
- [ ] **Step 3: Implement.** In `backend.go`:
  - Set `const subjectCacheVersion = 3` (comment: bumped for `level` and `hidden_at`) and add `const assignmentCacheVersion = 1`.
  - Delete `subjectCache` and replace `syncSubjects` with:

```go
// resourceCache is the on-disk form of a synced collection.
type resourceCache[T any] struct {
	Version  int                          `json:"version"`
	SyncedAt time.Time                    `json:"synced_at"`
	Items    map[int]wanikani.Resource[T] `json:"items"`
}

// syncResources loads a cached collection from path, fetches what changed
// since the last sync (everything if the cache is missing, corrupt, or from
// another version), merges by ID, and saves it back.
func syncResources[T any](path string, version int,
	fetch func(since time.Time) ([]wanikani.Resource[T], error)) (map[int]wanikani.Resource[T], error) {
	var cache resourceCache[T]
	if err := store.ReadJSON(path, &cache); err != nil || cache.Version != version {
		cache = resourceCache[T]{Version: version}
	}
	started := time.Now()
	fresh, err := fetch(cache.SyncedAt)
	if err != nil {
		return nil, err
	}
	if cache.Items == nil {
		cache.Items = map[int]wanikani.Resource[T]{}
	}
	for _, r := range fresh {
		cache.Items[r.ID] = r
	}
	cache.SyncedAt = started
	return cache.Items, store.WriteJSON(path, cache)
}

func (b *backend) syncSubjects(ctx context.Context) (map[int]wanikani.Resource[wanikani.Subject], error) {
	return syncResources(filepath.Join(b.dir, "subjects.json"), subjectCacheVersion,
		func(since time.Time) ([]wanikani.Resource[wanikani.Subject], error) { return b.client.Subjects(ctx, since) })
}

func (b *backend) syncAssignments(ctx context.Context) (map[int]wanikani.Resource[wanikani.Assignment], error) {
	return syncResources(filepath.Join(b.dir, "assignments.json"), assignmentCacheVersion,
		func(since time.Time) ([]wanikani.Resource[wanikani.Assignment], error) { return b.client.Assignments(ctx, since) })
}
```

  - Extract the token block at the top of `Load` into `connect` and call it from `Load` (`if err := b.connect(); err != nil { return nil, 0, err }`):

```go
// connect makes sure there is a client, loading the saved token if needed.
func (b *backend) connect() error {
	if b.client != nil {
		return nil
	}
	tok, err := store.LoadToken()
	if errors.Is(err, store.ErrNoToken) {
		return wanikani.ErrUnauthorized
	}
	if err != nil {
		return err
	}
	b.client = wanikani.NewClient(b.base, tok)
	return nil
}
```

  - Add (import `github.com/ParkerSuzuki/durtle-tui/dashboard`):

```go
// Dashboard sends any saved answers, syncs subjects and assignments, and
// computes the home screen.
func (b *backend) Dashboard(ctx context.Context) (dashboard.Dashboard, error) {
	var none dashboard.Dashboard
	if err := b.connect(); err != nil {
		return none, err
	}
	if err := b.flushPending(ctx); err != nil {
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
	user, err := b.client.User(ctx)
	if err != nil {
		return none, err
	}
	sum, err := b.client.Summary(ctx)
	if err != nil {
		return none, err
	}
	return dashboard.Build(time.Now(), user.Level, sum, assignments, subjects), nil
}
```

- [ ] **Step 4:** `go test ./...` passes (including `TestOldSubjectCacheForcesFullSync`, which now exercises `syncResources`).
- [ ] **Step 5:** Commit `backend: generic cache sync, assignment cache, Dashboard`.

---

### Task 4: UI home screen

**Files:** Modify `ui/model.go`, `ui/view.go`, `ui/model_test.go`

**Interfaces:**
- Consumes: `dashboard.Dashboard`, `dashboard.Hour`, `dashboard.SRS`, `dashboard.StageNames`; `backend.Dashboard` (via interface).
- Produces: `Backend.Dashboard(ctx context.Context) (dashboard.Dashboard, error)`; screen `home`; `dashboardMsg`.

**Go concepts:** growing an interface and every implementation (compiler finds them all); sharing error handling in one method (`loadFailed`); fixed-width formatting verbs (`%-12s`, `%+5d`); `time.Time.Local` and layout strings (`"Mon 15:04"`).

- [ ] **Step 1: Failing tests.** In `ui/model_test.go`: add fields `dash dashboard.Dashboard; dashErr error` to `fakeBackend` and the method

```go
func (f *fakeBackend) Dashboard(context.Context) (dashboard.Dashboard, error) {
	return f.dash, f.dashErr
}
```

then append:

```go
var sampleDash = dashboard.Dashboard{Level: 12, Lessons: 5, Reviews: 67,
	Forecast: []dashboard.Hour{{At: time.Date(2026, 9, 30, 15, 0, 0, 0, time.Local), Added: 12, Total: 79}},
	Progress: dashboard.Progress{Radicals: 10, RadicalsPassed: 9, Kanji: 33, KanjiPassed: 21, KanjiNeeded: 30},
	SRS:      dashboard.SRS{88, 143, 97, 201, 12}}

func TestDashboardShowsPanels(t *testing.T) {
	for _, width := range []int{100, 30} { // 30: narrow terminals must not panic
		m, _ := step(t, New(&fakeBackend{}, false), tea.WindowSizeMsg{Width: width, Height: 40})
		m, _ = step(t, m, dashboardMsg{d: sampleDash})
		got := stripANSI(m.View().Content)
		for _, want := range []string{"Level 12", "Reviews 67", "21 / 33", "30 needed", "15:00", "+12", "79", "Apprentice", "143", "Burned"} {
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
```

(imports: add `github.com/ParkerSuzuki/durtle-tui/dashboard`.)

- [ ] **Step 2:** `go test ./ui/` fails to compile (`dashboardMsg` undefined, interface mismatch).
- [ ] **Step 3: Implement `ui/model.go`.**
  - Import `dashboard`. Add `Dashboard(ctx context.Context) (dashboard.Dashboard, error)` to `Backend`.
  - Add `home` to the `screen` constants (last). Add `dashboardMsg struct { d dashboard.Dashboard; err error }` to the message types.
  - Add fields `dash dashboard.Dashboard` and `loadingReviews bool // which load a retry repeats`.
  - `Init` returns `m.loadDashboard()`. Add:

```go
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
```

  - In `update`: add `case dashboardMsg:` that returns `m.loadFailed(msg.err)` on error, else sets `m.dash = msg.d; m.screen = home`. In `loginMsg` success, set `m.loadingReviews = false` and return `m.loadDashboard()` instead of `m.load()`. At the top of `case tea.KeyPressMsg:` add:

```go
		if m.screen == home {
			switch msg.String() {
			case "r", "enter":
				return m.startReviews()
			case "q", "esc", "ctrl+c":
				return m.quit()
			}
			return m, nil
		}
```

  - In `loaded`: replace the error `switch` with `if msg.err != nil { return m.loadFailed(msg.err) }`, and reset per-session counters before building the session: `m.pending, m.rejected, m.lost, m.lostErr = 0, 0, 0, nil`.
  - In `enter`: `case failed:` sets `m.screen, m.err = loading, nil` and returns `m.load()` if `m.loadingReviews`, else `m.loadDashboard()`. `case summary:` sets `m.screen, m.loadingReviews = loading, false` and returns `m.loadDashboard()`.

- [ ] **Step 4: Implement `ui/view.go`.** Import `dashboard`. In `View`, add `case home: body = m.homeView()`. In `summaryView`, change the hint to `"Enter for the dashboard, Esc to quit"`. Add:

```go
const (
	srsColor        = "#A8DADC"
	maxForecastRows = 8
)

func (m Model) homeView() string {
	d, p := m.dash, m.dash.Progress
	barW := max(m.innerWidth()-44, 5)
	lines := []string{
		title.Render("durtle-tui") + dim.Render(fmt.Sprintf("   Level %d", d.Level)),
		"",
		fmt.Sprintf("Lessons %-6d Reviews %d", d.Lessons, d.Reviews),
		"",
		fmt.Sprintf("%-18s %s  %d / %d   (%d needed to level up)", fmt.Sprintf("Level %d kanji", d.Level),
			progressBar(p.KanjiPassed, p.Kanji, barW, typeColors["kanji"]), p.KanjiPassed, p.Kanji, p.KanjiNeeded),
		fmt.Sprintf("%-18s %s  %d / %d", fmt.Sprintf("Level %d radicals", d.Level),
			progressBar(p.RadicalsPassed, p.Radicals, barW, typeColors["radical"]), p.RadicalsPassed, p.Radicals),
		"",
		"Upcoming reviews",
	}
	lines = append(lines, forecastLines(d.Forecast, barW)...)
	lines = append(lines, "")
	lines = append(lines, srsLines(d.SRS, barW)...)
	hint := "q quit"
	if d.Reviews > 0 {
		hint = "r start reviews   q quit"
	}
	lines = append(lines, "", dim.Render(hint))
	return strings.Join(lines, "\n")
}

// progressBar draws done out of total as width cells: filled in color, the rest dim.
func progressBar(done, total, width int, color string) string {
	filled := 0
	if total > 0 {
		filled = min(done*width/total, width)
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
```

- [ ] **Step 5:** `go test -race ./...` passes (existing review tests unchanged).
- [ ] **Step 6:** Commit `ui: dashboard home screen; reviews start from and return to it`.

---

### Task 5: Real run, docs

**Files:** Modify `README.md`, `TODO.md`, `docs/learning-go.md`

- [ ] **Step 1:** Build and run in tmux (never keys that answer or submit):

```bash
go build -o "$SCRATCH/durtle-tui" . && tmux new-session -d -s dash -x 100 -y 40 "$SCRATCH/durtle-tui"
sleep 15 && tmux capture-pane -t dash -p | head -40 ; tmux send-keys -t dash q
```

Expected: all four panels with real numbers (the first launch resyncs subjects and caches assignments). A second launch should load noticeably faster.
- [ ] **Step 2:** README status line mentions the dashboard; `TODO.md` marks milestone 2 done; `docs/learning-go.md` gains section 12 (generic `syncResources` and type inference from a function argument, interface growth, `slices.SortFunc` and `time.Compare`, integer ceiling division, format verbs).
- [ ] **Step 3:** Commit `docs: milestone 2 dashboard`.
