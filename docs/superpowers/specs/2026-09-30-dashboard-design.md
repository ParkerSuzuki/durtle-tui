# durtle-tui: milestone 2 design (dashboard)

durtle-tui opens on a dashboard that answers, at a glance: what can I do now,
what is coming, and how close am I to leveling up. Reviews start from it and
return to it. Why each choice was made lives in [../../decisions.md](../../decisions.md)
(decisions 21 and 22).

## Goals

- Open on a dashboard with four panels: available now, review forecast,
  level progress, SRS breakdown.
- Start reviews from the dashboard; finishing a session returns to a
  refreshed dashboard instead of quitting.
- Refresh cheaply: after the first run, only changed data is fetched.

## Non-goals (milestone 2)

Starting lessons (milestone 3; the dashboard shows the lesson count only),
level-up history, leech lists, accuracy statistics, anything that looks like
the WaniKani website.

## Data

| Panel | Source |
|---|---|
| Available now | `GET /summary`: `data.lessons` and `data.reviews` entries whose `available_at` is now or earlier, summed over `subject_ids` |
| Review forecast | `GET /summary`: `data.reviews` entries for the next 24 hours |
| Level progress | `GET /user` (`level`), cached subjects (new `level` field, `hidden_at`), cached assignments (`passed_at`) |
| SRS breakdown | cached assignments (`srs_stage`, `started_at`) |

**Assignment cache.** `assignments.json` in the cache directory, keyed by
assignment ID, synced with `GET /assignments?updated_after=...` the same way
subjects are, and versioned with its own `assignmentCacheVersion` constant.
First run fetches every assignment (a few thousand, 500 per page).

**Subject cache.** `wanikani.Subject` gains `Level int` and
`HiddenAt *time.Time`. `subjectCacheVersion` goes from 2 to 3, forcing one
full resync (decision 20's mechanism).

**Summary and user** are fetched fresh on every dashboard refresh (one
request each; the summary changes hourly).

## Computation (package `dashboard`, pure)

`dashboard.Build(now time.Time, level int, sum wanikani.Summary,
assignments, subjects) Dashboard` returns:

- `Lessons`, `Reviews int`: available now.
- `Forecast []Hour`: for each of the next 24 whole hours after `now` with at
  least one review, the hour (local time), its count, and the running total
  including what is available now.
- `Progress`: at the user's level, non-hidden radicals and kanji: totals and
  how many are passed (`passed_at` set). `KanjiNeeded` is 90% of the level's
  kanji, rounded up: passing that many levels you up.
- `SRS [5]int`: started assignments by stage group: Apprentice (stages 1-4),
  Guru (5-6), Master (7), Enlightened (8), Burned (9). Hidden subjects are
  excluded.

It imports only the standard library and `wanikani` types, and is covered by
table-driven tests like `review`.

## Screen flow

`loading` -> `dashboard` -> (`r` or Enter, when reviews > 0) -> `loading` ->
`reviewing` -> `summary` -> (Enter) -> `loading` -> `dashboard`.
`q` or Esc quits from the dashboard; Esc during reviews still quits the app
(waiting for in-flight submits, as today). Onboarding still appears whenever
the API answers 401.

`ui.Backend` gains `Dashboard(ctx context.Context) (dashboard.Dashboard, error)`.
It flushes pending answers, syncs subjects and assignments, fetches user and
summary, and calls `dashboard.Build`. `Load` (reviews) is unchanged.

## Layout

Full terminal width, same page padding and palette as reviews:

```
  durtle-tui                                        Level 12

  Lessons 5      Reviews 67              r  start reviews

  Level 12 kanji   ████████████████░░░░░░░░  21 / 33   (30 to level up)
  Level 12 radicals ██████████████████████░░  9 / 10

  Upcoming reviews
    15:00   +12   79   ████
    16:00   +3    82   █
    21:00   +40  122   █████████████

  Apprentice   88   ████████████
  Guru        143   ████████████████████
  Master       97   █████████████
  Enlightened 201   ████████████████████████████
  Burned       12   ██
```

- Progress bars use the kanji and radical colors from decision 16; the
  90% mark is the "to level up" count, not a marker in the bar.
- Forecast lists only hours with reviews, at most 8 rows. Each row shows the
  hour, the added count, the running total, and a bar scaled to the largest
  added count.
- SRS bars share one neutral color (`#A8DADC`), scaled to the largest group.
  Stage colors are left out on purpose: WaniKani's are distinctive, and one
  color keeps the palette small.

## Errors

- Refresh failure: the error with Enter to retry and Esc to quit, the same
  as today's `failed` screen.
- 401 anywhere: onboarding.
- Submits still in flight when returning to the dashboard: the dashboard
  loads anyway; the counts catch up on the next refresh.

## Testing

- `dashboard`: table-driven tests for each panel, including hidden subjects,
  unstarted assignments, the 90% rounding (e.g. 33 kanji needs 30), hours with
  no reviews, and a forecast entry exactly at `now`.
- `wanikani`: `httptest` tests for `Summary` and `Assignments(updatedAfter)`.
- backend: assignment cache incremental sync and version reset.
- `ui`: dashboard renders all four panels at a fixed width; `r` starts
  reviews only when reviews are available; the summary's Enter returns to the
  dashboard.
- Manual: tmux capture of the dashboard against the real account (no
  keystrokes that submit anything).
