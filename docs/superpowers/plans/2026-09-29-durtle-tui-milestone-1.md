# durtle-tui Milestone 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A Go terminal app that onboards a WaniKani API token, syncs due reviews, runs a back-to-back review session graded like the website, and submits every finished item without ever losing one.

**Architecture:** Four packages with one job each. `review` is pure logic (kana, grading, session) with no I/O. `wanikani` is the HTTP client. `store` holds the token and JSON files on disk. `ui` is the Bubble Tea app and talks to the outside world only through a small `Backend` interface, which `package main` implements by gluing `wanikani` and `store` together.

**Tech Stack:** Go 1.27, `charm.land/bubbletea/v2`, `charm.land/bubbles/v2` (textinput), `charm.land/lipgloss/v2`, `github.com/zalando/go-keyring`. Everything else is the standard library.

**Spec:** `docs/superpowers/specs/2026-09-29-durtle-design.md`. Why-questions are answered in `docs/decisions.md`.

## Global Constraints

- Module path: `github.com/ParkerSuzuki/durtle-tui`. Binary name: `durtle-tui`.
- Every request sends `Authorization: Bearer <token>` and `Wanikani-Revision: 20170710`. Base URL `https://api.wanikani.com/v2/`.
- Only the four external modules above. Nothing else without asking the user first.
- Keyring service name `durtle-tui`, user `api-token`. Fallback file `os.UserConfigDir()/durtle-tui/token`, mode 0600. Cache dir `os.UserCacheDir()/durtle-tui`.
- The app describes itself as "an unofficial third-party app for WaniKani". Colors are our own palette, never WaniKani's pink/blue/purple.
- No em dashes anywhere (code, comments, docs, commit messages).
- Every task ends with `gofmt -w .` (Go has one canonical format; let the tool apply it) and `go vet ./...` passing.
- Working agreement (decision 10): Claude writes the code; for every task, walk the user through the **Go concepts** listed in that task before moving on, and raise any new design choice before making it.

## Review Focus

1. Typing a reading live, one key at a time (`o`, `n`, `n`, `a`) must end as `おんな`, not `おんあ`. Pinned in Task 2 (`TestLiveTyping`).
2. Double consonants and `tch` (`kitte`, `zasshi`, `matcha`) must produce `っ`. Pinned in Task 2 (`TestToHiragana`).
3. Meanings with stray capitals and spaces (`"  Grown   UP "`) must count as correct. Pinned in Task 3 (`TestGradeMeaning`).
4. Quitting while a submit is still in flight must wait for it instead of dropping it. Pinned in Task 8 (`TestQuitWaitsForSubmits`).
5. A submit that fails for a retryable reason (network, 5xx, 401, rate limit after retries) must land in `pending.json`; one WaniKani refuses outright (422) must not be retried forever. Pinned in Task 7 (`TestSubmitPendingAndRejected`, `TestFlushPending`).

## Decisions made while planning (user sign-off requested in the handoff)

- D13: Charm v2 libraries (`charm.land/...`), not v1.
- D14: A fourth package `store`, and `ui` depends on a `Backend` interface implemented in `package main`.
- D15: Review UX: a correct answer advances immediately; a wrong answer shows the accepted answers and waits for Enter.
- D16: Palette: radical `#2A9D8F` (teal), kanji `#E9A23B` (amber), vocabulary `#6A994E` (green); meaning prompt bar light (`#F4F1DE` on `#1D1D1D` text), reading prompt bar dark (`#3D405B` with `#F4F1DE` text).

## File Map

```
go.mod, go.sum
main.go                 entry point: build backend, run the Bubble Tea program
backend.go              implements ui.Backend: sync, cache, pending queue, buildItems
backend_test.go
review/kana.go          romaji to hiragana, katakana to hiragana
review/kana_test.go
review/item.go          Item, Part
review/grade.go         Verdict, Grade, GradeMeaning, GradeReading, OSA distance
review/grade_test.go
review/session.go       Session, Submission, Result (back-to-back order)
review/session_test.go
wanikani/types.go       JSON types
wanikani/client.go      Client, do(), rate limit retry, APIError, ErrUnauthorized
wanikani/client_test.go
wanikani/endpoints.go   User, Subjects, StudyMaterials, ReviewAssignments, SubmitReview
wanikani/endpoints_test.go
store/store.go          token load/save, ReadJSON/WriteJSON, CacheDir
store/store_test.go
ui/model.go             Backend interface, Model, Update
ui/view.go              styles and View
ui/model_test.go
```

---

### Task 1: Toolchain, module, and decision log

**Files:**
- Create: `go.mod`, `main.go`
- Modify: `docs/decisions.md`, `docs/superpowers/specs/2026-09-29-durtle-design.md`

**Interfaces:**
- Consumes: nothing.
- Produces: a module `github.com/ParkerSuzuki/durtle-tui` that builds.

**Go concepts to explain:** what a module is vs a package; why the module path looks like a URL; `package main` and `func main`; `go build` / `go run` / `go vet` / `gofmt` and why Go has exactly one formatting style.

- [ ] **Step 1: Install Go (graphical password prompt)**

Run: `pkexec pacman -S --needed --noconfirm go`
Then verify unprivileged: `go version`
Expected: `go version go1.27.x linux/amd64`

- [ ] **Step 2: Create the module**

Run: `cd ~/WaniKani && go mod init github.com/ParkerSuzuki/durtle-tui`
Expected: `go.mod` containing `module github.com/ParkerSuzuki/durtle-tui` and a `go 1.27` line.

- [ ] **Step 3: Minimal main.go**

```go
// durtle-tui is an unofficial third-party terminal client for WaniKani.
package main

import "fmt"

func main() {
	fmt.Println("durtle-tui: an unofficial third-party app for WaniKani")
}
```

- [ ] **Step 4: Build and run**

Run: `gofmt -w . && go run . && go vet ./...`
Expected: prints the line above; vet prints nothing.

- [ ] **Step 5: Record D13 to D16 in `docs/decisions.md`** (append, same format as existing entries)

```markdown
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
```

- [ ] **Step 6: Update the spec layout** to list `store/` (token and JSON files) and `backend.go` (implements `ui.Backend`) under Layout, and change the dependency line to: `main` -> `ui`, `wanikani`, `store`; `ui` -> `review` (and `wanikani` only for `ErrUnauthorized`).

- [ ] **Step 7: Commit**

```bash
git add go.mod main.go docs
git commit -m "Initialize Go module; record decisions 13-16"
```

---

### Task 2: Romaji to kana converter

**Files:**
- Create: `review/kana.go`, `review/kana_test.go`

**Interfaces:**
- Produces: `func ToHiragana(s string, final bool) string`, `func KatakanaToHiragana(s string) string`, and unexported `containsKana(s string) bool`, `containsLatin(s string) bool` (used by Task 3).

**Go concepts to explain:** strings are byte slices holding UTF-8; bytes vs runes and why `for i := range s` over bytes needs `utf8.DecodeRuneInString` for kana; `strings.Builder`; map literals; `strings.Map`; table-driven tests with `t.Run` subtests; why the test file lives next to the code and is package `review` (internal test).

- [ ] **Step 1: Write the failing tests** (`review/kana_test.go`)

```go
package review

import "testing"

func TestToHiragana(t *testing.T) {
	tests := []struct {
		in    string
		final bool
		want  string
	}{
		{"kanji", true, "かんじ"},
		{"onna", true, "おんな"},
		{"konnichiha", true, "こんにちは"},
		{"kitte", true, "きって"},
		{"zasshi", true, "ざっし"},
		{"matcha", true, "まっちゃ"},
		{"gakkou", true, "がっこう"},
		{"shinnyuu", true, "しんにゅう"},
		{"kyou", true, "きょう"},
		{"KANA", true, "かな"},
		{"ra-men", true, "らーめん"},
		{"n'a", true, "んあ"},
		{"hon", true, "ほん"},
		{"hon", false, "ほn"},
		{"honn", false, "ほnn"},
		{"ky", false, "ky"},
		{"かn", true, "かん"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := ToHiragana(tt.in, tt.final); got != tt.want {
				t.Errorf("ToHiragana(%q, %v) = %q, want %q", tt.in, tt.final, got, tt.want)
			}
		})
	}
}

// TestLiveTyping feeds one key at a time, the way the text input does.
func TestLiveTyping(t *testing.T) {
	for _, tt := range []struct{ keys, want string }{
		{"onna", "おんな"},
		{"kitte", "きって"},
		{"shinnyuu", "しんにゅう"},
		{"hon", "ほん"},
	} {
		v := ""
		for _, k := range tt.keys {
			v = ToHiragana(v+string(k), false)
		}
		if got := ToHiragana(v, true); got != tt.want {
			t.Errorf("typing %q gave %q, want %q", tt.keys, got, tt.want)
		}
	}
}

func TestKatakanaToHiragana(t *testing.T) {
	if got := KatakanaToHiragana("ラーメン"); got != "らーめん" {
		t.Errorf("got %q", got)
	}
}
```

