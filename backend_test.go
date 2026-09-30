package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ParkerSuzuki/durtle-tui/lessons"
	"github.com/ParkerSuzuki/durtle-tui/review"
	"github.com/ParkerSuzuki/durtle-tui/store"
	"github.com/ParkerSuzuki/durtle-tui/wanikani"
	"github.com/zalando/go-keyring"
)

func ptr(s string) *string { return &s }

func TestBuildItems(t *testing.T) {
	subjects := map[int]wanikani.Resource[wanikani.Subject]{
		1: {ID: 1, Object: "kanji", Data: wanikani.Subject{
			Characters:        ptr("大"),
			Meanings:          []wanikani.Meaning{{Meaning: "Large", AcceptedAnswer: true}, {Meaning: "Big", Primary: true, AcceptedAnswer: true}},
			AuxiliaryMeanings: []wanikani.AuxMeaning{{Meaning: "Huge", Type: "whitelist"}, {Meaning: "Grand", Type: "blacklist"}},
			Readings: []wanikani.Reading{
				{Reading: "たい", Primary: true, AcceptedAnswer: true, Type: "onyomi"},
				{Reading: "おお", Type: "kunyomi"},
			},
		}},
		2: {ID: 2, Object: "radical", Data: wanikani.Subject{Characters: nil}},
	}
	assignments := []wanikani.Resource[wanikani.Assignment]{
		{ID: 10, Data: wanikani.Assignment{SubjectID: 1}},
		{ID: 20, Data: wanikani.Assignment{SubjectID: 2}},  // image-only radical: skipped
		{ID: 30, Data: wanikani.Assignment{SubjectID: 99}}, // unknown subject: skipped
	}
	items, _ := buildItems(assignments, subjects, map[int][]string{1: {"massive"}}, func(wanikani.Resource[wanikani.Subject]) image.Image { return nil })
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	it := items[0]
	if it.AssignmentID != 10 || it.Meanings[0] != "Big" || it.ReadingKind != "on'yomi" {
		t.Errorf("item = %+v", it)
	}
	for _, want := range []string{"Large", "Huge", "massive"} {
		if review.GradeMeaning(it, want).Verdict != review.Correct {
			t.Errorf("%q should be accepted; meanings = %v", want, it.Meanings)
		}
	}
	if len(it.Blacklist) != 1 || len(it.OtherReadings) != 1 {
		t.Errorf("blacklist %v, other readings %v", it.Blacklist, it.OtherReadings)
	}
}

// fakeAPI answers POST /reviews with the next status from statuses.
func fakeAPI(t *testing.T, statuses ...int) (*backend, *int) {
	t.Helper()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statuses[calls%len(statuses)])
		calls++
	}))
	t.Cleanup(srv.Close)
	b := &backend{dir: t.TempDir(), base: srv.URL + "/"}
	b.client.Store(wanikani.NewClient(b.base, "tok"))
	return b, &calls
}

func readPending(t *testing.T, b *backend) []review.Submission {
	t.Helper()
	var list []review.Submission
	if err := store.ReadJSON(filepath.Join(b.dir, "pending.json"), &list); err != nil {
		t.Fatal(err)
	}
	return list
}

func TestSubmitPendingAndRejected(t *testing.T) {
	ctx := context.Background()
	sub := review.Submission{AssignmentID: 5, IncorrectMeaning: 1}

	b, _ := fakeAPI(t, http.StatusCreated)
	if pending, err := b.Submit(ctx, sub); pending || err != nil {
		t.Errorf("success: pending=%v err=%v", pending, err)
	}

	b, _ = fakeAPI(t, http.StatusInternalServerError)
	if pending, err := b.Submit(ctx, sub); !pending || err != nil {
		t.Errorf("5xx: pending=%v err=%v, want saved for later", pending, err)
	}
	if got := readPending(t, b); len(got) != 1 || got[0].AssignmentID != sub.AssignmentID || got[0].IncorrectMeaning != sub.IncorrectMeaning {
		t.Errorf("pending.json = %v", got)
	}

	b, _ = fakeAPI(t, http.StatusUnauthorized)
	if pending, _ := b.Submit(ctx, sub); !pending {
		t.Error("401: answer must be saved, not lost")
	}

	b, _ = fakeAPI(t, http.StatusForbidden)
	pending, err := b.Submit(ctx, sub)
	var apiErr *wanikani.APIError
	if !pending || !errors.As(err, &apiErr) || apiErr.Status != 403 {
		t.Errorf("403: pending=%v err=%v; want saved, and the 403 reported so the UI can explain it", pending, err)
	}

	b, _ = fakeAPI(t, http.StatusUnprocessableEntity)
	if pending, err := b.Submit(ctx, sub); pending || err == nil {
		t.Errorf("422: pending=%v err=%v, want rejected and not queued", pending, err)
	}
}

