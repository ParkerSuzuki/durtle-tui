# Decision log

Newest at the bottom. Each entry: the question (Context), what we chose (Decision),
why (Why), and the alternatives we rejected (Passed on).

## 1. Build a new client instead of forking one (2026-09-29)

**Context:** Several WaniKani terminal clients already exist. Fork one or write our own?
**Decision:** Write durtle from scratch. Use ferjjp/wk-terminal (Python, MIT) as a
behavior reference only.
**Why:** The project exists to learn Go. The best existing client, wk-terminal,
is Python, 2 weeks old, a single squashed commit, and imitates the WaniKani look.
**Passed on:** Forking wk-terminal (fastest path to a working tool); hebikani
(mature but a line-based prompt, not a TUI); the Rust and Go clients (no
license file, so not legally reusable, or GPL, or too small to matter).

## 2. Go as the language (2026-09-29)

**Context:** Which language to build the client in, given the goal is learning it?
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

## 3. Name: durtle-tui (2026-09-29)

**Context:** What to call the app, given WaniKani's branding rules for third-party apps?
**Decision:** The repo, module and binary are `durtle-tui`, after the WaniKani
community's turtle meme. It is described as "an unofficial third-party client
for WaniKani".
**Why:** The API terms forbid "WaniKani" in a product name and require the
unofficial label. The `-tui` suffix says what it is at a glance.
**Passed on:** `durtle-cli` (it is a full-screen TUI, not a line-based CLI);
repo `durtle-tui` with a shorter `durtle` command.

## 4. Milestone 1 is reviews only (2026-09-29)

**Context:** What is the smallest first milestone that is still useful every day?
**Decision:** Sync, review, grade, submit. Nothing else.
**Why:** It is the daily-use feature, and it exercises every Go basic: HTTP,
JSON, structs, errors, tests, and the TUI loop.
**Passed on:** Read-only status command first (safer but less useful);
lessons plus reviews (larger first step, needs a teaching screen).

## 5. Local data: JSON files, standard library only (2026-09-29)

**Context:** Where does WaniKani data live between runs?
**Decision:** Subjects and study materials are cached as JSON files in
`os.UserCacheDir()/durtle`, updated incrementally with `updated_after`.
Assignments are always fetched fresh.
**Why:** Subjects change rarely and are about 10 requests to fetch in full.
JSON files need zero dependencies and teach `encoding/json` and `os`.
**Passed on:** SQLite (driver plus schema for data we do not query yet);
no cache (about 10 requests on every launch).

## 6. Image-only radicals are skipped in milestone 1 (2026-09-29)

**Context:** Some radicals have no Unicode character, only an SVG image. How are they reviewed?
**Decision:** Radicals with no Unicode character stay due and are left for the
website. Tracked in TODO.md.
**Why:** Rendering them needs SVG rasterizing plus the kitty graphics protocol,
which is a milestone of its own.
**Passed on:** Opening the SVG in a browser (clunky); pulling images into milestone 1.

## 7. Own romaji to kana converter (2026-09-29)

**Context:** Reading answers are typed in romaji and must become kana. Which converter?
**Decision:** Write a small converter in package `review`, tested against a
table of cases.
**Why:** The only Go port of WanaKana we found is unvetted. The converter is
roughly 150 lines and a good exercise in runes, maps, and table-driven tests.

## 8. Token entered in the app, stored in the OS keyring (2026-09-29, revised)

**Context:** Where does the API token come from, and where is it stored?
**Decision:** First run shows an onboarding screen where you paste your token.
It is validated with `GET /user`, then saved with go-keyring (Linux Secret
Service, macOS Keychain, Windows Credential Manager). If no keyring is
available, it goes to `os.UserConfigDir()/durtle-tui/token` with mode 0600.
This is the same pattern GitHub's `gh` CLI uses.
**Why:** The repo is public, so onboarding must be easy and work on any OS.
A keyring keeps the token out of plain-text files where one exists.
**Passed on:** `secret-tool` plus an environment variable (the original plan,
Linux only and fiddly for new users); config file only (plain text on disk);
keyring only (breaks on headless machines).

## 9. Never lose a finished answer (2026-09-29)

**Context:** What happens to a finished review if submitting it fails?
**Decision:** A finished item that fails to submit goes into `pending.json`,
which is retried on the next launch.
**Why:** A dropped connection should not silently throw away a review.
This is not full offline mode; syncing still needs the network.

## 10. Working agreement (2026-09-29)

**Context:** Who writes the code, and how are decisions made?
**Decision:** Claude writes the code and explains the Go idioms it uses. Every
design decision is raised with the user first and then recorded here.
**Why:** The goal is understanding Go's choices, not typing speed.

## 11. Back-to-back review order (2026-09-29)

**Context:** In what order are items and their meaning/reading questions asked?
**Decision:** Ask one item at a time. Meaning or reading first is a coin flip
per item. A wrong answer re-asks the same part until it is correct, then the
other part comes up. The item is submitted once both are correct.
**Why:** It is how the user already reviews on the website, via the "Back to
back" (https://greasyfork.org/en/scripts/439837) and "Reorder Omega"
(https://greasyfork.org/en/scripts/441619) userscripts. It also makes the
session logic simpler: no requeueing, just a list and a position.
**Passed on:** WaniKani's default (parts interleaved across the whole queue,
wrong items requeued later).