- [ ] **Step 2: Run to see it fail**

Run: `go test ./review/`
Expected: FAIL, `undefined: ToHiragana`.

- [ ] **Step 3: Implement** (`review/kana.go`)

```go
package review

import (
	"strings"
	"unicode/utf8"
)

// romaji maps syllables to hiragana. Keys are at most 3 bytes; ToHiragana
// tries the longest match first.
var romaji = map[string]string{
	"a": "あ", "i": "い", "u": "う", "e": "え", "o": "お",
	"ka": "か", "ki": "き", "ku": "く", "ke": "け", "ko": "こ",
	"ga": "が", "gi": "ぎ", "gu": "ぐ", "ge": "げ", "go": "ご",
	"sa": "さ", "si": "し", "shi": "し", "su": "す", "se": "せ", "so": "そ",
	"za": "ざ", "zi": "じ", "ji": "じ", "zu": "ず", "ze": "ぜ", "zo": "ぞ",
	"ta": "た", "ti": "ち", "chi": "ち", "tu": "つ", "tsu": "つ", "te": "て", "to": "と",
	"da": "だ", "di": "ぢ", "du": "づ", "de": "で", "do": "ど",
	"na": "な", "ni": "に", "nu": "ぬ", "ne": "ね", "no": "の",
	"ha": "は", "hi": "ひ", "hu": "ふ", "fu": "ふ", "he": "へ", "ho": "ほ",
	"ba": "ば", "bi": "び", "bu": "ぶ", "be": "べ", "bo": "ぼ",
	"pa": "ぱ", "pi": "ぴ", "pu": "ぷ", "pe": "ぺ", "po": "ぽ",
	"ma": "ま", "mi": "み", "mu": "む", "me": "め", "mo": "も",
	"ya": "や", "yu": "ゆ", "yo": "よ",
	"ra": "ら", "ri": "り", "ru": "る", "re": "れ", "ro": "ろ",
	"wa": "わ", "wo": "を",
	"kya": "きゃ", "kyu": "きゅ", "kyo": "きょ",
	"gya": "ぎゃ", "gyu": "ぎゅ", "gyo": "ぎょ",
	"sha": "しゃ", "shu": "しゅ", "sho": "しょ", "she": "しぇ",
	"sya": "しゃ", "syu": "しゅ", "syo": "しょ",
	"ja": "じゃ", "ju": "じゅ", "jo": "じょ", "je": "じぇ",
	"jya": "じゃ", "jyu": "じゅ", "jyo": "じょ",
	"zya": "じゃ", "zyu": "じゅ", "zyo": "じょ",
	"cha": "ちゃ", "chu": "ちゅ", "cho": "ちょ", "che": "ちぇ",
	"tya": "ちゃ", "tyu": "ちゅ", "tyo": "ちょ",
	"nya": "にゃ", "nyu": "にゅ", "nyo": "にょ",
	"hya": "ひゃ", "hyu": "ひゅ", "hyo": "ひょ",
	"bya": "びゃ", "byu": "びゅ", "byo": "びょ",
	"pya": "ぴゃ", "pyu": "ぴゅ", "pyo": "ぴょ",
	"mya": "みゃ", "myu": "みゅ", "myo": "みょ",
	"rya": "りゃ", "ryu": "りゅ", "ryo": "りょ",
	"fa": "ふぁ", "fi": "ふぃ", "fe": "ふぇ", "fo": "ふぉ",
	"xa": "ぁ", "xi": "ぃ", "xu": "ぅ", "xe": "ぇ", "xo": "ぉ",
	"xya": "ゃ", "xyu": "ゅ", "xyo": "ょ", "xtu": "っ", "ltu": "っ",
	"-": "ー",
}

func isVowel(c byte) bool { return strings.IndexByte("aiueo", c) >= 0 }

// ToHiragana converts romaji in s to hiragana, leaving anything it does not
// recognize (including kana already converted) untouched. While the user is
// still typing (final is false), a trailing "n" or "nn" stays as romaji,
// because the next key decides whether it is ん or the start of な, にゃ, etc.
func ToHiragana(s string, final bool) string {
	in := strings.ToLower(s)
	var b strings.Builder
	for i := 0; i < len(in); {
		rest := in[i:]
		c := rest[0]

		if c == 'n' {
			switch {
			case rest == "n" || rest == "nn":
				if !final {
					b.WriteString(rest)
					return b.String()
				}
				b.WriteString("ん")
				i += len(rest)
				continue
			case isVowel(rest[1]) || rest[1] == 'y':
				// na, nya...: handled by the table below.
			case rest[1] == 'n' && len(rest) > 2 && (isVowel(rest[2]) || rest[2] == 'y'):
				b.WriteString("ん") // "nna" is ん + な
				i++
				continue
			case rest[1] == 'n' || rest[1] == '\'':
				b.WriteString("ん")
				i += 2
				continue
			default:
				b.WriteString("ん")
				i++
				continue
			}
		}

		// A doubled consonant (kk, ss, tt...) or "tch" becomes a small っ.
		if len(rest) > 1 && c >= 'a' && c <= 'z' && !isVowel(c) &&
			(rest[1] == c || strings.HasPrefix(rest, "tch")) {
			b.WriteString("っ")
			i++
			continue
		}

		matched := false
		for n := min(3, len(rest)); n >= 1; n-- {
			if kana, ok := romaji[rest[:n]]; ok {
				b.WriteString(kana)
				i += n
				matched = true
				break
			}
		}
		if matched {
			continue
		}

		// Not romaji: copy one whole UTF-8 character through unchanged.
		r, size := utf8.DecodeRuneInString(rest)
		b.WriteRune(r)
		i += size
	}
	return b.String()
}

// KatakanaToHiragana shifts katakana (ァ..ヶ) down to the matching hiragana.
func KatakanaToHiragana(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'ァ' && r <= 'ヶ' {
			return r - ('ァ' - 'ぁ')
		}
		return r
	}, s)
}

func containsKana(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool { return r >= 'ぁ' && r <= 'ヿ' })
}

func containsLatin(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool { return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' })
}
```

Note: the `n` branch never double-consonants `nn` because `c == 'n'` always `continue`s or falls to the table (the fall-through case only happens when `rest[1]` is a vowel or `y`, so `rest[1] != c`).

- [ ] **Step 4: Run tests**

Run: `go test ./review/ -v -run 'Hiragana|LiveTyping|Katakana'`
Expected: PASS for every subtest.

- [ ] **Step 5: Commit**

```bash
gofmt -w . && go vet ./...
git add review/kana.go review/kana_test.go
git commit -m "review: romaji and katakana to hiragana conversion"
```

---

### Task 3: Items and grading

**Files:**
- Create: `review/item.go`, `review/grade.go`, `review/grade_test.go`

**Interfaces:**
- Consumes: `ToHiragana`, `KatakanaToHiragana`, `containsKana`, `containsLatin` (Task 2).
- Produces:
  ```go
  type Part int              // Meaning, Reading; String() "meaning"/"reading"
  type Item struct {
      AssignmentID  int
      Type          string   // "radical", "kanji", "vocabulary", "kana_vocabulary"
      Characters    string
      Meanings      []string // accepted answers, primary first
      Blacklist     []string
      Readings      []string // accepted answers, primary first; empty = meaning only
      OtherReadings []string // kanji readings WaniKani knows but is not asking for
      ReadingKind   string   // "on'yomi", "kun'yomi", "nanori", or ""
  }
  func (it Item) HasReading() bool
  type Verdict int           // Wrong, Correct, CorrectTypo, Warn
  type Grade struct { Verdict Verdict; Hint string }
  func GradeMeaning(it Item, input string) Grade
  func GradeReading(it Item, input string) Grade
  ```

**Go concepts to explain:** named types and `iota` enums; methods on value receivers (`String()` makes `%v` print nicely via `fmt.Stringer`); struct literals; `[][]int` slices of slices; the built-in `min` (Go 1.21); why `review` keeps its own `Item` instead of reusing the API's JSON types (dependency direction).

- [ ] **Step 1: Write the failing tests** (`review/grade_test.go`)

