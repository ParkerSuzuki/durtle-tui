package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
	items := buildItems(assignments, subjects, map[int][]string{1: {"massive"}})
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
	if _, err := b.Load(context.Background()); !errors.Is(err, wanikani.ErrUnauthorized) {
		t.Errorf("err = %v, want ErrUnauthorized", err)
	}
}
