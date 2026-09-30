package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ParkerSuzuki/durtle-tui/review"
	"github.com/ParkerSuzuki/durtle-tui/store"
	"github.com/ParkerSuzuki/durtle-tui/wanikani"
)

// backend implements ui.Backend on top of the WaniKani API and local files.
type backend struct {
	dir    string // cache directory
	base   string // API base URL
	client *wanikani.Client
	mu     sync.Mutex // guards pending.json; submits run concurrently
}

type subjectCache struct {
	SyncedAt time.Time                                   `json:"synced_at"`
	Subjects map[int]wanikani.Resource[wanikani.Subject] `json:"subjects"`
}

type synonymCache struct {
	SyncedAt time.Time        `json:"synced_at"`
	Synonyms map[int][]string `json:"synonyms"` // by subject ID
}

// Login checks the token against the API and saves it.
func (b *backend) Login(ctx context.Context, token string) error {
	token = strings.TrimSpace(token)
	c := wanikani.NewClient(b.base, token)
	if _, err := c.User(ctx); err != nil {
		return err
	}
	if err := store.SaveToken(token); err != nil {
		return fmt.Errorf("saving token: %w", err)
	}
	b.client = c
	return nil
}

// Load sends any saved answers, syncs, and returns the items due for review.
// It returns wanikani.ErrUnauthorized when there is no token or it was rejected.
func (b *backend) Load(ctx context.Context) ([]review.Item, error) {
	if b.client == nil {
		tok, err := store.LoadToken()
		if errors.Is(err, store.ErrNoToken) {
			return nil, wanikani.ErrUnauthorized
		}
		if err != nil {
			return nil, err
		}
		b.client = wanikani.NewClient(b.base, tok)
	}
	if err := b.flushPending(ctx); err != nil {
		return nil, err
	}
	subjects, err := b.syncSubjects(ctx)
	if err != nil {
		return nil, err
	}
	synonyms, err := b.syncSynonyms(ctx)
	if err != nil {
		return nil, err
	}
	assignments, err := b.client.ReviewAssignments(ctx)
	if err != nil {
		return nil, err
	}
	return buildItems(assignments, subjects, synonyms), nil
}

func (b *backend) syncSubjects(ctx context.Context) (map[int]wanikani.Resource[wanikani.Subject], error) {
	path := filepath.Join(b.dir, "subjects.json")
	var cache subjectCache
	if err := store.ReadJSON(path, &cache); err != nil {
		cache = subjectCache{} // corrupt cache: start over with a full sync
	}
	started := time.Now()
	fresh, err := b.client.Subjects(ctx, cache.SyncedAt)
	if err != nil {
		return nil, err
	}
	if cache.Subjects == nil {
		cache.Subjects = map[int]wanikani.Resource[wanikani.Subject]{}
	}
	for _, s := range fresh {
		cache.Subjects[s.ID] = s
	}
	cache.SyncedAt = started
	return cache.Subjects, store.WriteJSON(path, cache)
}

func (b *backend) syncSynonyms(ctx context.Context) (map[int][]string, error) {
	path := filepath.Join(b.dir, "study_materials.json")
	var cache synonymCache
	if err := store.ReadJSON(path, &cache); err != nil {
		cache = synonymCache{}
	}
	started := time.Now()
	fresh, err := b.client.StudyMaterials(ctx, cache.SyncedAt)
	if err != nil {
		return nil, err
	}
	if cache.Synonyms == nil {
		cache.Synonyms = map[int][]string{}
	}
	for _, m := range fresh {
		cache.Synonyms[m.Data.SubjectID] = m.Data.MeaningSynonyms
	}
	cache.SyncedAt = started
	return cache.Synonyms, store.WriteJSON(path, cache)
}