```go
package review

import (
	"strings"
	"testing"
)

var (
	big = Item{
		Type: "kanji", Characters: "大",
		Meanings:      []string{"Big", "Large"},
		Readings:      []string{"たい", "だい"},
		OtherReadings: []string{"おお"},
		ReadingKind:   "on'yomi",
	}
	adult = Item{
		Type: "vocabulary", Characters: "大人",
		Meanings:  []string{"Adult", "Grown Up"},
		Blacklist: []string{"Big Person"},
		Readings:  []string{"おとな"},
	}
)

func TestGradeMeaning(t *testing.T) {
	tests := []struct {
		item  Item
		input string
		want  Verdict
	}{
		{big, "big", Correct},
		{big, "  BIG ", Correct},
		{adult, "  Grown   UP ", Correct},
		{big, "larg", CorrectTypo},   // "large" has 5 letters: 1 edit allowed
		{adult, "adlut", CorrectTypo}, // transposition counts as 1 edit
		{big, "bog", Wrong},           // 3 letters: no edits allowed
		{adult, "big person", Wrong},  // blacklisted
		{adult, "adult2", Wrong},      // digits must match exactly
		{big, "たい", Warn},             // kana in a meaning answer
	}
	for _, tt := range tests {
		if got := GradeMeaning(tt.item, tt.input); got.Verdict != tt.want {
			t.Errorf("GradeMeaning(%s, %q) = %v, want %v", tt.item.Characters, tt.input, got.Verdict, tt.want)
		}
	}
}

func TestGradeReading(t *testing.T) {
	tests := []struct {
		input string
		want  Verdict
	}{
		{"tai", Correct},
		{"dai", Correct},
		{"タイ", Correct},
		{"たい", Correct},
		{"ookii", Wrong},
		{"big", Warn}, // still has latin letters after conversion
	}
	for _, tt := range tests {
		if got := GradeReading(big, tt.input); got.Verdict != tt.want {
			t.Errorf("GradeReading(%q) = %v, want %v", tt.input, got.Verdict, tt.want)
		}
	}
	g := GradeReading(big, "oo")
	if g.Verdict != Warn || !strings.Contains(g.Hint, "on'yomi") {
		t.Errorf("GradeReading(oo) = %+v, want a Warn mentioning on'yomi", g)
	}
}

func TestOSADistance(t *testing.T) {
	for _, tt := range []struct {
		a, b string
		want int
	}{{"abc", "acb", 1}, {"kitten", "sitting", 3}, {"", "abc", 3}, {"same", "same", 0}} {
		if got := osaDistance(tt.a, tt.b); got != tt.want {
			t.Errorf("osaDistance(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestTypoTolerance(t *testing.T) {
	for n, want := range map[int]int{3: 0, 4: 1, 5: 1, 6: 2, 7: 2, 8: 3, 14: 4} {
		if got := typoTolerance(n); got != want {
			t.Errorf("typoTolerance(%d) = %d, want %d", n, got, want)
		}
	}
}
```

- [ ] **Step 2: Run to see it fail**

Run: `go test ./review/`
Expected: FAIL, `undefined: Item`.

- [ ] **Step 3: Implement `review/item.go`**

```go
package review

// Part is which half of an item is being asked.
type Part int

const (
	Meaning Part = iota
	Reading
)

func (p Part) String() string {
	if p == Reading {
		return "reading"
	}
	return "meaning"
}

// Item is one thing to review, already flattened from the API's subject,
// assignment, and study-material data.
type Item struct {
	AssignmentID  int
	Type          string   // "radical", "kanji", "vocabulary", "kana_vocabulary"
	Characters    string
	Meanings      []string // accepted answers, primary first
	Blacklist     []string
	Readings      []string // accepted answers, primary first; empty means meaning only
	OtherReadings []string // kanji readings WaniKani knows but is not asking for
	ReadingKind   string   // "on'yomi", "kun'yomi", "nanori", or ""
}

// HasReading reports whether this item asks for a reading at all.
// Radicals and kana-only vocabulary do not.
func (it Item) HasReading() bool { return len(it.Readings) > 0 }
```

- [ ] **Step 4: Implement `review/grade.go`**

```go
package review

import (
	"strings"
	"unicode/utf8"
)

// Verdict is the outcome of grading one answer.
type Verdict int

const (
	Wrong       Verdict = iota
	Correct             // exact match
	CorrectTypo         // accepted within typo tolerance
	Warn                // not graded: the user typed the wrong kind of answer
)

func (v Verdict) String() string {
	return [...]string{"Wrong", "Correct", "CorrectTypo", "Warn"}[v]
}

// Grade is a Verdict plus, for Warn, a hint to show the user.
type Grade struct {
	Verdict Verdict
	Hint    string
}

func normalizeMeaning(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// GradeMeaning checks a meaning answer the way the website does: exact match
// against accepted meanings and synonyms, blacklist first, then typo tolerance.
func GradeMeaning(it Item, input string) Grade {
	answer := normalizeMeaning(input)
	if containsKana(answer) {
		return Grade{Warn, "We want the meaning, not the reading."}
	}
	for _, b := range it.Blacklist {
		if answer == normalizeMeaning(b) {
			return Grade{Verdict: Wrong}
		}
	}
	for _, m := range it.Meanings {
		if answer == normalizeMeaning(m) {
			return Grade{Verdict: Correct}
		}
	}
	if strings.ContainsAny(answer, "0123456789") {
		return Grade{Verdict: Wrong}
	}
	for _, m := range it.Meanings {
		m = normalizeMeaning(m)
		if osaDistance(answer, m) <= typoTolerance(utf8.RuneCountInString(m)) {
			return Grade{Verdict: CorrectTypo}
		}
	}
	return Grade{Verdict: Wrong}
}

// GradeReading checks a reading answer: exact match after conversion to
// hiragana. A kanji reading of the wrong type is a warning, not a miss.
func GradeReading(it Item, input string) Grade {
	answer := KatakanaToHiragana(ToHiragana(strings.TrimSpace(input), true))
	if containsLatin(answer) {
		return Grade{Warn, "We want the reading, in kana."}
	}
	for _, r := range it.Readings {
		if answer == KatakanaToHiragana(r) {
			return Grade{Verdict: Correct}
		}
	}
	for _, r := range it.OtherReadings {
		if answer == KatakanaToHiragana(r) {
			if it.ReadingKind != "" {
				return Grade{Warn, "Close, but we want the " + it.ReadingKind + "."}
			}
			return Grade{Warn, "Close, but that's not the reading we want here."}
		}
	}
	return Grade{Verdict: Wrong}
}

// typoTolerance is how many edits a meaning of n letters allows.
// Community reverse-engineered from the website (see the spec).
func typoTolerance(n int) int {
	switch {
	case n <= 3:
		return 0
	case n <= 5:
		return 1
	case n <= 7:
		return 2
	default:
		return 2 + n/7
	}
}

// osaDistance is the Optimal String Alignment distance: Levenshtein edits
// plus swapping two adjacent letters as a single edit.
func osaDistance(a, b string) int {
	s, t := []rune(a), []rune(b)
	d := make([][]int, len(s)+1)
	for i := range d {
		d[i] = make([]int, len(t)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(s); i++ {
		for j := 1; j <= len(t); j++ {
			cost := 1
			if s[i-1] == t[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && s[i-1] == t[j-2] && s[i-2] == t[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(s)][len(t)]
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./review/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
gofmt -w . && go vet ./...
git add review/item.go review/grade.go review/grade_test.go
git commit -m "review: items and answer grading with typo tolerance"
```

---

### Task 4: Back-to-back review session

**Files:**
- Create: `review/session.go`, `review/session_test.go`

**Interfaces:**
- Consumes: `Item`, `Part`, `GradeMeaning`, `GradeReading`, `Grade`, `Verdict` (Task 3).
- Produces:
  ```go
  type Submission struct { AssignmentID, IncorrectMeaning, IncorrectReading int }
  type Result struct { Item Item; Submission Submission }
  func NewSession(items []Item, rng *rand.Rand) *Session   // math/rand/v2
  func (s *Session) Current() (Item, Part, bool)
  func (s *Session) Answer(input string) (Grade, *Submission)
  func (s *Session) Total() int
  func (s *Session) Results() []Result
  ```

**Go concepts to explain:** pointer receivers (`*Session`) because methods mutate state; returning `*Submission` as an "optional" value (nil = none); `math/rand/v2` and why injecting a seeded `*rand.Rand` makes tests deterministic; `slices.Clone` so the caller's slice is not reshuffled; arrays (`[2]int`) indexed by a typed constant.

- [ ] **Step 1: Write the failing tests** (`review/session_test.go`)

