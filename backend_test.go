package main

import (
	"context"
	"errors"
	"image"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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
	b.client = wanikani.NewClient(b.base, "tok")
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
	if got := readPending(t, b); len(got) != 1 || got[0] != sub {
		t.Errorf("pending.json = %v", got)
	}

	b, _ = fakeAPI(t, http.StatusUnauthorized)
	if pending, _ := b.Submit(ctx, sub); !pending {
		t.Error("401: answer must be saved, not lost")
	}

	b, _ = fakeAPI(t, http.StatusForbidden)
	if pending, _ := b.Submit(ctx, sub); !pending {
		t.Error("403 (token lacks reviews:create): answer must be saved, not lost")
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
	b.client = wanikani.NewClient(b.base, "tok")
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
