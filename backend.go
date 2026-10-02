package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ParkerSuzuki/durtle-tui/dashboard"
	"github.com/ParkerSuzuki/durtle-tui/lessons"
	"github.com/ParkerSuzuki/durtle-tui/review"
	"github.com/ParkerSuzuki/durtle-tui/store"
	"github.com/ParkerSuzuki/durtle-tui/wanikani"
)

// backend implements ui.Backend on top of the WaniKani API and local files.
type backend struct {
	dir    string                          // cache directory
	base   string                          // API base URL
	client atomic.Pointer[wanikani.Client] // swapped by Login while commands run
	mu     sync.Mutex                      // guards pending.json and wkBatch; commands run concurrently

	wkBatch int // the user's WaniKani lesson batch size, once a sync has seen it

	// details is the teaching content of the items last loaded (reviews,
	// lessons, or practice), by assignment ID, for the item-info key.
	details map[int]lessons.Lesson
}

// Cache versions change whenever the cached type gains fields, so an older
// cache (which never stored them) is refetched in full. Subjects went to 3
// for level and hidden_at, and 4 for the teaching fields.
const (
	subjectCacheVersion    = 4
	assignmentCacheVersion = 1
	statsCacheVersion      = 1
)

// radicalArtPx is the size radical images are rasterized to: 20 columns by
// 10 rows of half-block characters.
const radicalArtPx = 20

// radicalPNGPx is the size of the sharper PNG that kitty draws over the same
// 20x10 cell area (decision 30).
const radicalPNGPx = 160

// resourceCache is the on-disk form of a synced collection.
type resourceCache[T any] struct {
	Version int                          `json:"version"`
	Items   map[int]wanikani.Resource[T] `json:"items"`
}

// syncOverlap re-asks for this much before the newest cached timestamp, so an
// item updated while an earlier sync was paging through is not skipped.
const syncOverlap = 5 * time.Minute

type synonymCache struct {
	SyncedAt time.Time        `json:"synced_at"`
	Synonyms map[int][]string `json:"synonyms"` // by subject ID
}

// syncAccuracy syncs WaniKani's per-subject answer counters and turns every
// increase into answers on the day it happened (decision 29). Daily totals
// live in accuracy.json, pruned to the last 30 days.
func (b *backend) syncAccuracy(ctx context.Context) (dashboard.Days, dashboard.Mistakes, error) {
	daysPath := filepath.Join(b.dir, "accuracy.json")
	days := dashboard.Days{}
	if err := store.ReadJSON(daysPath, &days); err != nil || days == nil {
		days = dashboard.Days{} // corrupt: start the history over
	}
	mistakes := b.readMistakes()
	_, err := syncResources(filepath.Join(b.dir, "review_statistics.json"), statsCacheVersion,
		func(since time.Time) ([]wanikani.Resource[wanikani.ReviewStatistic], error) {
			return b.api().ReviewStatistics(ctx, since)
		},
		func(old map[int]wanikani.Resource[wanikani.ReviewStatistic], fresh []wanikani.Resource[wanikani.ReviewStatistic]) {
			dashboard.AddDeltas(days, old, fresh, time.Local)
			dashboard.AddMistakes(mistakes, old, fresh)
		})
	if err != nil {
		return nil, nil, err
	}
	days.Prune(time.Now(), 30)
	if err := store.WriteJSON(daysPath, days); err != nil {
		return nil, nil, err
	}
	return days, mistakes, store.WriteJSON(b.mistakesPath(), mistakes)
}

func (b *backend) mistakesPath() string { return filepath.Join(b.dir, "mistakes.json") }

// readMistakes loads recent mistakes, already pruned to the last 24 hours.
func (b *backend) readMistakes() dashboard.Mistakes {
	m := dashboard.Mistakes{}
	if err := store.ReadJSON(b.mistakesPath(), &m); err != nil || m == nil {
		m = dashboard.Mistakes{} // corrupt: start over
	}
	m.Prune(time.Now())
	return m
}