```go
package review

import (
	"math/rand/v2"
	"testing"
)

var ground = Item{AssignmentID: 1, Type: "radical", Characters: "一", Meanings: []string{"Ground"}}

func seeded(n uint64) *rand.Rand { return rand.New(rand.NewPCG(n, n)) }

// correct returns a right answer for the given part of an item.
func correct(it Item, p Part) string {
	if p == Reading {
		return it.Readings[0]
	}
	return it.Meanings[0]
}

func TestRadicalAsksMeaningOnly(t *testing.T) {
	s := NewSession([]Item{ground}, seeded(1))
	if _, p, _ := s.Current(); p != Meaning {
		t.Fatalf("first part = %v, want meaning", p)
	}
	if g, sub := s.Answer("sky"); g.Verdict != Wrong || sub != nil {
		t.Fatalf("wrong answer: got %v, %v", g.Verdict, sub)
	}
	g, sub := s.Answer("ground")
	if g.Verdict != Correct || sub == nil {
		t.Fatalf("right answer: got %v, %v", g.Verdict, sub)
	}
	want := Submission{AssignmentID: 1, IncorrectMeaning: 1}
	if *sub != want {
		t.Errorf("submission = %+v, want %+v", *sub, want)
	}
	if _, _, ok := s.Current(); ok {
		t.Error("session should be over")
	}
}

func TestBackToBack(t *testing.T) {
	kanji := big
	kanji.AssignmentID = 2
	s := NewSession([]Item{kanji, ground}, seeded(7))
	if it, p, _ := s.Current(); !it.HasReading() {
		s.Answer(correct(it, p)) // get the radical out of the way
	}
	first, p1, _ := s.Current()
	if _, sub := s.Answer(correct(first, p1)); sub != nil {
		t.Fatal("kanji finished after only one part")
	}
	same, p2, _ := s.Current()
	if same.AssignmentID != first.AssignmentID || p2 == p1 {
		t.Fatalf("after part %v of %s, got part %v of %s; want the other part of the same item",
			p1, first.Characters, p2, same.Characters)
	}
	wrong := map[Part]string{Meaning: "definitely wrong", Reading: "ぜんぜん"}
	warn := map[Part]string{Meaning: "あ", Reading: "xyz"}
	if g, _ := s.Answer(wrong[p2]); g.Verdict != Wrong {
		t.Errorf("wrong %v answer graded %v", p2, g.Verdict)
	}
	if g, _ := s.Answer(warn[p2]); g.Verdict != Warn {
		t.Errorf("wrong-kind %v answer graded %v, want Warn", p2, g.Verdict)
	}
	_, sub := s.Answer(correct(same, p2))
	if sub == nil {
		t.Fatal("item should be finished")
	}
	if got := sub.IncorrectMeaning + sub.IncorrectReading; got != 1 {
		t.Errorf("incorrect total = %d, want 1 (warnings do not count)", got)
	}
	if s.Total() != 2 {
		t.Errorf("Total = %d, want 2", s.Total())
	}
}

func TestPartOrderIsRandom(t *testing.T) {
	seen := map[Part]bool{}
	for seed := range uint64(40) {
		_, p, _ := NewSession([]Item{big}, seeded(seed)).Current()
		seen[p] = true
	}
	if !seen[Meaning] || !seen[Reading] {
		t.Errorf("over 40 seeds saw %v; want both meaning-first and reading-first", seen)
	}
}
```

- [ ] **Step 2: Run to see it fail**

Run: `go test ./review/`
Expected: FAIL, `undefined: NewSession`.

- [ ] **Step 3: Implement** (`review/session.go`)

```go
package review

import (
	"math/rand/v2"
	"slices"
)

// Submission is what gets sent to WaniKani when an item is finished.
type Submission struct {
	AssignmentID     int
	IncorrectMeaning int
	IncorrectReading int
}

// Result pairs a finished item with its submission, for the summary screen.
type Result struct {
	Item       Item
	Submission Submission
}

// Session runs items back to back: one item at a time, meaning or reading
// first by coin flip, and a wrong part is asked again until it is right.
type Session struct {
	items   []Item
	pos     int
	parts   []Part // parts of the current item still to answer; parts[0] is being asked
	wrong   [2]int // incorrect answers for the current item, indexed by Part
	results []Result
	rng     *rand.Rand
}

// NewSession shuffles a copy of items and starts on the first one.
func NewSession(items []Item, rng *rand.Rand) *Session {
	s := &Session{items: slices.Clone(items), rng: rng}
	rng.Shuffle(len(s.items), func(i, j int) { s.items[i], s.items[j] = s.items[j], s.items[i] })
	s.start()
	return s
}

func (s *Session) start() {
	s.wrong = [2]int{}
	s.parts = nil
	if s.pos >= len(s.items) {
		return
	}
	s.parts = []Part{Meaning}
	if s.items[s.pos].HasReading() {
		s.parts = append(s.parts, Reading)
		if s.rng.IntN(2) == 0 {
			s.parts[0], s.parts[1] = Reading, Meaning
		}
	}
}

// Current returns the item and part being asked. ok is false once every item is done.
func (s *Session) Current() (it Item, p Part, ok bool) {
	if s.pos >= len(s.items) {
		return Item{}, Meaning, false
	}
	return s.items[s.pos], s.parts[0], true
}

// Answer grades input against the current question. If that finishes the
// item, it returns the Submission to send; otherwise the Submission is nil.
func (s *Session) Answer(input string) (Grade, *Submission) {
	it, part, ok := s.Current()
	if !ok {
		return Grade{Verdict: Wrong}, nil
	}
	var g Grade
	if part == Meaning {
		g = GradeMeaning(it, input)
	} else {
		g = GradeReading(it, input)
	}
	switch g.Verdict {
	case Wrong:
		s.wrong[part]++
	case Correct, CorrectTypo:
		s.parts = s.parts[1:]
		if len(s.parts) == 0 {
			sub := Submission{it.AssignmentID, s.wrong[Meaning], s.wrong[Reading]}
			s.results = append(s.results, Result{it, sub})
			s.pos++
			s.start()
			return g, &sub
		}
	}
	return g, nil
}

// Total is the number of items in the session.
func (s *Session) Total() int { return len(s.items) }

// Results lists finished items in the order they were finished.
func (s *Session) Results() []Result { return s.results }
```

- [ ] **Step 4: Run tests**

Run: `go test ./review/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w . && go vet ./...
git add review/session.go review/session_test.go
git commit -m "review: back-to-back session order"
```

---

### Task 5: WaniKani HTTP client core

**Files:**
- Create: `wanikani/types.go`, `wanikani/client.go`, `wanikani/client_test.go`

**Interfaces:**
- Produces:
  ```go
  const BaseURL = "https://api.wanikani.com/v2/"
  var ErrUnauthorized error
  type APIError struct { Status int; Method, URL, Body string }   // Error() string
  type Client struct { /* unexported */ }
  func NewClient(baseURL, token string) *Client
  func (c *Client) do(ctx context.Context, method, url string, body, out any) error  // package-internal, used by Task 6
  type Resource[T any] struct { ID int; Object string; DataUpdatedAt time.Time; Data T }
  type page[T any] struct { Pages struct{ NextURL string }; Data []Resource[T] }
  type Subject, Meaning, AuxMeaning, Reading, Assignment, StudyMaterial, User  // see code
  ```

**Go concepts to explain:** struct tags and `encoding/json`; `*string` for JSON `null`; generics (`Resource[T any]`) and why they fit an API where every object has the same envelope; `context.Context` for cancellation and timeouts; sentinel errors (`errors.New`) vs error types (`*APIError`) and how callers use `errors.Is` / `errors.As`; `defer resp.Body.Close()` and why not to `defer` inside a loop; `httptest.NewServer`; passing the base URL into the constructor (dependency injection without a framework).

- [ ] **Step 1: Write `wanikani/types.go`** (no behavior, just shapes)

```go
package wanikani

import "time"

// Resource is the envelope every WaniKani object arrives in.
type Resource[T any] struct {
	ID            int       `json:"id"`
	Object        string    `json:"object"` // "radical", "kanji", "vocabulary", "kana_vocabulary", "assignment", ...
	DataUpdatedAt time.Time `json:"data_updated_at"`
	Data          T         `json:"data"`
}

// page is one page of a collection. NextURL is empty on the last page.
type page[T any] struct {
	Pages struct {
		NextURL string `json:"next_url"`
	} `json:"pages"`
	Data []Resource[T] `json:"data"`
}

type Subject struct {
	Characters        *string      `json:"characters"` // nil for image-only radicals
	Meanings          []Meaning    `json:"meanings"`
	AuxiliaryMeanings []AuxMeaning `json:"auxiliary_meanings"`
	Readings          []Reading    `json:"readings"`
}

type Meaning struct {
	Meaning        string `json:"meaning"`
	Primary        bool   `json:"primary"`
	AcceptedAnswer bool   `json:"accepted_answer"`
}

type AuxMeaning struct {
	Meaning string `json:"meaning"`
	Type    string `json:"type"` // "whitelist" or "blacklist"
}

type Reading struct {
	Reading        string `json:"reading"`
	Primary        bool   `json:"primary"`
	AcceptedAnswer bool   `json:"accepted_answer"`
	Type           string `json:"type"` // kanji only: "onyomi", "kunyomi", "nanori"
}

type Assignment struct {
	SubjectID   int    `json:"subject_id"`
	SubjectType string `json:"subject_type"`
}

type StudyMaterial struct {
	SubjectID       int      `json:"subject_id"`
	MeaningSynonyms []string `json:"meaning_synonyms"`
}

type User struct {
	Username string `json:"username"`
	Level    int    `json:"level"`
}
```