// Submit sends one finished review. If it cannot be sent right now, it is
// saved to pending.json and pending is true. err is non-nil only when
// WaniKani refused the review or it could not be saved.
func (b *backend) Submit(ctx context.Context, s review.Submission) (pending bool, err error) {
	err = b.client.SubmitReview(ctx, s.AssignmentID, s.IncorrectMeaning, s.IncorrectReading)
	if err == nil {
		return false, nil
	}
	if rejected(err) {
		return false, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	var list []review.Submission
	if err := store.ReadJSON(b.pendingPath(), &list); err != nil {
		return false, err
	}
	if err := store.WriteJSON(b.pendingPath(), append(list, s)); err != nil {
		return false, err
	}
	return true, nil
}

// flushPending retries saved answers, keeping only those that still fail
// for a retryable reason.
func (b *backend) flushPending(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	var list []review.Submission
	if err := store.ReadJSON(b.pendingPath(), &list); err != nil {
		return fmt.Errorf("reading pending answers: %w", err)
	}
	if len(list) == 0 {
		return nil
	}
	var keep []review.Submission
	for _, s := range list {
		err := b.client.SubmitReview(ctx, s.AssignmentID, s.IncorrectMeaning, s.IncorrectReading)
		if errors.Is(err, wanikani.ErrUnauthorized) {
			return err // leave the file as is; retry after a new token
		}
		if err != nil && !rejected(err) {
			keep = append(keep, s)
		}
	}
	return store.WriteJSON(b.pendingPath(), keep)
}

func (b *backend) pendingPath() string { return filepath.Join(b.dir, "pending.json") }

// rejected reports whether WaniKani refused the review itself (a 4xx other
// than 401, 403 or 429), so retrying it later would never succeed. 403 means
// the token lacks reviews:create: the answer is fine, the token is not, so
// it is kept for after the user fixes the token.
func rejected(err error) bool {
	var apiErr *wanikani.APIError
	return errors.As(err, &apiErr) && apiErr.Status >= 400 && apiErr.Status < 500 &&
		apiErr.Status != 401 && apiErr.Status != 403 && apiErr.Status != 429
}

// buildItems joins due assignments with their subjects and the user's
// synonyms. Image-only radicals are skipped for now (decision 6).
func buildItems(assignments []wanikani.Resource[wanikani.Assignment],
	subjects map[int]wanikani.Resource[wanikani.Subject], synonyms map[int][]string) []review.Item {
	var items []review.Item
	for _, a := range assignments {
		s, ok := subjects[a.Data.SubjectID]
		if !ok || s.Data.Characters == nil {
			continue
		}
		it := review.Item{AssignmentID: a.ID, Type: s.Object, Characters: *s.Data.Characters}
		for _, m := range s.Data.Meanings {
			switch {
			case m.AcceptedAnswer && m.Primary:
				it.Meanings = append([]string{m.Meaning}, it.Meanings...)
			case m.AcceptedAnswer:
				it.Meanings = append(it.Meanings, m.Meaning)
			}
		}
		for _, aux := range s.Data.AuxiliaryMeanings {
			switch aux.Type {
			case "whitelist":
				it.Meanings = append(it.Meanings, aux.Meaning)
			case "blacklist":
				it.Blacklist = append(it.Blacklist, aux.Meaning)
			}
		}
		it.Meanings = append(it.Meanings, synonyms[s.ID]...)
		for _, r := range s.Data.Readings {
			switch {
			case r.AcceptedAnswer && r.Primary:
				it.Readings = append([]string{r.Reading}, it.Readings...)
				it.ReadingKind = readingKind(r.Type)
			case r.AcceptedAnswer:
				it.Readings = append(it.Readings, r.Reading)
			case s.Object == "kanji":
				it.OtherReadings = append(it.OtherReadings, r.Reading)
			}
		}
		items = append(items, it)
	}
	return items
}

func readingKind(t string) string {
	switch t {
	case "onyomi":
		return "on'yomi"
	case "kunyomi":
		return "kun'yomi"
	}
	return t // "nanori" or ""
}