// Mistakes returns the items answered wrong in the last 24 hours, as recorded
// by the last dashboard sync, for a practice session that sends nothing to
// WaniKani (decision 31).
func (b *backend) Mistakes(ctx context.Context) (items []review.Item, skipped int, err error) {
	if err := b.connect(); err != nil {
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
	// buildItems works from assignments; practice needs no real assignment,
	// so each mistake becomes a stand-in one that only names its subject.
	var missed []wanikani.Resource[wanikani.Assignment]
	for id := range b.readMistakes() {
		missed = append(missed, wanikani.Resource[wanikani.Assignment]{ID: id, Data: wanikani.Assignment{SubjectID: id}})
	}
	art := func(s wanikani.Resource[wanikani.Subject]) (image.Image, []byte) { return b.radicalArt(ctx, s) }
	items, skipped = buildItems(missed, subjects, synonyms, art)
	byID := make(map[int]wanikani.Resource[wanikani.Assignment], len(missed))
	for _, a := range missed {
		byID[a.ID] = a
	}
	b.remember(buildLessons(items, byID, subjects))
	return items, skipped, nil
}

// newest is the latest DataUpdatedAt among rs, or since when none is later.
// The next incremental sync asks for changes after it: WaniKani's own clock,
// so a skewed local clock cannot skip updates.
func newest[T any](since time.Time, rs []wanikani.Resource[T]) time.Time {
	for _, r := range rs {
		if r.DataUpdatedAt.After(since) {
			since = r.DataUpdatedAt
		}
	}
	return since
}

// api is the current WaniKani client.
func (b *backend) api() *wanikani.Client { return b.client.Load() }

// connect makes sure there is a client, loading the saved token if needed.
func (b *backend) connect() error {
	if b.client.Load() != nil {
		return nil
	}
	tok, err := store.LoadToken()
	if errors.Is(err, store.ErrNoToken) {
		return wanikani.ErrUnauthorized
	}
	if err != nil {
		return err
	}
	b.client.Store(wanikani.NewClient(b.base, tok))
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
	user, err := b.api().User(ctx)
	if err != nil {
		return none, err
	}
	sum, err := b.api().Summary(ctx)
	if err != nil {
		return none, err
	}
	settings, err := b.loadSettings(user.Preferences.LessonsBatchSize)
	if err != nil {
		return none, err
	}
	now := time.Now()
	d := dashboard.Build(now, user.Level, sum, assignments, subjects)
	days, mistakes, err := b.syncAccuracy(ctx)
	if err != nil {
		return none, err
	}
	d.Mistakes = len(mistakes)
	d.Today = days[now.Format(time.DateOnly)]
	d.Yesterday = days[now.AddDate(0, 0, -1).Format(time.DateOnly)]
	d.LessonsToday = len(lessons.Pick(lessonCandidates(now, sum, assignments, subjects, b.drawable(ctx)),
		lessons.StartedToday(now, startTimes(assignments)), settings))
	return d, nil
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
	b.client.Store(c)
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
	assignments, err := b.api().ReviewAssignments(ctx)
	if err != nil {
		return nil, 0, err
	}
	art := func(s wanikani.Resource[wanikani.Subject]) (image.Image, []byte) { return b.radicalArt(ctx, s) }
	items, skipped := buildItems(assignments, subjects, synonyms, art)
	byID := make(map[int]wanikani.Resource[wanikani.Assignment], len(assignments))
	for _, a := range assignments {
		byID[a.ID] = a
	}
	b.remember(buildLessons(items, byID, subjects))
	return items, skipped, nil
}

// remember keeps the teaching content of freshly loaded items.
func (b *backend) remember(ls []lessons.Lesson) {
	details := make(map[int]lessons.Lesson, len(ls))
	for _, l := range ls {
		details[l.AssignmentID] = l
	}
	b.mu.Lock()
	b.details = details
	b.mu.Unlock()
}

// Details returns the teaching content (mnemonics, readings, components,
// sentences) of an item from the last load.
func (b *backend) Details(assignmentID int) (lessons.Lesson, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	l, ok := b.details[assignmentID]
	return l, ok
}

// syncResources loads a cached collection from path, fetches what changed
// since the cache's newest server timestamp (everything if the cache is
// missing, corrupt, or from another version), merges by ID, and saves it
// back when anything actually changed. The cursor comes from WaniKani's own
// data_updated_at values, so the local clock never matters.
//
// before, when not nil, sees the cached items and the fetched changes before
// they are merged: the only moment both old and new values are available.
func syncResources[T any](path string, version int,
	fetch func(since time.Time) ([]wanikani.Resource[T], error),
	before func(old map[int]wanikani.Resource[T], fresh []wanikani.Resource[T])) (map[int]wanikani.Resource[T], error) {
	var cache resourceCache[T]
	if err := store.ReadJSON(path, &cache); err != nil || cache.Version != version {
		cache = resourceCache[T]{Version: version}
	}
	var since time.Time
	for _, r := range cache.Items {
		if r.DataUpdatedAt.After(since) {
			since = r.DataUpdatedAt
		}
	}
	if !since.IsZero() {
		since = since.Add(-syncOverlap)
	}
	fresh, err := fetch(since)
	if err != nil {
		return nil, err
	}
	if before != nil {
		before(cache.Items, fresh)
	}
	if cache.Items == nil {
		cache.Items = map[int]wanikani.Resource[T]{}
	}
	changed := false
	for _, r := range fresh {
		if old, ok := cache.Items[r.ID]; !ok || !old.DataUpdatedAt.Equal(r.DataUpdatedAt) {
			changed = true
		}
		cache.Items[r.ID] = r
	}
	if !changed {
		// The overlap re-fetches recent items; only rewrite when something is
		// new (subjects.json is ~15 MB).
		// ponytail: the file is still read on every refresh (~0.25 s for
		// subjects); keep the maps in memory if refreshes ever feel slow.
		return cache.Items, nil
	}
	return cache.Items, store.WriteJSON(path, cache)
}

func (b *backend) syncSubjects(ctx context.Context) (map[int]wanikani.Resource[wanikani.Subject], error) {
	return syncResources(filepath.Join(b.dir, "subjects.json"), subjectCacheVersion,
		func(since time.Time) ([]wanikani.Resource[wanikani.Subject], error) {
			return b.api().Subjects(ctx, since)
		}, nil)
}

func (b *backend) syncAssignments(ctx context.Context) (map[int]wanikani.Resource[wanikani.Assignment], error) {
	return syncResources(filepath.Join(b.dir, "assignments.json"), assignmentCacheVersion,
		func(since time.Time) ([]wanikani.Resource[wanikani.Assignment], error) {
			return b.api().Assignments(ctx, since)
		}, nil)
}

func (b *backend) syncSynonyms(ctx context.Context) (map[int][]string, error) {
	path := filepath.Join(b.dir, "study_materials.json")
	var cache synonymCache
	if err := store.ReadJSON(path, &cache); err != nil {
		cache = synonymCache{}
	}
	fresh, err := b.api().StudyMaterials(ctx, cache.SyncedAt)
	if err != nil {
		return nil, err
	}
	if cache.Synonyms == nil {
		cache.Synonyms = map[int][]string{}
	}
	for _, m := range fresh {
		cache.Synonyms[m.Data.SubjectID] = m.Data.MeaningSynonyms
	}
	cache.SyncedAt = newest(cache.SyncedAt, fresh)
	return cache.Synonyms, store.WriteJSON(path, cache)
}

// Submit sends one finished review. The answer is written to pending.json
// first and removed once WaniKani has it (or refused it for good), so killing
// the app mid-send loses nothing: it goes out on the next launch. pending is
// true when it could not be sent now. err is non-nil when WaniKani refused
// the review or the pending file could not be written.
func (b *backend) Submit(ctx context.Context, s review.Submission) (pending bool, err error) {
	if s.CompletedAt.IsZero() {
		s.CompletedAt = time.Now().UTC()
	}
	if err := b.editPending(func(list []review.Submission) []review.Submission { return append(list, s) }); err != nil {
		return false, err
	}
	// The first send lets the server stamp the time: a local clock running
	// ahead would make WaniKani refuse a created_at in its future.
	err = b.api().SubmitReview(ctx, s.AssignmentID, s.IncorrectMeaning, s.IncorrectReading, time.Time{})
	if err != nil && !rejected(err) {
		var apiErr *wanikani.APIError
		if errors.As(err, &apiErr) && apiErr.Status == 403 {
			return true, err // saved, and the UI explains the missing permission
		}
		return true, nil // stays in pending.json
	}
	if rerr := b.editPending(func(list []review.Submission) []review.Submission {
		if i := slices.IndexFunc(list, func(p review.Submission) bool {
			return p.AssignmentID == s.AssignmentID && p.CompletedAt.Equal(s.CompletedAt)
		}); i >= 0 {
			return slices.Delete(list, i, i+1)
		}
		return list
	}); rerr != nil {
		return false, rerr
	}
	return false, err
}

// editPending rewrites pending.json under the lock; submits run concurrently.
func (b *backend) editPending(change func([]review.Submission) []review.Submission) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	var list []review.Submission
	if err := store.ReadJSON(b.pendingPath(), &list); err != nil {
		return err
	}
	return store.WriteJSON(b.pendingPath(), change(list))
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
	for i, s := range list {
		err := b.api().SubmitReview(ctx, s.AssignmentID, s.IncorrectMeaning, s.IncorrectReading, s.CompletedAt)
		if errors.Is(err, wanikani.ErrUnauthorized) {
			// Keep this answer and the untried ones for after a new token;
			// drop the ones already accepted so they are not sent twice.
			if werr := store.WriteJSON(b.pendingPath(), append(keep, list[i:]...)); werr != nil {
				return werr
			}
			return err
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
	art func(wanikani.Resource[wanikani.Subject]) (image.Image, []byte)) (items []review.Item, skipped int) {
	for _, a := range assignments {
		s, ok := subjects[a.Data.SubjectID]
		if !ok {
			continue
		}
		it := review.Item{AssignmentID: a.ID, SRSStage: a.Data.SRSStage, Type: s.Object}
		if s.Data.Characters != nil {
			it.Characters = *s.Data.Characters
		} else if it.Image, it.PNG = art(s); it.Image == nil {
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

// radicalArt returns the pictures for a radical with no Unicode character:
// a small image for half-block art everywhere, and a sharper PNG for kitty.
// Both come from its SVG, downloaded once into the cache and rasterized by
// rsvg-convert (decisions 20 and 30). The image is nil when that is not
// possible, for example when rsvg-convert is not installed.
func (b *backend) radicalArt(ctx context.Context, s wanikani.Resource[wanikani.Subject]) (image.Image, []byte) {
	var url string
	for _, ci := range s.Data.CharacterImages {
		if ci.ContentType == "image/svg+xml" {
			url = ci.URL
		}
	}
	if url == "" {
		return nil, nil
	}
	path := filepath.Join(b.dir, "radicals", fmt.Sprintf("%d.svg", s.ID))
	if _, err := os.Stat(path); err != nil {
		if err := download(ctx, url, path); err != nil {
			return nil, nil
		}
	}
	rasterize := func(px int) []byte {
		size := fmt.Sprint(px)
		out, err := exec.CommandContext(ctx, "rsvg-convert", "-w", size, "-h", size, path).Output()
		if err != nil {
			return nil
		}
		return out
	}
	small := rasterize(radicalArtPx)
	if small == nil {
		return nil, nil
	}
	img, err := png.Decode(bytes.NewReader(small))
	if err != nil {
		return nil, nil
	}
	return img, rasterize(radicalPNGPx)
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

// loadSettings reads settings.json over the defaults, so a hand-edited file
// keeps its values and missing fields get defaults (batch size seeded from
// WaniKani). A corrupt file is kept as settings.json.bad and replaced. If the
// defaults cannot be written (read-only config dir), they are used anyway.
func (b *backend) loadSettings(waniKaniBatch int) (lessons.Settings, error) {
	b.mu.Lock()
	if waniKaniBatch > 0 {
		b.wkBatch = waniKaniBatch
	}
	batch := b.wkBatch
	b.mu.Unlock()
	s := lessons.Default(batch)
	path, err := store.ConfigFile("settings.json")
	if err != nil {
		return s, nil
	}
	raw, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		_ = store.WriteJSON(path, s) // best effort: defaults work without the file
		return s, nil
	case err != nil:
		return s, nil
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		_ = os.WriteFile(path+".bad", raw, 0o600) // keep the user's edits to fix by hand
		s = lessons.Default(batch)
		_ = store.WriteJSON(path, s)
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
	subjects map[int]wanikani.Resource[wanikani.Subject],
	keep func(wanikani.Resource[wanikani.Subject]) bool) []lessons.Candidate {
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
			if !ok || !has || s.Data.HiddenAt != nil || !keep(s) {
				continue
			}
			out = append(out, lessons.Candidate{AssignmentID: aid, SubjectID: id, Level: s.Data.Level, Type: s.Object})
		}
	}
	return out
}

// drawable reports whether a subject can be shown: it has characters, or it
// is an image-only radical whose picture can be rasterized. Undrawable ones
// must not take daily-cap slots they can never use.
func (b *backend) drawable(ctx context.Context) func(wanikani.Resource[wanikani.Subject]) bool {
	return func(s wanikani.Resource[wanikani.Subject]) bool {
		if s.Data.Characters != nil {
			return true
		}
		img, _ := b.radicalArt(ctx, s)
		return img != nil
	}
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
	user, err := b.api().User(ctx)
	if err != nil {
		return none, err
	}
	sum, err := b.api().Summary(ctx)
	if err != nil {
		return none, err
	}
	settings, err := b.loadSettings(user.Preferences.LessonsBatchSize)
	if err != nil {
		return none, err
	}
	now := time.Now()
	picked := lessons.Pick(lessonCandidates(now, sum, assignments, subjects, b.drawable(ctx)), lessons.StartedToday(now, startTimes(assignments)), settings)
	due := make([]wanikani.Resource[wanikani.Assignment], 0, len(picked))
	for _, c := range picked {
		due = append(due, assignments[c.AssignmentID])
	}
	art := func(s wanikani.Resource[wanikani.Subject]) (image.Image, []byte) { return b.radicalArt(ctx, s) }
	items, _ := buildItems(due, subjects, synonyms, art)
	taught := buildLessons(items, assignments, subjects)
	b.remember(taught)
	return lessons.Plan{Lessons: taught, BatchSize: settings.BatchSize}, nil
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
	return b.api().StartAssignment(ctx, assignmentID)
}