func TestFlushPending(t *testing.T) {
	b, calls := fakeAPI(t, http.StatusCreated, http.StatusInternalServerError, http.StatusUnprocessableEntity)
	list := []review.Submission{{AssignmentID: 1}, {AssignmentID: 2}, {AssignmentID: 3}}
	if err := store.WriteJSON(filepath.Join(b.dir, "pending.json"), list); err != nil {
		t.Fatal(err)
	}
	if err := b.flushPending(context.Background()); err != nil {
		t.Fatal(err)
	}
	if *calls != 3 {
		t.Errorf("calls = %d, want 3", *calls)
	}
	if got := readPending(t, b); len(got) != 1 || got[0].AssignmentID != 2 {
		t.Errorf("kept %v, want only the 5xx one (id 2)", got)
	}
}

func TestLoadWithoutTokenAsksForOne(t *testing.T) {
	keyring.MockInit()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	b := &backend{dir: t.TempDir(), base: "http://unused/"}
	if _, _, err := b.Load(context.Background()); !errors.Is(err, wanikani.ErrUnauthorized) {
		t.Errorf("err = %v, want ErrUnauthorized", err)
	}
}

// A cache written before Subject gained fields (like character images) must
// be refetched in full: an incremental sync would never fill them in.
func TestOldSubjectCacheForcesFullSync(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Write([]byte(`{"pages":{"next_url":null},"data":[]}`))
	}))
	t.Cleanup(srv.Close)
	b := &backend{dir: t.TempDir(), base: srv.URL + "/"}
	b.client.Store(wanikani.NewClient(b.base, "tok"))
	old := map[string]any{"synced_at": "2026-09-01T00:00:00Z", "subjects": map[string]any{}}
	if err := store.WriteJSON(filepath.Join(b.dir, "subjects.json"), old); err != nil {
		t.Fatal(err)
	}
	if _, err := b.syncSubjects(context.Background()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(gotQuery, "updated_after") {
		t.Errorf("old cache format synced incrementally (%q); want a full sync", gotQuery)
	}
}

const testSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1000 1000"><path d="M100 500H900" style="fill:none;stroke:#000;stroke-width:200px"/></svg>`

func TestRadicalImage(t *testing.T) {
	if _, err := exec.LookPath("rsvg-convert"); err != nil {
		t.Skip("rsvg-convert not installed")
	}
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.Header.Get("Authorization") != "" {
			t.Error("the API token must not be sent to the image host")
		}
		w.Write([]byte(testSVG))
	}))
	t.Cleanup(srv.Close)
	b := &backend{dir: t.TempDir()}
	rad := wanikani.Resource[wanikani.Subject]{ID: 7, Object: "radical", Data: wanikani.Subject{
		CharacterImages: []wanikani.CharacterImage{{URL: srv.URL + "/png", ContentType: "image/png"}, {URL: srv.URL + "/svg", ContentType: "image/svg+xml"}},
	}}
	img := b.radicalImage(context.Background(), rad)
	if img == nil {
		t.Fatal("no image")
	}
	if got := img.Bounds().Dx(); got != radicalArtPx {
		t.Errorf("image is %d px wide, want %d", got, radicalArtPx)
	}
	if _, _, _, a := img.At(radicalArtPx/2, radicalArtPx/2).RGBA(); a == 0 {
		t.Error("the stroke through the middle is missing")
	}
	srv.Close()
	if b.radicalImage(context.Background(), rad) == nil || hits != 1 {
		t.Errorf("second call should come from the cache (hits = %d)", hits)
	}
}

