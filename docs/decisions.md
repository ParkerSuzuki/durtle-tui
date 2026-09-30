# Decision log

Newest at the bottom. Each entry: what we decided, why, and what we passed on.

## 1. Build a new client instead of forking one (2026-09-29)

**Decision:** Write durtle from scratch. Use ferjjp/wk-terminal (Python, MIT) as a
behavior reference only.
**Why:** The project exists to learn Go. The best existing client, wk-terminal,
is Python, 2 weeks old, a single squashed commit, and imitates the WaniKani look.
**Passed on:** Forking wk-terminal (fastest path to a working tool); hebikani
(mature but a line-based prompt, not a TUI); the Rust and Go clients (no
license file, so not legally reusable, or GPL, or too small to matter).

## 2. Go as the language (2026-09-29)

**Decision:** Go.
**Why:** The app is network-bound with small data, so raw speed is irrelevant.
What matters: TUI library quality (Bubble Tea is among the best in any language),
correct width for Japanese characters (Lip Gloss handles double-width runes),
a strong standard library for HTTP, JSON and tests, and a single static binary.
**Costs we accept:** No vetted WanaKana port, so we write our own romaji
converter. SVG radicals later need a library or `rsvg-convert`.
**Passed on:** Python + Textual (fastest, but just re-does wk-terminal);
Rust + Ratatui (best WanaKana port, but much steeper to learn for no payoff here);
TypeScript + Ink (thinner ecosystem for full-screen apps).

## 3. Name: durtle (2026-09-29)

**Decision:** The app and binary are `durtle`, after the WaniKani community's
turtle meme. It is described as "an unofficial third-party client for WaniKani".
**Why:** The API terms forbid "WaniKani" in a product name and require the
unofficial label.

## 4. Milestone 1 is reviews only (2026-09-29)

**Decision:** Sync, review, grade, submit. Nothing else.
**Why:** It is the daily-use feature, and it exercises every Go basic: HTTP,
JSON, structs, errors, tests, and the TUI loop.
**Passed on:** Read-only status command first (safer but less useful);
lessons plus reviews (larger first step, needs a teaching screen).

## 5. Local data: JSON files, standard library only (2026-09-29)

**Decision:** Subjects and study materials are cached as JSON files in
`os.UserCacheDir()/durtle`, updated incrementally with `updated_after`.
Assignments are always fetched fresh.
**Why:** Subjects change rarely and are about 10 requests to fetch in full.
JSON files need zero dependencies and teach `encoding/json` and `os`.
**Passed on:** SQLite (driver plus schema for data we do not query yet);
no cache (about 10 requests on every launch).

## 6. Image-only radicals are skipped in milestone 1 (2026-09-29)

**Decision:** Radicals with no Unicode character stay due and are left for the
website. Tracked in TODO.md.
**Why:** Rendering them needs SVG rasterizing plus the kitty graphics protocol,
which is a milestone of its own.
**Passed on:** Opening the SVG in a browser (clunky); pulling images into milestone 1.

## 7. Own romaji to kana converter (2026-09-29)

**Decision:** Write a small converter in package `review`, tested against a
table of cases.
**Why:** The only Go port of WanaKana we found is unvetted. The converter is
roughly 150 lines and a good exercise in runes, maps, and table-driven tests.

## 8. API token lives in the system keyring (2026-09-29)

**Decision:** Read the token with `secret-tool lookup service durtle`, falling
back to the `DURTLE_TOKEN` environment variable.
**Why:** Tokens never go in files that could be committed or synced. Matches how
the rest of this machine stores credentials.

## 9. Never lose a finished answer (2026-09-29)

**Decision:** A finished item that fails to submit goes into `pending.json`,
which is retried on the next launch.
**Why:** A dropped connection should not silently throw away a review.
This is not full offline mode; syncing still needs the network.

## 10. Working agreement (2026-09-29)

**Decision:** Claude writes the code and explains the Go idioms it uses. Every
design decision is raised with the user first and then recorded here.
**Why:** The goal is understanding Go's choices, not typing speed.