## 12. Public repo, MIT license (2026-09-29)

**Context:** Where is the code hosted, and under what license?
**Decision:** Host at github.com/ParkerSuzuki/durtle-tui, public, MIT licensed.
The README opens by stating it is an unofficial third-party app.
**Why:** MIT is short and permissive and matches wk-terminal, our behavior
reference. The API terms require the unofficial label up front.
**Passed on:** GPL-3.0 (forces forks to stay open); Apache-2.0 (adds a patent
grant, longer).

## 13. Charm v2 libraries (2026-09-29)

**Context:** Bubble Tea, Bubbles and Lip Gloss each have a v1 and a v2 line.
**Decision:** Use v2, imported from `charm.land/bubbletea/v2`, `charm.land/bubbles/v2`, `charm.land/lipgloss/v2`.
**Why:** v2 is where maintenance happens (v2.0.10 shipped 2026-09-24); v1 has had no release since 2025.
**Passed on:** v1 (more tutorials online, but frozen).

## 14. store package and a Backend interface (2026-09-29)

**Context:** The UI must log in, sync, and submit, but should be testable without a network or a keyring.
**Decision:** Add package `store` (token plus JSON files). `ui` declares a three-method `Backend` interface; `package main` implements it by combining `wanikani` and `store`. Tests pass a fake.
**Why:** "Accept interfaces, return structs": the consumer defines the small interface it needs. Keeps `ui` free of HTTP and disk code.
**Passed on:** `ui` importing `wanikani` and `store` directly (untestable without a server); a struct of function fields (works, less idiomatic).

## 15. Answer flow (2026-09-29)

**Context:** What happens on screen after each answer?
**Decision:** Correct answers advance immediately with a short confirmation line. Wrong answers show the accepted answers and wait for Enter. Warnings (kana in a meaning, wrong reading type) keep the input so you can fix it.
**Why:** Matches the website's rhythm and gives time to read the correction.
**Passed on:** Waiting for Enter after every answer (slower); auto-advancing after wrong answers (no time to read).

## 16. Color palette (2026-09-29)

**Context:** The API terms forbid copying WaniKani's visual design, including its pink/blue/purple.
**Decision:** Radical teal `#2A9D8F`, kanji amber `#E9A23B`, vocabulary green `#6A994E`. Meaning prompt: light bar `#F4F1DE` with `#1D1D1D` text. Reading prompt: dark bar `#3D405B` with `#F4F1DE` text.
**Why:** Distinct from WaniKani, readable on dark and light terminals, meaning vs reading is obvious at a glance.
**Passed on:** Nothing formal; the user accepted it "for now", so revisit once it is on screen.

## 17. Review screen spans the terminal (2026-09-30)

**Context:** The review screen was a fixed 55 cells wide; the user wanted it to fill the terminal and follow resizes.
**Decision:** The character block, prompt bar, and input stretch to the terminal width minus a 2-column margin, recomputed on every `tea.WindowSizeMsg`. Characters and the prompt label are centered; typed answers stay left-aligned.
**Why:** Matches how the review page uses the whole screen. Bubble Tea already sends a message on every resize, so it costs one width field and a few style calls.
**Passed on:** A max width with centered layout (reads better on very wide terminals; revisit if it looks stretched).

## 18. Taller character block, centered answer (2026-09-30)

**Context:** The user wanted bigger kanji and the typed answer centered.
**Decision:** The character block gets 3 blank rows above and below (7 rows total). The answer input drops its `> ` prompt, is sized to its text, and is centered, so it grows from the middle as you type.
**Why:** Terminals draw every character at one font size, so "bigger" in plain terminal cells means more colored space. Sizing the input to its text is the simplest way to center text inside a Bubbles textinput, which only aligns left.
**Passed on:** Real 2x/3x glyphs via kitty's text sizing protocol for now; a throwaway spike is checking whether Bubble Tea's renderer tolerates it.

## 19. Big characters in kitty via the text sizing protocol (2026-09-30)

**Context:** The user wanted the kanji themselves bigger, not just the block around them. Terminals draw one font size; kitty 0.40+ can draw text at 2x or 3x with OSC 66.
**Decision:** When kitty itself draws the app (`TERM=xterm-kitty`), the view leaves the character row empty and a command 40 ms after each update writes the characters at 3x (2x if too wide, normal size if neither fits) straight to the terminal with `tea.Raw`. Other terminals keep the normal layout.
**Why:** A throwaway spike showed Bubble Tea v2's renderer strips OSC 66 from the view, but raw output after the frame works, including across keypresses and resizes. The worst failure is cosmetic: the glyph vanishes until the next message.
**Known limit:** It is a timing hack, marked with a `ponytail:` comment in `ui/model.go`. A Bubble Tea upgrade that changes when frames are painted could break it; check the review screen after upgrading.
**Passed on:** A taller block only (decision 18 stays as the fallback); putting OSC 66 in the view (stripped by the renderer).

**Revised 2026-09-30:** Detection first used `KITTY_WINDOW_ID`, which herdr (and tmux) inherit from the outer kitty while dropping OSC 66, leaving an empty block. It now checks `TERM=xterm-kitty`, which any multiplexer in between replaces.