func TestBuildItemsImageRadicals(t *testing.T) {
	subjects := map[int]wanikani.Resource[wanikani.Subject]{
		2: {ID: 2, Object: "radical", Data: wanikani.Subject{
			Meanings: []wanikani.Meaning{{Meaning: "Beggar", Primary: true, AcceptedAnswer: true}}}},
	}
	assignments := []wanikani.Resource[wanikani.Assignment]{{ID: 20, Data: wanikani.Assignment{SubjectID: 2}}}
	pic := image.NewAlpha(image.Rect(0, 0, 2, 2))

	items, skipped := buildItems(assignments, subjects, nil, func(wanikani.Resource[wanikani.Subject]) image.Image { return pic })
	if len(items) != 1 || items[0].Image != pic || items[0].Meanings[0] != "Beggar" || skipped != 0 {
		t.Errorf("with an image: items %+v, skipped %d", items, skipped)
	}
	items, skipped = buildItems(assignments, subjects, nil, func(wanikani.Resource[wanikani.Subject]) image.Image { return nil })
	if len(items) != 0 || skipped != 1 {
		t.Errorf("without an image: %d items, skipped %d; want 0 and 1", len(items), skipped)
	}
}

func TestDashboard(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // Dashboard writes settings.json
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
		fmt.Fprint(w, `{"pages":{"next_url":null},"data":[{"id":9,"object":"assignment","data_updated_at":"2026-01-02T00:00:00Z","data":{"subject_id":1,"srs_stage":5,"started_at":"2026-01-01T00:00:00Z","passed_at":"2026-01-02T00:00:00Z"}}]}`)
	})
	mux.HandleFunc("/review_statistics", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"pages":{"next_url":null},"data":[]}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	b := &backend{dir: t.TempDir(), base: srv.URL + "/"}
	b.client.Store(wanikani.NewClient(b.base, "tok"))

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

// An assignment cache from another format version is refetched in full.
func TestOldAssignmentCacheForcesFullSync(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Write([]byte(`{"pages":{"next_url":null},"data":[]}`))
	}))
	t.Cleanup(srv.Close)
	b := &backend{dir: t.TempDir(), base: srv.URL + "/"}
	b.client.Store(wanikani.NewClient(b.base, "tok"))
	old := map[string]any{"version": 0, "synced_at": "2026-09-01T00:00:00Z", "items": map[string]any{}}
	if err := store.WriteJSON(filepath.Join(b.dir, "assignments.json"), old); err != nil {
		t.Fatal(err)
	}
	if _, err := b.syncAssignments(context.Background()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(gotQuery, "updated_after") {
		t.Errorf("old cache synced incrementally (%q); want a full sync", gotQuery)
	}
}

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
	mux.HandleFunc("/review_statistics", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"pages":{"next_url":null},"data":[]}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	b := &backend{dir: t.TempDir(), base: srv.URL + "/"}
	b.client.Store(wanikani.NewClient(b.base, "tok"))
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
	if d, err := b.Dashboard(context.Background()); err != nil || d.LessonsToday != 3 {
		t.Errorf("dashboard promises %d lessons (%v); want 3: the undrawable radical must not count", d.LessonsToday, err)
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
	b.client.Store(wanikani.NewClient(b.base, "tok"))
	if err := b.StartLesson(context.Background(), 7); err != nil || path != "PUT /assignments/7/start" {
		t.Errorf("StartLesson: %q, %v", path, err)
	}
}