- [ ] **Step 2: Write the failing tests** (`wanikani/client_test.go`)

```go
package wanikani

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return NewClient(srv.URL+"/", "tok123")
}

func TestHeadersAndDecode(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tok123" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("Wanikani-Revision"); got != "20170710" {
			t.Errorf("Wanikani-Revision = %q", got)
		}
		fmt.Fprint(w, `{"data":{"username":"durtle","level":5}}`)
	})
	var r Resource[User]
	if err := c.do(context.Background(), http.MethodGet, c.base+"user", nil, &r); err != nil {
		t.Fatal(err)
	}
	if r.Data.Username != "durtle" || r.Data.Level != 5 {
		t.Errorf("decoded %+v", r.Data)
	}
}

func TestUnauthorized(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	err := c.do(context.Background(), http.MethodGet, c.base+"user", nil, nil)
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("err = %v, want ErrUnauthorized", err)
	}
}

func TestAPIError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		fmt.Fprint(w, `{"error":"not available"}`)
	})
	err := c.do(context.Background(), http.MethodPost, c.base+"reviews", map[string]int{"x": 1}, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 422 {
		t.Fatalf("err = %v, want *APIError with status 422", err)
	}
}

func TestRetriesAfterRateLimit(t *testing.T) {
	hits := 0
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits == 1 {
			w.Header().Set("RateLimit-Reset", fmt.Sprint(time.Now().Add(-time.Second).Unix()))
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		fmt.Fprint(w, `{}`)
	})
	if err := c.do(context.Background(), http.MethodGet, c.base+"user", nil, nil); err != nil {
		t.Fatal(err)
	}
	if hits != 2 {
		t.Errorf("hits = %d, want 2", hits)
	}
}
```

- [ ] **Step 3: Run to see it fail**

Run: `go test ./wanikani/`
Expected: FAIL, `undefined: NewClient`.

- [ ] **Step 4: Implement** (`wanikani/client.go`)

```go
// Package wanikani is a minimal client for the parts of the WaniKani API v2
// that durtle-tui uses.
package wanikani

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// BaseURL is the production API root.
const BaseURL = "https://api.wanikani.com/v2/"

const maxRateLimitRetries = 3

// ErrUnauthorized means WaniKani rejected the token (HTTP 401).
var ErrUnauthorized = errors.New("wanikani: API token was rejected")

// APIError is any other non-2xx response.
type APIError struct {
	Status      int
	Method, URL string
	Body        string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("wanikani: %s %s: HTTP %d: %s", e.Method, e.URL, e.Status, e.Body)
}

// Client talks to the WaniKani API with one personal access token.
type Client struct {
	base  string
	token string
	http  *http.Client
}

// NewClient returns a client for baseURL (normally BaseURL; tests pass a local server).
func NewClient(baseURL, token string) *Client {
	return &Client{base: baseURL, token: token, http: &http.Client{Timeout: 30 * time.Second}}
}

// do sends one request, waiting and retrying while rate limited, and decodes
// the JSON response into out unless out is nil.
func (c *Client) do(ctx context.Context, method, url string, body, out any) error {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return err
		}
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Wanikani-Revision", "20170710")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return err
		}
		if resp.StatusCode == http.StatusTooManyRequests && attempt < maxRateLimitRetries {
			resp.Body.Close()
			if err := waitUntil(ctx, resetTime(resp.Header)); err != nil {
				return err
			}
			continue
		}
		return decode(resp, method, url, out)
	}
}

func decode(resp *http.Response, method, url string, out any) error {
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return ErrUnauthorized
	}
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return &APIError{Status: resp.StatusCode, Method: method, URL: url, Body: string(bytes.TrimSpace(msg))}
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("wanikani: decoding %s: %w", url, err)
	}
	return nil
}

// resetTime reads when the rate limit window resets. Without the header,
// wait a few seconds and try again.
func resetTime(h http.Header) time.Time {
	secs, err := strconv.ParseInt(h.Get("RateLimit-Reset"), 10, 64)
	if err != nil {
		return time.Now().Add(5 * time.Second)
	}
	return time.Unix(secs, 0)
}

// waitUntil blocks until t or until ctx is cancelled, whichever comes first.
func waitUntil(ctx context.Context, t time.Time) error {
	timer := time.NewTimer(time.Until(t)) // a past time fires immediately
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./wanikani/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
gofmt -w . && go vet ./...
git add wanikani/
git commit -m "wanikani: HTTP client with auth headers, errors, and rate limit retry"
```

---

### Task 6: WaniKani endpoints

**Files:**
- Create: `wanikani/endpoints.go`, `wanikani/endpoints_test.go`

**Interfaces:**
- Consumes: `Client.do`, `page[T]`, `Resource[T]`, types (Task 5).
- Produces:
  ```go
  func (c *Client) User(ctx context.Context) (User, error)
  func (c *Client) Subjects(ctx context.Context, updatedAfter time.Time) ([]Resource[Subject], error)
  func (c *Client) StudyMaterials(ctx context.Context, updatedAfter time.Time) ([]Resource[StudyMaterial], error)
  func (c *Client) ReviewAssignments(ctx context.Context) ([]Resource[Assignment], error)
  func (c *Client) SubmitReview(ctx context.Context, assignmentID, incorrectMeaning, incorrectReading int) error
  ```
  A zero `updatedAfter` means "everything".

**Go concepts to explain:** why `getAll` is a generic *function* and not a method (Go methods cannot have their own type parameters); `net/url` query escaping; `time.Time.IsZero` as "not set"; anonymous structs for one-off JSON bodies.

- [ ] **Step 1: Write the failing tests** (`wanikani/endpoints_test.go`)

```go
package wanikani

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSubjectsFollowsPages(t *testing.T) {
	var base string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page_after_id") == "" {
			if got := r.URL.Query().Get("updated_after"); got != "2026-01-02T03:04:05Z" {
				t.Errorf("updated_after = %q", got)
			}
			fmt.Fprintf(w, `{"pages":{"next_url":%q},"data":[{"id":1,"object":"kanji","data":{"characters":"大"}}]}`,
				base+"subjects?page_after_id=1")
			return
		}
		fmt.Fprint(w, `{"pages":{"next_url":null},"data":[{"id":2,"object":"radical","data":{"characters":null}}]}`)
	})
	base = c.base
	got, err := c.Subjects(context.Background(), time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || *got[0].Data.Characters != "大" || got[1].Data.Characters != nil {
		t.Errorf("got %+v", got)
	}
}

func TestReviewAssignmentsQuery(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/assignments" || !strings.Contains(r.URL.RawQuery, "immediately_available_for_review") {
			t.Errorf("request = %s", r.URL)
		}
		fmt.Fprint(w, `{"pages":{"next_url":null},"data":[{"id":9,"object":"assignment","data":{"subject_id":1,"subject_type":"kanji"}}]}`)
	})
	got, err := c.ReviewAssignments(context.Background())
	if err != nil || len(got) != 1 || got[0].ID != 9 || got[0].Data.SubjectID != 1 {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestSubmitReviewBody(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/reviews" {
			t.Errorf("request = %s %s", r.Method, r.URL)
		}
		var body struct {
			Review map[string]int `json:"review"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		want := map[string]int{"assignment_id": 9, "incorrect_meaning_answers": 1, "incorrect_reading_answers": 2}
		for k, v := range want {
			if body.Review[k] != v {
				t.Errorf("%s = %d, want %d", k, body.Review[k], v)
			}
		}
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{}`)
	})
	if err := c.SubmitReview(context.Background(), 9, 1, 2); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Run to see it fail**

Run: `go test ./wanikani/`
Expected: FAIL, `c.Subjects undefined`.

- [ ] **Step 3: Implement** (`wanikani/endpoints.go`)

```go
package wanikani

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// getAll fetches every page of a collection, following pages.next_url.
func getAll[T any](ctx context.Context, c *Client, path string) ([]Resource[T], error) {
	var all []Resource[T]
	for next := c.base + path; next != ""; {
		var p page[T]
		if err := c.do(ctx, http.MethodGet, next, nil, &p); err != nil {
			return nil, err
		}
		all = append(all, p.Data...)
		next = p.Pages.NextURL
	}
	return all, nil
}

