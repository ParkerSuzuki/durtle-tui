# durtle-tui: milestone 3 design (lessons)

durtle-tui teaches new items: pick today's lessons by the user's own rules,
show teaching screens, quiz the batch, and start each passed item on
WaniKani so it enters the review queue. Why each choice was made lives in
[../../decisions.md](../../decisions.md) (decisions 25 to 28).

## Background (verified 2026-09-30)

- The WaniKani website picks "Today's Lessons" with an unpublished algorithm
  and caps them with **Maximum Recommended Daily Lessons**, reset at local
  midnight. Neither is in the API.
- `lessons_presentation_order` is deprecated ("Setting this preference will
  do nothing"). `lessons_batch_size` is readable (this user: 3).
- `GET /summary` lists subject IDs available for lessons now.
- `PUT /assignments/<id>/start` starts a lesson (SRS stage 0 to 1). It needs
  the token permission `assignments:start`. The API does not require a quiz.

## Goals

- Editable lesson rules: daily cap, order, type filter, batch size.
- Teaching screens for radicals, kanji, and vocabulary.
- A quiz after each batch; each passed item is started on WaniKani.
- Lessons done on the website count toward durtle-tui's daily cap.

## Non-goals (milestone 3)

Lesson audio, syncing settings to WaniKani, a free-form lesson picker,
lessons for image-only radicals without `rsvg-convert` (skipped, as in reviews).

## Settings

Stored in `os.UserConfigDir()/durtle-tui/settings.json` (0600), edited on a
settings screen opened with `s` from the dashboard.

| Setting | Values | Default |
|---|---|---|
| `daily_cap` | 0 to 100 (0 means no lessons) | 10 |
| `order` | `classic` or `interleaved` | `classic` |
| `types` | any non-empty subset of radical, kanji, vocabulary | all three |
| `batch_size` | 3 to 10 | the user's WaniKani `lessons_batch_size` on first run, else 5 |

`vocabulary` includes `kana_vocabulary`. A missing or corrupt file means
defaults; unknown fields are ignored; out-of-range values are clamped.

## Selection (package `lessons`, pure)

`lessons.Pick(available []Candidate, startedToday int, s Settings) []Candidate`:

1. Drop candidates whose type is turned off.
2. Order:
   - `classic`: level ascending, then radical, kanji, vocabulary, then
     subject ID (WaniKani's per-level order).
   - `interleaved`: take the classic order and deal it round-robin by type
     (radical, kanji, vocabulary, repeat), skipping types that ran out.
3. Keep the first `max(daily_cap - startedToday, 0)`.

`startedToday` counts cached assignments whose `started_at` is at or after
local midnight today, from any client.

Candidates come from `GET /summary` lesson subject IDs available now,
joined with cached subjects and assignments (assignment ID, level, type),
excluding hidden subjects.

## Screens and flow

`home` (`l`, when today's lesson count > 0) -> `loading` -> `teaching` ->
`quiz` -> (next batch: `teaching` ... ) -> `lessonSummary` -> (Enter) ->
`home`. `s` on `home` -> `settings` -> (Esc saves) -> `home`.

- **Dashboard:** the Lessons tile shows today's count
  (`min(cap left, available after filters)`), with "of N available" under it.
  Hint line gains `l start lessons   s settings`.
- **Teaching:** one item at a time, with ← and → moving between items in the
  batch (and between the item's pages: meaning, then reading where there
  is one, then context for vocabulary). Enter on the last page of the last
  item starts the quiz. `:q` returns to the dashboard; nothing is started.
  - Radical: characters or half-block image, meaning, meaning mnemonic.
  - Kanji: meaning, meaning mnemonic and hint; on'yomi and kun'yomi with
    the ones WaniKani accepts marked; reading mnemonic and hint; component
    radicals (characters and meanings).
  - Vocabulary: meaning, parts of speech, reading, mnemonics, component
    kanji, up to 3 context sentences (Japanese and English).
  - Mnemonic markup: `<radical>`, `<kanji>`, `<vocabulary>` in their type
    colors; `<meaning>` and `<reading>` bold; `<ja>` plain; any other tag
    stripped, text kept.
- **Quiz:** the batch runs through the existing `review.Session`
  (back-to-back, same grading and warnings). When an item finishes, it is
  started with `PUT /assignments/<id>/start` in the background; quiz
  answers are never sent to `POST /reviews`. `:q` leaves; items already
  passed stay started, the rest remain lessons.
- **Lesson summary:** items started, and any that failed to start.
- **Settings:** ↑/↓ choose a setting, ←/→ change it, space toggles a type.
  Esc saves and returns.

## Data and API

- `wanikani.Subject` gains `MeaningMnemonic`, `MeaningHint`,
  `ReadingMnemonic`, `ReadingHint`, `ContextSentences []{En, Ja}`,
  `PartsOfSpeech`, `ComponentSubjectIDs`. `subjectCacheVersion` goes to 4
  (one full resync).
- `wanikani.User` gains `Preferences.LessonsBatchSize`.
- `Client.StartAssignment(ctx, id)`: `PUT assignments/<id>/start` with
  `{"assignment":{}}`.
- `ui.Backend` gains `Lessons(ctx) ([]lessons.Lesson, error)` (picked,
  ordered, with teaching content) and `StartLesson(ctx, assignmentID) error`.
  The dashboard's lesson count comes from the same pick.
- Settings load and save live in `store` (`LoadSettings`, `SaveSettings`).

## Errors

- A 403 on start (token lacks `assignments:start`): the lesson summary says
  so and explains how to make a token with that permission; the items stay
  lessons.
- Any other start failure: listed on the lesson summary; the item stays a
  lesson (nothing to lose: it is taught again next time).
- 401: onboarding, as elsewhere.

## Testing

- `lessons`: table tests for type filter, both orders (including a type
  running out mid-deal), the cap with lessons already started today, cap 0,
  and a cap larger than what is available.
- Mnemonic markup parser: tags, nested text, unknown tags, `<ja>`.
- `store`: settings round trip, defaults on missing and corrupt files,
  clamping.
- `wanikani`: `StartAssignment` method, path, and body; new subject fields
  decode.
- backend: `Lessons` joins summary, subjects, assignments, and settings;
  `startedToday` counts across the local midnight boundary.
- `ui`: `l` only with lessons today; paging through teaching screens; quiz
  finishing an item calls `StartLesson`, never `Submit`; 403 message; `:q`
  from teaching starts nothing; settings screen edits and saves.
- Manual: tmux capture of the dashboard and the settings screen only.
  Starting a real lesson is the user's own first run.