// Accuracy comes from how WaniKani's answer counters change between syncs:
// the first sync is only a baseline, later increases count toward today.
func TestAccuracyAcrossSyncs(t *testing.T) {
	correct := 10
	mux := http.NewServeMux()
	mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"data":{"level":1}}`) })
	mux.HandleFunc("/summary", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"data":{"lessons":[],"reviews":[]}}`) })
	for _, p := range []string{"/subjects", "/assignments"} {
		mux.HandleFunc(p, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"pages":{"next_url":null},"data":[]}`) })
	}
	mux.HandleFunc("/review_statistics", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"pages":{"next_url":null},"data":[{"id":1,"object":"review_statistic","data_updated_at":%q,
			"data":{"subject_id":1,"meaning_correct":%d,"meaning_incorrect":1,"reading_correct":0,"reading_incorrect":0}}]}`,
			time.Now().UTC().Format(time.RFC3339), correct)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	b := &backend{dir: t.TempDir(), base: srv.URL + "/"}
	b.client.Store(wanikani.NewClient(b.base, "tok"))

	d, err := b.Dashboard(context.Background())
	if err != nil || d.Today.Total() != 0 {
		t.Fatalf("first sync is a baseline: today %+v, %v", d.Today, err)
	}
	correct = 13 // three more correct meaning answers since
	if d, err = b.Dashboard(context.Background()); err != nil || d.Today.Correct != 3 || d.Today.Incorrect != 0 {
		t.Errorf("today = %+v, %v; want 3 correct", d.Today, err)
	}
}

// Incremental sync resumes from WaniKani's own newest timestamp (full
// precision), minus an overlap so an item updated mid-sync is not skipped.
// Re-fetching unchanged items must not rewrite the ~15 MB cache.
func TestSyncCursorAndChangeDetection(t *testing.T) {
	newest := time.Date(2026, 1, 2, 3, 4, 5, 123456000, time.UTC)
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		after := r.URL.Query().Get("updated_after")
		queries = append(queries, after)
		since, _ := time.Parse(time.RFC3339Nano, after)
		if after == "" || newest.After(since) { // the server compares at full precision
			fmt.Fprintf(w, `{"pages":{"next_url":null},"data":[{"id":1,"object":"kanji","data_updated_at":%q,"data":{}}]}`,
				newest.Format(time.RFC3339Nano))
			return
		}
		fmt.Fprint(w, `{"pages":{"next_url":null},"data":[]}`)
	}))
	t.Cleanup(srv.Close)
	b := &backend{dir: t.TempDir(), base: srv.URL + "/"}
	b.client.Store(wanikani.NewClient(b.base, "tok"))
	if _, err := b.syncSubjects(context.Background()); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(b.dir, "subjects.json")
	past := time.Now().Add(-time.Hour)
	os.Chtimes(path, past, past)
	if _, err := b.syncSubjects(context.Background()); err != nil {
		t.Fatal(err)
	}
	if want := newest.Add(-syncOverlap).Format(time.RFC3339Nano); queries[1] != want {
		t.Errorf("second sync asked updated_after=%q, want %q", queries[1], want)
	}
	if info, _ := os.Stat(path); !info.ModTime().Equal(past) {
		t.Errorf("a sync that changed nothing rewrote the cache")
	}
}