func updatedAfter(path string, t time.Time) string {
	if t.IsZero() {
		return path
	}
	return path + "?updated_after=" + url.QueryEscape(t.UTC().Format(time.RFC3339))
}

// User fetches the token owner's profile. Used to check a token is valid.
func (c *Client) User(ctx context.Context) (User, error) {
	var r Resource[User]
	err := c.do(ctx, http.MethodGet, c.base+"user", nil, &r)
	return r.Data, err
}

// Subjects fetches subjects changed after t (all of them if t is zero).
func (c *Client) Subjects(ctx context.Context, t time.Time) ([]Resource[Subject], error) {
	return getAll[Subject](ctx, c, updatedAfter("subjects", t))
}

// StudyMaterials fetches the user's notes and synonyms changed after t.
func (c *Client) StudyMaterials(ctx context.Context, t time.Time) ([]Resource[StudyMaterial], error) {
	return getAll[StudyMaterial](ctx, c, updatedAfter("study_materials", t))
}

// ReviewAssignments fetches assignments that are due for review right now.
func (c *Client) ReviewAssignments(ctx context.Context) ([]Resource[Assignment], error) {
	return getAll[Assignment](ctx, c, "assignments?immediately_available_for_review=true&hidden=false")
}

// SubmitReview records a finished review for one assignment.
func (c *Client) SubmitReview(ctx context.Context, assignmentID, incorrectMeaning, incorrectReading int) error {
	body := map[string]any{"review": map[string]int{
		"assignment_id":             assignmentID,
		"incorrect_meaning_answers": incorrectMeaning,
		"incorrect_reading_answers": incorrectReading,
	}}
	return c.do(ctx, http.MethodPost, c.base+"reviews", body, nil)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./wanikani/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w . && go vet ./...
git add wanikani/endpoints.go wanikani/endpoints_test.go
git commit -m "wanikani: user, subjects, study materials, assignments, submit review"
```

---

### Task 7: Token store, JSON files, and the backend

**Files:**
- Create: `store/store.go`, `store/store_test.go`, `backend.go`, `backend_test.go`

**Interfaces:**
- Consumes: `wanikani.NewClient`, `Client.User`, `Client.Subjects`, `Client.StudyMaterials`, `Client.ReviewAssignments`, `Client.SubmitReview`, `wanikani.ErrUnauthorized`, `*wanikani.APIError` (Tasks 5 and 6); `review.Item`, `review.Submission` (Tasks 3 and 4).
- Produces:
  ```go
  // store
  var ErrNoToken error
  func LoadToken() (string, error)
  func SaveToken(token string) error
  func CacheDir() (string, error)
  func ReadJSON(path string, v any) error   // missing file: nil, v untouched
  func WriteJSON(path string, v any) error  // atomic
  // package main
  type backend struct { dir, base string; client *wanikani.Client; mu sync.Mutex }
  func (b *backend) Login(ctx context.Context, token string) error
  func (b *backend) Load(ctx context.Context) ([]review.Item, error)
  func (b *backend) Submit(ctx context.Context, s review.Submission) (pending bool, err error)
  func buildItems(assignments []wanikani.Resource[wanikani.Assignment], subjects map[int]wanikani.Resource[wanikani.Subject], synonyms map[int][]string) []review.Item
  ```
  These three methods are exactly the `ui.Backend` interface in Task 8.

**Go concepts to explain:** adding a dependency (`go get`, `go.mod` / `go.sum` and why `go.sum` is committed); `os.UserConfigDir` / `os.UserCacheDir` for cross-platform paths; file permission literals (`0o600`); write-to-temp-then-rename for atomic writes; `sync.Mutex` and why concurrent submits need it (Bubble Tea runs commands on goroutines); `errors.As` with a pointer to a pointer; `%w` wrapping; `map[int]T` in JSON (keys become strings); `t.Setenv` and `t.TempDir` in tests.

- [ ] **Step 1: Add the keyring dependency**

Run: `go get github.com/zalando/go-keyring@v0.2.8`
Then confirm the mock helpers exist: `go doc github.com/zalando/go-keyring MockInitWithError`
Expected: a function signature. If it is missing, use `keyring.MockInit()` only and drop the fallback-file test's keyring failure in Step 2 in favor of testing `writeTokenFile`/`readTokenFile` directly.

- [ ] **Step 2: Write the failing store tests** (`store/store_test.go`)

```go
package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestTokenKeyring(t *testing.T) {
	keyring.MockInit()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := LoadToken(); !errors.Is(err, ErrNoToken) {
		t.Fatalf("empty store: err = %v, want ErrNoToken", err)
	}
	if err := SaveToken("abc"); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadToken(); err != nil || got != "abc" {
		t.Errorf("LoadToken = %q, %v", got, err)
	}
}

func TestTokenFileFallback(t *testing.T) {
	keyring.MockInitWithError(errors.New("no keyring here"))
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := SaveToken("abc"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "durtle-tui", "token"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("token file mode = %o, want 600", perm)
	}
	if got, err := LoadToken(); err != nil || got != "abc" {
		t.Errorf("LoadToken = %q, %v", got, err)
	}
}

func TestJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "x.json")
	v := map[int]string{1: "a"}
	if err := ReadJSON(path, &v); err != nil || v[1] != "a" {
		t.Fatalf("missing file should be a no-op: %v %v", v, err)
	}
	if err := WriteJSON(path, map[int]string{2: "b"}); err != nil {
		t.Fatal(err)
	}
	var got map[int]string
	if err := ReadJSON(path, &got); err != nil || got[2] != "b" {
		t.Errorf("round trip = %v, %v", got, err)
	}
	os.WriteFile(path, []byte("{not json"), 0o600)
	if err := ReadJSON(path, &got); err == nil {
		t.Error("corrupt file should return an error")
	}
}
```

- [ ] **Step 3: Run to see it fail**

Run: `go test ./store/`
Expected: FAIL, `undefined: LoadToken`.

- [ ] **Step 4: Implement** (`store/store.go`)

```go
// Package store keeps durtle-tui's token and JSON files on disk.
package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/zalando/go-keyring"
)

const (
	appName     = "durtle-tui"
	keyringUser = "api-token"
)

// ErrNoToken means no token has been saved yet.
var ErrNoToken = errors.New("no API token saved")

// LoadToken reads the token from the OS keyring, or from the fallback file.
func LoadToken() (string, error) {
	if tok, err := keyring.Get(appName, keyringUser); err == nil {
		return tok, nil
	}
	path, err := tokenPath()
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", ErrNoToken
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// SaveToken stores the token in the OS keyring. If there is no usable
// keyring, it writes a file only the current user can read.
func SaveToken(tok string) error {
	if err := keyring.Set(appName, keyringUser, tok); err == nil {
		return nil
	}
	path, err := tokenPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(tok+"\n"), 0o600)
}

func tokenPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, appName, "token"), nil
}

// CacheDir is where synced data lives, e.g. ~/.cache/durtle-tui on Linux.
func CacheDir() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, appName), nil
}

