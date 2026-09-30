package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ParkerSuzuki/durtle-tui/dashboard"
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

// Cache versions change whenever the cached type gains fields, so an older
// cache (which never stored them) is refetched in full. Subjects went to 3
// for level and hidden_at.
const (
	subjectCacheVersion    = 3
	assignmentCacheVersion = 1
)

// radicalArtPx is the size radical images are rasterized to: 20 columns by
// 10 rows of half-block characters.
const radicalArtPx = 20

// resourceCache is the on-disk form of a synced collection.
type resourceCache[T any] struct {
	Version  int                          `json:"version"`
	SyncedAt time.Time                    `json:"synced_at"`
	Items    map[int]wanikani.Resource[T] `json:"items"`
}

type synonymCache struct {
	SyncedAt time.Time        `json:"synced_at"`
	Synonyms map[int][]string `json:"synonyms"` // by subject ID
}

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

// Load sends any saved answers, syncs, and returns the items due for review,
// plus how many image-only radicals had to be skipped. It returns
// wanikani.ErrUnauthorized when there is no token or it was rejected.
func (b *backend) Load(ctx context.Context) ([]review.Item, int, error) {
	if err := b.connect(); err != nil {
		return nil, 0, err
	}
	if err := b.flushPending(ctx); err != nil {
		return nil, 0, err
	}
	subjects, err := b.syncSubjects(ctx)
	if err != nil {
		return nil, 0, err
	}
	synonyms, err := b.syncSynonyms(ctx)
	if err != nil {
		return nil, 0, err
	}
	assignments, err := b.client.ReviewAssignments(ctx)
	if err != nil {
		return nil, 0, err
	}
	art := func(s wanikani.Resource[wanikani.Subject]) image.Image { return b.radicalImage(ctx, s) }
	items, skipped := buildItems(assignments, subjects, synonyms, art)
	return items, skipped, nil
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
		func(since time.Time) ([]wanikani.Resource[wanikani.Subject], error) {
			return b.client.Subjects(ctx, since)
		})
}

func (b *backend) syncAssignments(ctx context.Context) (map[int]wanikani.Resource[wanikani.Assignment], error) {
	return syncResources(filepath.Join(b.dir, "assignments.json"), assignmentCacheVersion,
		func(since time.Time) ([]wanikani.Resource[wanikani.Assignment], error) {
			return b.client.Assignments(ctx, since)
		})
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
// synonyms. Radicals with no Unicode character get a picture from art; when
// art returns nil they are skipped and counted.
func buildItems(assignments []wanikani.Resource[wanikani.Assignment],
	subjects map[int]wanikani.Resource[wanikani.Subject], synonyms map[int][]string,
	art func(wanikani.Resource[wanikani.Subject]) image.Image) (items []review.Item, skipped int) {
	for _, a := range assignments {
		s, ok := subjects[a.Data.SubjectID]
		if !ok {
			continue
		}
		it := review.Item{AssignmentID: a.ID, Type: s.Object}
		if s.Data.Characters != nil {
			it.Characters = *s.Data.Characters
		} else if it.Image = art(s); it.Image == nil {
			skipped++
			continue
		}
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
	return items, skipped
}

// radicalImage returns the picture for a radical with no Unicode character:
// its SVG, downloaded once into the cache, rasterized by rsvg-convert (see
// decision 20). It returns nil when that is not possible, for example when
// rsvg-convert is not installed or the download fails.
func (b *backend) radicalImage(ctx context.Context, s wanikani.Resource[wanikani.Subject]) image.Image {
	var url string
	for _, ci := range s.Data.CharacterImages {
		if ci.ContentType == "image/svg+xml" {
			url = ci.URL
		}
	}
	if url == "" {
		return nil
	}
	path := filepath.Join(b.dir, "radicals", fmt.Sprintf("%d.svg", s.ID))
	if _, err := os.Stat(path); err != nil {
		if err := download(ctx, url, path); err != nil {
			return nil
		}
	}
	size := fmt.Sprint(radicalArtPx)
	out, err := exec.CommandContext(ctx, "rsvg-convert", "-w", size, "-h", size, path).Output()
	if err != nil {
		return nil
	}
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		return nil
	}
	return img
}

// download saves url to path. It sends no API token: images live on a
// public file host, and the token should only ever go to the API.
func download(ctx context.Context, url, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading %s: %s", url, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp" // write then rename: a cut-off download never looks cached
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
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
