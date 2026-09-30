# durtle: milestone 1 design (reviews)

durtle is an unofficial third-party terminal client for WaniKani, written in Go.
Milestone 1 does one thing: run your due reviews from the terminal and submit
them to WaniKani, graded the way the website grades them.

Why each choice was made lives in [../../decisions.md](../../decisions.md).
Deferred work lives in [../../../TODO.md](../../../TODO.md).

## Goals

- Sync your subjects and due assignments from the WaniKani API v2.
- Run a review session in the terminal: meaning and reading for each item.
- Grade answers client side (the API does not grade) and submit each finished item.
- Never lose a finished answer, even if the network drops or you quit mid-session.
- Serve as a readable Go codebase: small packages, standard library first, idiomatic tests.

## Non-goals (milestone 1)

Lessons, dashboard, audio, radical images, undo, wrap-up mode, offline reviews,
visual similarity to the WaniKani website (forbidden by the API terms anyway).

## Layout

```
~/WaniKani/
  go.mod               module github.com/ParkerSuzuki/durtle
  main.go              wiring: token, client, cache, start the UI
  wanikani/            API client: types, requests, paging, rate limits
  review/              pure logic: grading, typo tolerance, kana, session queue
  ui/                  Bubble Tea models: loading, review, summary screens
  docs/decisions.md    decision log
  TODO.md              deferred work
```

Dependency direction: `main` -> `ui` -> `review`, and `main` -> `wanikani`.
`review` imports nothing outside the standard library and never touches the
network or the screen, so it is fully unit testable.

External dependencies: Bubble Tea, Bubbles (text input), Lip Gloss (styling).
Everything else is standard library.

## Data flow

1. **Token.** Run `secret-tool lookup service durtle`. If that fails, read the
   `DURTLE_TOKEN` environment variable. Neither present: print how to store one
   and exit. Required token permissions: `reviews:create` (plus the default read access).
2. **Subjects.** Load `$XDG_CACHE_HOME/durtle/subjects.json` (via
   `os.UserCacheDir`). Fetch `GET /subjects?updated_after=<last sync>`, following
   `pages.next_url` until null, merge by subject id, write the file back.
   First run fetches everything (about 10 pages).
3. **Study materials.** Same pattern, cached in `study_materials.json`. Source
   of your personal meaning synonyms.
4. **Pending submissions.** If `pending.json` exists, submit its entries first
   and remove each one that succeeds.
5. **Assignments.** `GET /assignments?immediately_available_for_review=true`,
   always fresh, never cached.
6. **Queue.** Join assignments to subjects, drop image-only radicals
   (`characters` is null), shuffle.
7. **Session.** See below.
8. **Submit.** When an item is finished, `POST /reviews` with
   `assignment_id`, `incorrect_meaning_answers`, `incorrect_reading_answers`.

## Review session rules

- Radicals and kana-only vocabulary ask for meaning only. Kanji and vocabulary
  ask for meaning and reading, in random order, not necessarily back to back.
- A wrong answer shows the accepted answers, increments that part's incorrect
  count, and puts the item back into the queue at a random later position.
- An item is finished when both of its parts have been answered correctly.
  It is submitted immediately in the background.
- Quitting mid-session is safe: finished items are already submitted or in
  `pending.json`; unfinished items are simply not submitted and stay due.

## Grading (package `review`)

**Meaning**
- Normalize: lowercase, trim, collapse inner whitespace.
- Accepted set: `meanings` with `accepted_answer=true`, `auxiliary_meanings`
  of type `whitelist`, and your study-material synonyms.
- Exact match with a `blacklist` auxiliary meaning is wrong, before typo tolerance runs.
- Typo tolerance is Optimal String Alignment distance, with the allowed number
  of edits depending on the answer's length L:
  - L <= 3: 0 edits
  - L 4 to 5: 1 edit
  - L 6 to 7: 2 edits
  - L >= 8: 2 + floor(L / 7) edits

  Answers that contain digits must match exactly. These rules are community
  reverse-engineered: https://community.wanikani.com/t/how-to-check-if-answer-is-correct-or-not/47194
- Input containing kana gets a warning ("we want the meaning") and is not counted as wrong.

**Reading**
- Input is converted from romaji to hiragana as you type (our own converter,
  see decisions). Katakana input is normalized to hiragana before comparing.
- A trailing `n` becomes `ん` on submit.
- Correct: exact match with a reading where `accepted_answer=true`.
- Kanji only: a match with a reading where `accepted_answer=false` (for
  example on'yomi when kun'yomi is wanted) gets a warning, not a wrong answer.

The result of grading is one of: `Correct`, `CorrectWithTypo`, `Wrong`, `Warn`.

## API client (package `wanikani`)

- Every request sends `Authorization: Bearer <token>` and `Wanikani-Revision: 20170710`.
- One `http.Client` with a timeout. `context.Context` on every call so the UI can cancel.
- On HTTP 429, wait until the `RateLimit-Reset` time, then retry. Stay under
  the 60 requests per minute limit.
- Typed structs for only the fields we use. Unknown JSON fields are ignored.

## Screens (package `ui`)

- **Loading:** sync progress (subjects page n, assignments), then the due count.
- **Review:** item characters, a clear prompt of which part is being asked
  (meaning or reading, visually distinct), the input line, a feedback line,
  and a progress line (done, remaining, percent correct).
  Keys: Enter submits or continues after feedback, Ctrl+C or Esc quits.
- **Summary:** correct and incorrect items, and whether any are still pending.

Styling uses our own color theme, not WaniKani's.

## Errors

- Network or API error while syncing: show it on the loading screen with a
  retry key. Stale subject cache is fine to use if assignments loaded.
- Submit failure: append to `pending.json`, keep going, say so on the summary.
- Corrupt cache file: discard it and do a full sync.

## Testing

- `review`: table-driven tests for grading, typo thresholds, kana conversion,
  and queue behavior (wrong answers requeue, item finishes after both parts).
- `wanikani`: `httptest.Server` tests for paging, headers, 429 retry, submit body.
- `ui`: unit tests on the model's `Update` for the main state transitions.
  Manual end-to-end run against the real account before calling milestone 1 done.