// ReadJSON decodes the file at path into v. A missing file is not an error
// and leaves v untouched.
func ReadJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// WriteJSON writes v to path atomically: it writes a temp file and renames
// it over the old one, so a crash never leaves a half-written file.
func WriteJSON(path string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
```

- [ ] **Step 5: Run store tests**

Run: `go test ./store/ -v`
Expected: PASS.

- [ ] **Step 6: Write the failing backend tests** (`backend_test.go`)

```go
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
			Characters: ptr("大"),
			Meanings:   []wanikani.Meaning{{Meaning: "Large", AcceptedAnswer: true}, {Meaning: "Big", Primary: true, AcceptedAnswer: true}},
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
		{ID: 20, Data: wanikani.Assignment{SubjectID: 2}}, // image-only radical: skipped
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
```

- [ ] **Step 7: Run to see it fail**

Run: `go test .`
Expected: FAIL, `undefined: backend`.

- [ ] **Step 8: Implement** (`backend.go`)

```go
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
// than 401 or 429), so retrying it later would never succeed.
func rejected(err error) bool {
	var apiErr *wanikani.APIError
	return errors.As(err, &apiErr) && apiErr.Status >= 400 && apiErr.Status < 500 &&
		apiErr.Status != 401 && apiErr.Status != 429
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
```

- [ ] **Step 9: Run tests**

Run: `go test ./... -v`
Expected: PASS for `store`, `review`, `wanikani`, and the root package.

- [ ] **Step 10: Commit**

```bash
gofmt -w . && go vet ./...
git add go.mod go.sum store/ backend.go backend_test.go
git commit -m "Add token store, JSON cache, pending queue, and backend"
```

---

### Task 8: The Bubble Tea UI

**Files:**
- Create: `ui/model.go`, `ui/view.go`, `ui/model_test.go`

**Interfaces:**
- Consumes: `review.Item`, `review.Session`, `review.NewSession`, `review.Submission`, `review.Result`, `review.ToHiragana`, `review.Part`, `review.Verdict` values (Tasks 2 to 4); `wanikani.ErrUnauthorized` (Task 5).
- Produces:
  ```go
  type Backend interface {
      Login(ctx context.Context, token string) error
      Load(ctx context.Context) ([]review.Item, error)
      Submit(ctx context.Context, s review.Submission) (pending bool, err error)
  }
  func New(b Backend) Model   // Model implements tea.Model
  ```

**Go concepts to explain:** the Elm architecture (Model, Update, View) as Bubble Tea uses it; value receivers returning a modified copy of the model; type switches (`switch msg := msg.(type)`); `tea.Cmd` as "a function that runs on another goroutine and returns a message", which is how the UI stays responsive during HTTP calls; interfaces are satisfied implicitly (no `implements` keyword), so `*backend` from Task 7 is a `ui.Backend` without mentioning it; closures capturing variables in commands.

- [ ] **Step 1: Add the Charm dependencies and check the v2 API**

Run:
```bash
go get charm.land/bubbletea/v2@v2.0.10 charm.land/bubbles/v2@v2.2.1 charm.land/lipgloss/v2@v2.0.6
go doc charm.land/bubbletea/v2 Model
go doc charm.land/bubbletea/v2 KeyPressMsg
go doc charm.land/bubbletea/v2 NewView
go doc charm.land/bubbles/v2/textinput Model.Focus
```
Expected: `Model` has `View() View`; `KeyPressMsg` has a `String()` method; `NewView` exists; note whether `Focus` returns a `tea.Cmd`. If any signature differs from the code below, adapt the code to the real API and mention the difference to the user before continuing.

- [ ] **Step 2: Write the failing tests** (`ui/model_test.go`)

```go
package ui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/ParkerSuzuki/durtle-tui/review"
	"github.com/ParkerSuzuki/durtle-tui/wanikani"
)

type fakeBackend struct {
	items     []review.Item
	loadErr   error
	submitted []review.Submission
}

func (f *fakeBackend) Login(context.Context, string) error { return nil }
func (f *fakeBackend) Load(context.Context) ([]review.Item, error) {
	return f.items, f.loadErr
}
func (f *fakeBackend) Submit(_ context.Context, s review.Submission) (bool, error) {
	f.submitted = append(f.submitted, s)
	return false, nil
}

var ground = review.Item{AssignmentID: 1, Type: "radical", Characters: "一", Meanings: []string{"Ground"}}

// step runs one Update and returns the new model and command.
func step(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

// typeAndEnter puts text in the input and presses Enter.
func typeAndEnter(t *testing.T, m Model, text string) (Model, tea.Cmd) {
	t.Helper()
	m.input.SetValue(text)
	return step(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
}

func TestNoTokenShowsOnboarding(t *testing.T) {
	m, _ := step(t, New(&fakeBackend{}), loadedMsg{err: wanikani.ErrUnauthorized})
	if m.screen != onboarding {
		t.Errorf("screen = %v, want onboarding", m.screen)
	}
}

func TestReviewFlowSubmits(t *testing.T) {
	fb := &fakeBackend{items: []review.Item{ground}}
	m, _ := step(t, New(fb), loadedMsg{items: fb.items})
	if m.screen != reviewing {
		t.Fatalf("screen = %v, want reviewing", m.screen)
	}
	m, _ = typeAndEnter(t, m, "sky")
	if !m.showingAnswer {
		t.Fatal("wrong answer should show the correct answers")
	}
	m, _ = step(t, m, tea.KeyPressMsg{Code: tea.KeyEnter}) // continue
	m, cmd := typeAndEnter(t, m, "ground")
	if m.screen != summary || m.inFlight != 1 || cmd == nil {
		t.Fatalf("screen=%v inFlight=%d cmd=%v", m.screen, m.inFlight, cmd)
	}
	m, _ = step(t, m, cmd()) // run the submit command, feed its message back
	if m.inFlight != 0 || len(fb.submitted) != 1 || fb.submitted[0].IncorrectMeaning != 1 {
		t.Errorf("inFlight=%d submitted=%+v", m.inFlight, fb.submitted)
	}
}

func TestQuitWaitsForSubmits(t *testing.T) {
	fb := &fakeBackend{items: []review.Item{ground}}
	m, _ := step(t, New(fb), loadedMsg{items: fb.items})
	m, submit := typeAndEnter(t, m, "ground")
	m, cmd := step(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd != nil || !m.quitting {
		t.Fatalf("quit with a submit in flight: cmd=%v quitting=%v; want to wait", cmd, m.quitting)
	}
	_, cmd = step(t, m, submit())
	if cmd == nil {
		t.Fatal("expected tea.Quit once the submit finished")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("final command should be tea.Quit")
	}
}
```

- [ ] **Step 3: Run to see it fail**

Run: `go test ./ui/`
Expected: FAIL, `undefined: New`.

- [ ] **Step 4: Implement `ui/model.go`**

```go
// Package ui is durtle-tui's Bubble Tea interface.
package ui

import (
	"context"
	"errors"
	"math/rand/v2"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/ParkerSuzuki/durtle-tui/review"
	"github.com/ParkerSuzuki/durtle-tui/wanikani"
)

// Backend is everything the UI needs from the outside world.
type Backend interface {
	Login(ctx context.Context, token string) error
	Load(ctx context.Context) ([]review.Item, error)
	Submit(ctx context.Context, s review.Submission) (pending bool, err error)
}

type screen int

const (
	loading screen = iota
	onboarding
	reviewing
	summary
	failed
)

type (
	loadedMsg    struct{ items []review.Item; err error }
	loginMsg     struct{ err error }
	submittedMsg struct{ pending bool; err error }
)

// Model is the whole UI state. Update returns a changed copy.
type Model struct {
	backend       Backend
	screen        screen
	input         textinput.Model
	session       *review.Session
	feedback      string
	showingAnswer bool // a wrong answer is on screen; Enter continues
	err           error
	inFlight      int // submits not yet finished
	pending       int // saved to retry next launch
	rejected      int // refused by WaniKani
	quitting      bool
	width         int
}

func New(b Backend) Model {
	in := textinput.New()
	in.Focus()
	return Model{backend: b, screen: loading, input: in}
}

func (m Model) Init() tea.Cmd { return m.load() }

func (m Model) load() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		items, err := m.backend.Load(ctx)
		return loadedMsg{items, err}
	}
}

func (m Model) login(token string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return loginMsg{m.backend.Login(ctx, token)}
	}
}

func (m Model) submit(s review.Submission) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		pending, err := m.backend.Submit(ctx, s)
		return submittedMsg{pending, err}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil
	case loadedMsg:
		return m.loaded(msg)
	case loginMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.screen = loading
		m.input.EchoMode = textinput.EchoNormal
		m.input.Placeholder = ""
		m.input.Reset()
		return m, m.load()
	case submittedMsg:
		m.inFlight--
		switch {
		case msg.err != nil:
			m.rejected++
		case msg.pending:
			m.pending++
		}
		if m.quitting && m.inFlight == 0 {
			return m, tea.Quit
		}
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			return m.quit()
		case "enter":
			return m.enter()
		}
		if m.showingAnswer {
			return m, nil // ignore typing while the correction is shown
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.screen == reviewing {
		if _, part, ok := m.session.Current(); ok && part == review.Reading {
			m.input.SetValue(review.ToHiragana(m.input.Value(), false))
			m.input.CursorEnd()
		}
	}
	return m, cmd
}

func (m Model) loaded(msg loadedMsg) (tea.Model, tea.Cmd) {
	switch {
	case errors.Is(msg.err, wanikani.ErrUnauthorized):
		m.screen = onboarding
		m.input.EchoMode = textinput.EchoPassword
		m.input.Placeholder = "paste your API token"
		m.input.Reset()
		return m, nil
	case msg.err != nil:
		m.screen = failed
		m.err = msg.err
		return m, nil
	}
	m.session = review.NewSession(msg.items, rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())))
	m.screen = reviewing
	if _, _, ok := m.session.Current(); !ok {
		m.screen = summary
	}
	return m, nil
}

func (m Model) enter() (tea.Model, tea.Cmd) {
	switch m.screen {
	case onboarding:
		if tok := strings.TrimSpace(m.input.Value()); tok != "" {
			m.err = nil
			return m, m.login(tok)
		}
	case failed:
		m.screen = loading
		m.err = nil
		return m, m.load()
	case summary:
		return m.quit()
	case reviewing:
		return m.answer()
	}
	return m, nil
}