// If the token is rejected partway through resending saved answers, the
// ones already accepted must not stay queued.
func TestFlushPendingStopsOn401WithoutResending(t *testing.T) {
	b, _ := fakeAPI(t, http.StatusCreated, http.StatusUnauthorized, http.StatusCreated)
	list := []review.Submission{{AssignmentID: 1}, {AssignmentID: 2}, {AssignmentID: 3}}
	if err := store.WriteJSON(filepath.Join(b.dir, "pending.json"), list); err != nil {
		t.Fatal(err)
	}
	if err := b.flushPending(context.Background()); !errors.Is(err, wanikani.ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
	if got := readPending(t, b); len(got) != 2 || got[0].AssignmentID != 2 || got[1].AssignmentID != 3 {
		t.Errorf("kept %v, want 2 and 3 (1 was already accepted)", got)
	}
}

// An answer is on disk before it is sent (so killing the app mid-send loses
// nothing) and gone from disk once WaniKani has it.
func TestSubmitWritesAhead(t *testing.T) {
	var onDisk []review.Submission
	var b *backend
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		onDisk = readPending(t, b)
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(srv.Close)
	b = &backend{dir: t.TempDir(), base: srv.URL + "/"}
	b.client.Store(wanikani.NewClient(b.base, "tok"))
	sub := review.Submission{AssignmentID: 9, IncorrectMeaning: 1}
	if pending, err := b.Submit(context.Background(), sub); pending || err != nil {
		t.Fatalf("pending=%v err=%v", pending, err)
	}
	if len(onDisk) != 1 || onDisk[0].AssignmentID != sub.AssignmentID || onDisk[0].IncorrectMeaning != sub.IncorrectMeaning || onDisk[0].CompletedAt.IsZero() {
		t.Errorf("while sending, pending.json = %v; want the answer saved first", onDisk)
	}
	if got := readPending(t, b); len(got) != 0 {
		t.Errorf("after success, pending.json = %v; want empty", got)
	}
}

// Settings files: hand edits keep their values and missing fields get
// defaults; a corrupt file is backed up, not silently lost; an unwritable
// config dir does not stop the dashboard.
func TestSettingsFileHandling(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "durtle-tui", "settings.json")
	b := &backend{dir: t.TempDir()}

	os.MkdirAll(filepath.Dir(path), 0o700)
	os.WriteFile(path, []byte(`{"daily_cap":1}`), 0o600)
	s, err := b.loadSettings(4)
	if err != nil || s.DailyCap != 1 || s.BatchSize != 4 || !s.Types.Kanji {
		t.Errorf("partial file: %+v, %v; want cap 1 kept, batch 4 from WaniKani, types on", s, err)
	}

	os.WriteFile(path, []byte(`{not json`), 0o600)
	if s, err = b.loadSettings(4); err != nil || s.DailyCap != 10 {
		t.Errorf("corrupt file: %+v, %v; want defaults", s, err)
	}
	if bad, _ := os.ReadFile(path + ".bad"); string(bad) != "{not json" {
		t.Errorf("corrupt file not backed up: %q", bad)
	}

	os.Remove(path)
	b.wkBatch = 6 // learned from an earlier dashboard load
	if s, _ = b.Settings(); s.BatchSize != 6 {
		t.Errorf("settings screen seeded batch %d, want the WaniKani value 6", s.BatchSize)
	}

	os.Remove(path)
	os.Chmod(filepath.Dir(path), 0o500) // read-only config dir
	t.Cleanup(func() { os.Chmod(filepath.Dir(path), 0o700) })
	if s, err = b.loadSettings(4); err != nil || s.DailyCap != 10 {
		t.Errorf("unwritable config dir: %+v, %v; want defaults in memory, no error", s, err)
	}
}

// Saved answers carry their completion time; only resends send it.
func TestResendCarriesCompletionTime(t *testing.T) {
	var createdAt []any
	status := http.StatusInternalServerError
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Review map[string]any `json:"review"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		createdAt = append(createdAt, body.Review["created_at"])
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	b := &backend{dir: t.TempDir(), base: srv.URL + "/"}
	b.client.Store(wanikani.NewClient(b.base, "tok"))
	if pending, _ := b.Submit(context.Background(), review.Submission{AssignmentID: 4}); !pending {
		t.Fatal("5xx should leave the answer pending")
	}
	saved := readPending(t, b)
	if len(saved) != 1 || saved[0].CompletedAt.IsZero() {
		t.Fatalf("pending.json = %+v; want the completion time recorded", saved)
	}
	status = http.StatusCreated
	if err := b.flushPending(context.Background()); err != nil {
		t.Fatal(err)
	}
	if createdAt[0] != nil || createdAt[1] != saved[0].CompletedAt.Format(time.RFC3339Nano) {
		t.Errorf("created_at sent = %v; want none on the first send, the saved time on the resend", createdAt)
	}
	if got := readPending(t, b); len(got) != 0 {
		t.Errorf("after the resend, pending.json = %v", got)
	}
}

// Logging in (token re-entry) while answers are sending must not race on the
// client; run with -race.
func TestLoginDuringSubmitIsRaceFree(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	keyring.MockInit()
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/reviews" {
			<-release
		}
		fmt.Fprint(w, `{"data":{"level":1}}`)
	}))
	t.Cleanup(srv.Close)
	b := &backend{dir: t.TempDir(), base: srv.URL + "/"}
	b.client.Store(wanikani.NewClient(b.base, "old"))
	done := make(chan struct{})
	go func() {
		b.Submit(context.Background(), review.Submission{AssignmentID: 1})
		close(done)
	}()
	if err := b.Login(context.Background(), "new"); err != nil {
		t.Fatal(err)
	}
	close(release)
	<-done
}