func (m Model) answer() (tea.Model, tea.Cmd) {
	if m.showingAnswer {
		m.showingAnswer = false
		m.feedback = ""
		m.input.Reset()
		return m, nil
	}
	value := strings.TrimSpace(m.input.Value())
	if value == "" {
		return m, nil
	}
	item, part, _ := m.session.Current()
	grade, sub := m.session.Answer(value)
	switch grade.Verdict {
	case review.Warn:
		m.feedback = grade.Hint
		return m, nil
	case review.Wrong:
		m.showingAnswer = true
		m.feedback = "Wrong. Accepted: " + strings.Join(accepted(item, part), ", ")
		return m, nil
	case review.CorrectTypo:
		m.feedback = "Correct (typo accepted): " + accepted(item, part)[0]
	case review.Correct:
		m.feedback = "Correct"
	}
	m.input.Reset()
	var cmd tea.Cmd
	if sub != nil {
		m.inFlight++
		cmd = m.submit(*sub)
	}
	if _, _, ok := m.session.Current(); !ok {
		m.screen = summary
	}
	return m, cmd
}

func accepted(it review.Item, p review.Part) []string {
	if p == review.Reading {
		return it.Readings
	}
	return it.Meanings
}

func (m Model) quit() (tea.Model, tea.Cmd) {
	if m.inFlight == 0 {
		return m, tea.Quit
	}
	m.quitting = true
	return m, nil
}
```

- [ ] **Step 5: Implement `ui/view.go`** (palette from D16)

```go
package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/ParkerSuzuki/durtle-tui/review"
)

var (
	typeColors = map[string]string{
		"radical":         "#2A9D8F",
		"kanji":           "#E9A23B",
		"vocabulary":      "#6A994E",
		"kana_vocabulary": "#6A994E",
	}
	meaningBar = lipgloss.NewStyle().Bold(true).Padding(0, 2).
			Foreground(lipgloss.Color("#1D1D1D")).Background(lipgloss.Color("#F4F1DE"))
	readingBar = lipgloss.NewStyle().Bold(true).Padding(0, 2).
			Foreground(lipgloss.Color("#F4F1DE")).Background(lipgloss.Color("#3D405B"))
	dim   = lipgloss.NewStyle().Faint(true)
	title = lipgloss.NewStyle().Bold(true)
)

func (m Model) View() tea.View {
	var body string
	switch m.screen {
	case loading:
		body = "Syncing with WaniKani..."
	case onboarding:
		body = m.onboardingView()
	case reviewing:
		body = m.reviewView()
	case summary:
		body = m.summaryView()
	case failed:
		body = fmt.Sprintf("Could not load reviews:\n\n%v\n\n%s", m.err, dim.Render("Enter to retry, Esc to quit"))
	}
	if m.quitting {
		body += "\n\n" + dim.Render(fmt.Sprintf("Finishing %d submission(s) before quitting...", m.inFlight))
	}
	v := tea.NewView(lipgloss.NewStyle().Padding(1, 2).Render(body))
	v.AltScreen = true
	return v
}

func (m Model) onboardingView() string {
	lines := []string{
		title.Render("durtle-tui"),
		dim.Render("An unofficial third-party app for WaniKani."),
		"",
		"To start, create a personal access token at:",
		"  https://www.wanikani.com/settings/personal_access_tokens",
		"Tick the \"reviews:create\" permission, then paste the token here.",
		"",
		m.input.View(),
	}
	if m.err != nil {
		lines = append(lines, "", "That token did not work: "+m.err.Error())
	}
	return strings.Join(lines, "\n")
}

func (m Model) reviewView() string {
	item, part, _ := m.session.Current()
	done := len(m.session.Results())
	chars := lipgloss.NewStyle().Bold(true).Padding(1, 4).
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color(typeColors[item.Type])).
		Render(item.Characters)
	bar := meaningBar
	if part == review.Reading {
		bar = readingBar
	}
	prompt := bar.Render(fmt.Sprintf("%s %s", typeLabel(item.Type), part))
	return strings.Join([]string{
		dim.Render(fmt.Sprintf("%d / %d done   %s correct", done, m.session.Total(), percentCorrect(m.session.Results()))),
		"",
		chars,
		"",
		prompt,
		m.input.View(),
		"",
		m.feedback,
	}, "\n")
}

func (m Model) summaryView() string {
	results := m.session.Results()
	lines := []string{
		title.Render("Session complete"),
		fmt.Sprintf("%d reviewed, %s correct", len(results), percentCorrect(results)),
	}
	var missed []string
	for _, r := range results {
		if r.Submission.IncorrectMeaning+r.Submission.IncorrectReading > 0 {
			missed = append(missed, r.Item.Characters)
		}
	}
	if len(missed) > 0 {
		lines = append(lines, "Missed: "+strings.Join(missed, "  "))
	}
	if m.inFlight > 0 {
		lines = append(lines, fmt.Sprintf("Sending %d...", m.inFlight))
	}
	if m.pending > 0 {
		lines = append(lines, fmt.Sprintf("%d saved offline; they will be sent next launch.", m.pending))
	}
	if m.rejected > 0 {
		lines = append(lines, fmt.Sprintf("%d refused by WaniKani (probably already reviewed elsewhere).", m.rejected))
	}
	lines = append(lines, "", dim.Render("Enter or Esc to quit"))
	return strings.Join(lines, "\n")
}

func typeLabel(t string) string {
	switch t {
	case "radical":
		return "Radical"
	case "kanji":
		return "Kanji"
	}
	return "Vocabulary"
}

func percentCorrect(results []review.Result) string {
	if len(results) == 0 {
		return "0%"
	}
	clean := 0
	for _, r := range results {
		if r.Submission.IncorrectMeaning+r.Submission.IncorrectReading == 0 {
			clean++
		}
	}
	return fmt.Sprintf("%d%%", clean*100/len(results))
}
```

- [ ] **Step 6: Run tests**

Run: `go test ./ui/ -v`
Expected: PASS for all three tests.

- [ ] **Step 7: Commit**

```bash
gofmt -w . && go vet ./...
git add go.mod go.sum ui/
git commit -m "ui: onboarding, review, and summary screens"
```

---

### Task 9: Wire it up and run it for real

**Files:**
- Modify: `main.go`, `README.md`

**Interfaces:**
- Consumes: `store.CacheDir`, `wanikani.BaseURL`, `backend` (Task 7), `ui.New` (Task 8).

**Go concepts to explain:** how `package main` stays thin (wiring only); implicit interface satisfaction at the `ui.New(b)` call; `os.Exit` skipping deferred calls (why errors are printed first); `go install` and where binaries land (`$(go env GOPATH)/bin`).

- [ ] **Step 1: Replace `main.go`**

```go
// durtle-tui is an unofficial third-party terminal client for WaniKani.
package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/ParkerSuzuki/durtle-tui/store"
	"github.com/ParkerSuzuki/durtle-tui/ui"
	"github.com/ParkerSuzuki/durtle-tui/wanikani"
)

func main() {
	dir, err := store.CacheDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "durtle-tui:", err)
		os.Exit(1)
	}
	b := &backend{dir: dir, base: wanikani.BaseURL}
	if _, err := tea.NewProgram(ui.New(b)).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "durtle-tui:", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Build and test everything**

Run: `gofmt -w . && go build ./... && go test ./... && go vet ./...`
Expected: all PASS, no output from vet.

- [ ] **Step 3: Manual end-to-end run against the real account** (use the `driving-tuis-with-tmux` skill to observe; the user pastes the token themselves)

1. `go run .` in kitty. Expect the onboarding screen.
2. User pastes a token with `reviews:create`. Expect "Syncing with WaniKani...", then the review screen or "Session complete" with 0 reviewed if nothing is due.
3. With reviews due: answer one item wrong once, then right. Confirm on wanikani.com that the review count dropped by one.
4. Quit and relaunch: expect no onboarding (token remembered) and a fast sync (incremental).
5. Check `ls -l ~/.cache/durtle-tui/` shows `subjects.json` and `study_materials.json`.

Record anything surprising in `TODO.md` instead of fixing it on the spot, unless it loses data.

- [ ] **Step 4: Update README.md** with an "Install" section:

````markdown
## Install

Requires Go 1.27 or newer.

```bash
go install github.com/ParkerSuzuki/durtle-tui@latest
durtle-tui
```

On first run, paste a WaniKani personal access token with the
`reviews:create` permission. It is stored in your OS keyring.
````

and change the Status line to: `Status: milestone 1 (reviews) works. Lessons, dashboard, and audio are next; see [TODO.md](TODO.md).`

- [ ] **Step 5: Commit and push**

```bash
git add main.go README.md TODO.md
git commit -m "Wire up durtle-tui: milestone 1 complete"
git push
```

- [ ] **Step 6: Ask the user** whether to put `$(go env GOPATH)/bin` on their PATH (that means editing a shell config tracked in dotfiles, so it is their call).
