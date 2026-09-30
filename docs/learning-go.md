# Learning Go through durtle-tui

One section per build step. Each explains the Go ideas that step used and why
Go does it that way. Read alongside the code; file references are clickable
in most editors.

## 1. Modules, packages, and the toolchain

**Module vs package.** A *package* is one directory of `.go` files that all
start with the same `package name` line. A *module* is a tree of packages
versioned together, defined by `go.mod` at its root. durtle-tui is one module
with several packages (`review`, `wanikani`, `store`, `ui`, and `main`).

**Why the module path looks like a URL.** `go.mod` says
`module github.com/ParkerSuzuki/durtle-tui`. Go has no central package
registry like npm or PyPI. The import path *is* the location: when someone
runs `go install github.com/ParkerSuzuki/durtle-tui@latest`, Go fetches it
from that address (through a caching proxy, proxy.golang.org). Inside the
project, packages import each other by full path, for example
`github.com/ParkerSuzuki/durtle-tui/review`.

**`package main` and `func main`.** A package named `main` builds into a
program instead of a library, and `func main()` is where it starts. There is
no class wrapper and no `if __name__ == "__main__"`.

**The `go` command does everything.** There is no separate build tool,
test runner, or formatter to choose:

| Command | What it does |
|---|---|
| `go run .` | compile and run the package in this directory |
| `go build ./...` | compile every package (`./...` means "this dir and below") |
| `go test ./...` | run every `_test.go` file |
| `go vet ./...` | catch suspicious code that compiles (wrong `Printf` verbs, copied locks, unreachable code) |
| `gofmt -w .` | rewrite files into the one official format |

**Why Go has exactly one format.** `gofmt` has no options. Tabs, brace
placement, and alignment are fixed, so Go code looks the same in every
project and nobody argues about style in code review. That is why this
project runs `gofmt -w .` instead of checking style by hand.

**`go 1.27.0` in go.mod** records the minimum Go version the code needs.
Newer toolchains keep compiling old code: Go promises backward compatibility
for the whole 1.x line.

## 2. Strings, bytes, runes, and table-driven tests

Code: [review/kana.go](../review/kana.go), [review/kana_test.go](../review/kana_test.go)

**A Go string is a read-only slice of bytes, usually UTF-8.** `len("かな")` is
6, not 2, because each kana takes 3 bytes. Indexing `s[i]` gives a *byte*
(`uint8`), not a character. That is why `ToHiragana` can look at
`rest[0] == 'n'` cheaply (romaji is ASCII, one byte per letter) but must use
`utf8.DecodeRuneInString` when it copies anything else through: slicing a
kana in the middle would produce garbage.

**A rune is a Unicode code point** (`int32` under the hood). Character
literals in single quotes, like `'ァ'`, are runes. Because runes are numbers,
`KatakanaToHiragana` converts with arithmetic: every katakana sits exactly
`'ァ' - 'ぁ'` (0x60) code points above its hiragana twin. `for _, r := range s`
over a string walks runes, not bytes, which `TestLiveTyping` relies on.

**`strings.Builder`** collects output without copying the whole string on
every append (strings are immutable, so `s += x` in a loop allocates each
time).

**Map literals.** `romaji` is a `map[string]string` written inline. Package
level `var` declarations like it are initialized once, before `main` runs.

**`strings.Map` and `strings.ContainsFunc`** take a function value. Go has
first-class functions and closures, used here instead of writing loops.

**The built-in `min`** (Go 1.21+) replaced the old habit of writing your own
`min` helper for every project.

**Table-driven tests** are the Go testing idiom: a slice of anonymous structs
(`[]struct{ in string; final bool; want string }`), one loop, one assertion.
Adding a case is one line. `t.Run(name, ...)` makes each row a named
*subtest*, so a failure prints `TestToHiragana/onna` and you can rerun just
that row with `go test -run 'TestToHiragana/onna'`.

**Tests live next to the code.** `kana_test.go` sits in the same directory
and declares `package review`, so it can call unexported (lowercase) names
like `containsKana`. Names starting with a capital letter are exported
(visible to other packages); lowercase names are private to the package.
That one rule replaces `public`/`private` keywords.

**`t.Errorf` vs `t.Fatalf`.** `Errorf` records a failure and keeps going, so
one run reports every broken row. `Fatalf` stops the test, for when later
checks would be meaningless.

## 3. Named types, enums with iota, and methods

Code: [review/item.go](../review/item.go), [review/grade.go](../review/grade.go)

**Named types.** `type Part int` makes a new type whose underlying
representation is `int`, but the compiler treats it as distinct: you cannot
pass a plain `int` or a `Verdict` where a `Part` is expected without an
explicit conversion. That is cheap type safety.

**Go has no `enum` keyword; `iota` fills the gap.** Inside a `const (...)`
block, `iota` starts at 0 and increases by one per line, and lines without
an expression repeat the previous one. So `Wrong Verdict = iota` followed by
bare `Correct`, `CorrectTypo`, `Warn` gives 0, 1, 2, 3, all typed `Verdict`.
Putting `Wrong` first means the zero value (what an uninitialized `Verdict`
holds) is the safe one.

**Methods attach to any named type,** not just structs:
`func (p Part) String() string`. The `(p Part)` part is the *receiver*.
Because `Part` now has a `String() string` method, it satisfies the standard
library's `fmt.Stringer` interface, so `fmt.Printf("%v", p)` prints
"reading" instead of 1. Nothing declares that `Part` implements `Stringer`:
Go interfaces are satisfied implicitly, by having the right methods.

**`[...]string{...}[v]`** is an array literal whose length the compiler
counts, indexed immediately. A compact way to map a small enum to names.

**Struct literals: positional vs named.** `Grade{Warn, "hint"}` fills fields
in order; `Grade{Verdict: Wrong}` names them and leaves the rest at their
zero value (`""` for `Hint`). Named is safer when a struct might grow; `go
vet` insists on it for structs from other packages.

**Zero values mean "no constructor needed".** An `Item` with no `Readings`
has a `nil` slice, and `len(nil)` is 0, so `HasReading` just works. Go
avoids null checks by giving every type a usable zero value.

**Slices of slices.** `osaDistance` builds its dynamic programming grid
with `make([][]int, rows)` and then one `make([]int, cols)` per row. Go has
no built-in 2D array with runtime sizes; a slice of slices is the idiom.

**`[]rune(a)`** converts a string to its code points so the edit distance
counts characters, not UTF-8 bytes. (Matters for accented synonyms.)

**`switch` with no value** (`switch { case n <= 3: ...}`) is Go's clean
if/else-if chain. Cases do not fall through, so no `break` needed.

**Why `review` has its own `Item` type** instead of using the API's JSON
structs: `review` should not know HTTP or JSON exists. It depends on nothing,
so it can be tested alone, and the API shape can change without touching
the grading code. Package `main` translates between the two (Task 7).

## 4. Pointer receivers, optional values, and deterministic randomness

Code: [review/session.go](../review/session.go), [review/session_test.go](../review/session_test.go)

**Value vs pointer receivers.** `Item.HasReading` uses a value receiver
`(it Item)`: it gets a copy and only reads. `Session` methods use
`(s *Session)` because they *change* the session (advance `pos`, bump
`wrong`). With a value receiver those changes would land on a copy and
vanish. Rule of thumb: if any method needs a pointer, give them all
pointers, so the type behaves consistently.

**Constructors are just functions.** Go has no `new ClassName()`. By
convention `NewSession(...)` builds and returns a `*Session`. `&Session{...}`
takes the address of a fresh struct literal; Go's escape analysis puts it on
the heap automatically because it outlives the function. There is no manual
memory management and no difference in syntax.

**`*Submission` as an optional value.** `Answer` returns `(Grade,
*Submission)`: a nil pointer means "item not finished yet", non-nil means
"send this". Returning `&sub` of a local variable is safe in Go (see above).
The other common idiom is a third `ok bool` result, which `Current` uses.

**Multiple return values** replace out-parameters and tuples:
`it, part, ok := s.Current()`. `Current` also uses *named results*
(`(it Item, p Part, ok bool)`), which document what each value means.

**Arrays vs slices.** `wrong [2]int` is a fixed-size array (a value; copying
the struct copies it), indexed by `Part` because `Meaning` is 0 and
`Reading` is 1. Slices (`[]Part`) are growable views onto arrays.
`s.parts = s.parts[1:]` "pops" the front by re-slicing, with no copying.

**`slices.Clone`** (standard library `slices` package, Go 1.21+) copies the
caller's slice before shuffling. A slice shares its backing array, so
shuffling `items` directly would reorder the caller's data behind their back.

**`math/rand/v2` and injected randomness.** `NewSession` takes a
`*rand.Rand` instead of calling a global random function. Production passes
a randomly seeded one; tests pass `rand.New(rand.NewPCG(seed, seed))`, so the
"random" order is identical on every run and a failing test can be
reproduced. This is dependency injection with no framework: just a
parameter.

**`for seed := range uint64(40)`** ranges over an integer (Go 1.22+):
0 through 39, typed `uint64` to match what `NewPCG` wants.

**`%+v` in test messages** prints struct field names
(`{AssignmentID:1 IncorrectMeaning:1 IncorrectReading:0}`), which makes
failures readable. Structs with only comparable fields can be compared
with `==`, as `*sub != want` does.

## 5. JSON, generics, context, and errors

Code: [wanikani/types.go](../wanikani/types.go), [wanikani/client.go](../wanikani/client.go), [wanikani/client_test.go](../wanikani/client_test.go)

**Struct tags drive `encoding/json`.** `` `json:"subject_id"` `` after a field
tells the decoder which JSON key maps to it. Only *exported* (capitalized)
fields are visible to the `json` package, which is why the fields are
`SubjectID`, not `subjectID`. Keys in the JSON that have no matching field
are silently ignored, so these structs list only what durtle-tui uses.

**`*string` for JSON `null`.** A plain `string` cannot tell "missing or
null" from "empty". `Characters *string` is `nil` when the API sends `null`
(image-only radicals), and callers must check before dereferencing.

**Generics (Go 1.18+).** Every WaniKani object has the same envelope (`id`,
`object`, `data_updated_at`, `data`), with a different `data` shape.
`Resource[T any]` writes the envelope once; `Resource[Subject]` and
`Resource[Assignment]` are the concrete types. Before generics you would
copy the envelope into every struct or decode `data` in two passes.

**`context.Context` is the first parameter of anything that can block.** It
carries cancellation and deadlines. `http.NewRequestWithContext(ctx, ...)`
aborts the request if the context is cancelled, and `waitUntil` uses
`select` to wait on *either* the rate-limit timer *or* `ctx.Done()`,
whichever fires first. Later the UI gives each network call a timeout this
way.

**`select`** waits on several channel operations at once and runs the first
one that is ready. `timer.C` and `ctx.Done()` are both channels.

**Errors are values, returned, not thrown.** Every call that can fail
returns an `error` as its last result, and the caller checks
`if err != nil`. Verbose, but every failure path is visible in the code.

**Two kinds of errors here:**
- A *sentinel*: `var ErrUnauthorized = errors.New(...)`. Callers test it
  with `errors.Is(err, wanikani.ErrUnauthorized)`.
- A *custom error type*: `*APIError` carries the status code. Any type with
  an `Error() string` method is an `error` (implicit interfaces again).
  Callers extract it with `errors.As(err, &apiErr)`, which fills `apiErr`
  if the error, or anything it wraps, is an `*APIError`.

**Wrapping with `%w`.** `fmt.Errorf("decoding %s: %w", url, err)` adds
context but keeps the original error inside, so `errors.Is` and `errors.As`
still see it. `%v` would flatten it to text.

**`defer`** schedules a call for when the surrounding *function* returns.
`defer resp.Body.Close()` in `decode` guarantees the connection is released
on every return path. It is not used inside the retry loop in `do`: defers
pile up until the function exits, so a loop would hold every retried body
open. The loop closes the 429 body explicitly instead.

**`any`** is an alias for `interface{}`, the empty interface every type
satisfies. `do` takes `body, out any` so one function serves every endpoint;
`json.Marshal` and `Decode` work out the real type at runtime.

**Testing HTTP with `httptest.NewServer`.** Each test starts a real local
server with a handler function, and the client is pointed at it through
`NewClient(srv.URL+"/", ...)`. Passing the base URL into the constructor is
what makes this possible: no mocking library, no global variables.
`t.Helper()` makes failures report the caller's line, and `t.Cleanup` shuts
the server down when the test ends.

## 6. Generic functions and paging

Code: [wanikani/endpoints.go](../wanikani/endpoints.go), [wanikani/endpoints_test.go](../wanikani/endpoints_test.go)

**Why `getAll` is a function, not a method.** Go methods cannot declare
their own type parameters: `func (c *Client) getAll[T any](...)` does not
compile. Only the *type* can be generic (`Resource[T]`), and its methods
share that `T`. So the generic helper is a plain function that takes the
client as an argument: `getAll[Subject](ctx, c, "subjects")`. The public
methods (`Subjects`, `StudyMaterials`, ...) are one-line wrappers that pick
the type, so callers never see the generics.

**Explicit type arguments.** `getAll[Subject](...)` names `T` because the
compiler cannot infer it from the arguments (none of them mention `T`).
When an argument does carry the type, Go infers it and you can leave the
brackets off.

**A `for` loop with an empty post statement.**
`for next := c.base + path; next != ""; { ... }` declares `next`, loops
while it is non-empty, and updates it inside the body. Go has only one loop
keyword, `for`, which covers while-loops, infinite loops, and ranges too.

**`append(all, p.Data...)`** appends every element of one slice to another;
the `...` spreads the slice into individual arguments.

**`time.Time` zero value as "not set".** `Subjects(ctx, time.Time{})` asks
for everything, because `t.IsZero()` is true for a `time.Time` nobody
filled in. No pointer or sentinel needed.

**`url.QueryEscape`** makes the timestamp safe in a query string (a `+` in a
timezone offset would otherwise read as a space). Times are formatted as
RFC 3339 in UTC, which is what the API expects.

**Nested map literals for one-off JSON.** `SubmitReview` builds
`map[string]any{"review": map[string]int{...}}` instead of declaring two
structs for a body used once. For shapes that are reused or decoded, use a
struct; for a fire-and-forget body, a map is fine.

**Anonymous structs in tests.** `TestSubmitReviewBody` decodes into
`var body struct{ Review map[string]int ... }`, a type declared inline
because nothing else needs it.

## 7. Dependencies, files on disk, and concurrency safety

Code: [store/store.go](../store/store.go), [backend.go](../backend.go), [backend_test.go](../backend_test.go)

**Adding a dependency.** `go get github.com/zalando/go-keyring@v0.2.8`
downloads the module, records it in `go.mod` (what we need) and `go.sum`
(cryptographic hashes of exactly what was downloaded). `go mod tidy` then
removes anything unused and adds anything missing. Both files are
committed: `go.sum` means everyone who builds durtle-tui gets byte-identical
dependencies, or the build fails. There is no lockfile tool to choose and
no `node_modules`; downloads live in a shared cache (`go env GOMODCACHE`).

**Cross-platform paths from the standard library.** `os.UserConfigDir()`
is `~/.config` on Linux, `~/Library/Application Support` on macOS, and
`%AppData%` on Windows. `os.UserCacheDir()` is the same idea for caches.
`filepath.Join` uses the right separator for the OS.

**Octal file modes.** `0o600` (owner read/write, nobody else) and `0o700`
(owner only, for directories). The `0o` prefix is Go's explicit octal
literal.

**Atomic writes.** `WriteJSON` writes `file.tmp` and then `os.Rename`s it
over `file`. On the same filesystem, rename is atomic: other readers see
either the old file or the new one, never half of each, even if the app
crashes mid-write. Cheap insurance for `pending.json`, which holds answers
we promised not to lose.

**`errors.Is(err, fs.ErrNotExist)`** is how Go checks "file not found"
portably. The OS-specific error is wrapped inside; `errors.Is` digs for it.

**`sync.Mutex` and why it is needed here.** Bubble Tea runs every command on
its own goroutine (lightweight thread). If you answer two items quickly and
both submits fail, two goroutines would read-modify-write `pending.json` at
the same time and one answer could be lost. `b.mu.Lock()` makes the second
wait for the first. `defer b.mu.Unlock()` right after `Lock` is the idiom:
the unlock can never be forgotten on an early return. The mutex is a field
used by value inside a struct that is always handled through a pointer
(`*backend`); copying a struct that contains a mutex is a bug, and `go vet`
flags it.

**`errors.As(err, &apiErr)`** takes a pointer to a variable of the target
type (here `**wanikani.APIError`, because the error type is itself a
pointer). If anything in the error chain matches, it fills the variable and
returns true. `rejected` uses it to read the status code.

**Named result parameters** in `Submit(...) (pending bool, err error)`
document what the two return values mean at the signature.

**Maps with int keys in JSON.** `map[int][]string` encodes as a JSON object
with string keys (`{"440": ["huge"]}`) and decodes back to ints. JSON only
allows string keys; `encoding/json` converts automatically.

**Prepending to a slice.** `append([]string{m.Meaning}, it.Meanings...)`
builds a new slice with the primary meaning first. Go has no `unshift`;
this is the idiom (fine for a handful of elements).

**Test helpers that set up the world.** `t.TempDir()` gives each test a
fresh directory that is deleted afterwards. `t.Setenv("XDG_CONFIG_HOME",
...)` redirects `os.UserConfigDir()` for the duration of one test, so the
token tests never touch your real config. `keyring.MockInit()` swaps the
real keyring for an in-memory one, a test hook the library provides.

**Package `main` can have tests too.** `backend_test.go` is `package main`
and tests unexported functions like `buildItems` directly.

## 8. Bubble Tea: the Elm architecture in Go

Code: [ui/model.go](../ui/model.go), [ui/view.go](../ui/view.go), [ui/model_test.go](../ui/model_test.go)

**Three methods run the whole UI.** A Bubble Tea program is any type with:
- `Init() tea.Cmd`: work to start immediately (here: load reviews).
- `Update(msg tea.Msg) (tea.Model, tea.Cmd)`: given an event (a key press,
  a window resize, a finished HTTP call), return the new state and optionally
  more work to do.
- `View() tea.View`: turn the current state into what the screen shows.

The framework loops: event in, `Update`, `View`, repeat. All state lives in
`Model`; nothing else changes the screen. This is "The Elm Architecture",
and it makes the UI easy to test: `model_test.go` feeds messages to
`Update` and inspects the returned model, with no terminal involved.

**Value receivers returning a new model.** `func (m Model) Update(...)`
receives a *copy* of the model, changes the copy, and returns it. The
framework keeps whatever you return. That is why every branch ends in
`return m, cmd`, and why the tests write `m, cmd = step(t, m, msg)`.
(`Session` is a pointer inside the model, so it is shared between copies on
purpose: there is one review session.)

**Commands are how slow work stays off the UI.** A `tea.Cmd` is just
`func() tea.Msg`. Bubble Tea runs it on its own goroutine and delivers the
returned message to `Update` when it finishes. `m.submit(sub)` returns a
closure that calls the backend and returns `submittedMsg`. The UI keeps
responding to keys while the HTTP request is in flight, with no callbacks,
promises, or `async` keyword: goroutines plus a message.

**Closures capture variables.** Inside `submit`, the returned function uses
`s` and `m.backend` from the enclosing call. Go closures capture variables,
so each command carries its own submission.

**Type switches.** `switch msg := msg.(type) { case tea.KeyPressMsg: ... }`
branches on the dynamic type stored in the `tea.Msg` interface, and inside
each case `msg` already has the concrete type. Our own message types
(`loadedMsg`, `submittedMsg`) are small unexported structs declared in one
`type (...)` block.

**Implicit interfaces, the big payoff.** `ui` declares
`type Backend interface { Login; Load; Submit }` and never mentions
`backend` from package `main`. `*backend` satisfies it simply by having
those three methods, so `main` can pass it in. The test's `fakeBackend`
satisfies it the same way. The package that *uses* the behavior defines the
interface, sized to what it needs: "accept interfaces, return structs".

**Context with timeouts per command.** Each command makes
`context.WithTimeout(context.Background(), ...)` and `defer cancel()`, so a
hung network call cannot freeze a submit forever. `cancel` must always be
called to release the timer; `defer` guarantees it.

**Import aliases.** `tea "charm.land/bubbletea/v2"` names the package `tea`
in this file. The `/v2` suffix is Go's rule for major versions: a breaking
v2 must have a different import path, so v1 and v2 can even coexist in one
build.

**`go mod tidy` after `go get`.** `go get` records the modules you asked
for; `tidy` also adds checksums for everything *their* packages import
(here, a clipboard library the text input uses). Run it whenever a build
says "missing go.sum entry".

**Lip Gloss styles are values.** `lipgloss.NewStyle().Bold(true).Padding(0, 2)`
chains methods that each return a modified copy, so package-level styles
like `meaningBar` can be shared safely. `style.Render(s)` returns a string
with ANSI escape codes baked in.

## 9. Wiring it together, and a real bug

Code: [main.go](../main.go)

**`main` stays thin.** It finds the cache directory, builds a `*backend`,
and hands it to `ui.New`. Every decision lives in a package that can be
tested; `main` only connects them. The line `ui.New(b)` is where implicit
interfaces pay off: the compiler checks right there that `*backend` has
every method `ui.Backend` requires, and would refuse to build if one were
missing.

**`os.Exit` skips deferred calls.** That is why errors are printed with
`fmt.Fprintln(os.Stderr, ...)` *before* `os.Exit(1)`, and why nothing
important is deferred in `main`.

**`go install`** builds the binary into `$(go env GOPATH)/bin` (usually
`~/go/bin`). `go install github.com/ParkerSuzuki/durtle-tui@latest` does the
same straight from GitHub, which is the whole install story for Go tools.

**The race detector.** `go test -race ./...` rebuilds everything with
instrumentation that reports two goroutines touching the same memory
without synchronization. It found nothing here, which backs up the
`sync.Mutex` around `pending.json`.

**A bug the unit tests missed.** Running the real binary showed the
onboarding placeholder as just `p`. Reading the library source
(`~/go/pkg/mod/charm.land/bubbles/v2@v2.2.1/textinput/textinput.go`,
`placeholderView`) showed why: it copies the placeholder into a buffer sized
by the input's width, and the width defaults to 0. The fix is one line,
`in.SetWidth(inputWidth)`, pinned by `TestOnboardingShowsFullPlaceholder`.
Two lessons: the module cache holds the exact source of every dependency,
so reading it is often the fastest way to understand a library; and a
screen you have never looked at is a screen you have not tested.

## 10. Wrapping a method, raw terminal output, and environment checks

Code: [ui/model.go](../ui/model.go) (`Update`, `update`), [ui/view.go](../ui/view.go) (`bigCharsSeq`), [main.go](../main.go) (`bigTextSupported`)

**Wrapping instead of editing every branch.** Big characters must be redrawn
after *every* update. Rather than adding a line to each `case`, the old
`Update` was renamed to lowercase `update`, and a new exported `Update`
calls it and adds one extra command. Lowercase means unexported: only the
wrapper is part of the `tea.Model` contract. Its one type assertion,
`next.(Model)`, turns the returned `tea.Model` interface back into the
concrete type to read its fields.

**`tea.Batch` and `tea.Tick`.** `Batch` runs several commands concurrently
and delivers each message as it arrives; `Tick(d, fn)` waits `d` and then
sends `fn`'s message. Together they say "also, 40 ms from now, send me
`drawBigMsg`". A `nil` command inside `Batch` is fine, which is why the
wrapper does not need to check.

**Escape sequences are just strings.** `bigCharsSeq` builds the terminal
commands with `fmt.Fprintf` into a `strings.Builder`: `\x1b7` saves the
cursor, `\x1b[row;colH` moves it, `\x1b[48;2;r;g;bm` sets a 24-bit
background, `\x1b]66;s=3;訓練\x07` is kitty's "draw this at 3x", and
`\x1b8` restores the cursor. `fmt.Sscanf(hex, "#%02x%02x%02x", &r, &g, &b)`
parses the palette's hex colors back into numbers, reusing one source of
truth for the colors.

**Constants with arithmetic.** `charRow = 1 + 1 + 1 + blockPadding + 1` is
computed at compile time and documents *why* the row is 7. A test checks it
against the rendered view, so changing the layout without updating the
constant fails loudly instead of drawing the glyph in the wrong place.

**Feature detection through the environment.** `os.Getenv("KITTY_WINDOW_ID")`
is empty unless kitty launched the process. Detection lives in `main`, and
`ui.New` takes a plain `bool`, so tests never depend on which terminal runs
them.

**`ponytail:` comments** mark a deliberate shortcut with its limit and
upgrade path, so the next person (or a Bubble Tea upgrade) knows exactly
what to check.

## 11. Running other programs, images, and passing functions in

Code: [backend.go](../backend.go) (`radicalImage`, `download`, `buildItems`), [ui/view.go](../ui/view.go) (`halfBlocks`)

**`os/exec` runs another program.** `exec.CommandContext(ctx, "rsvg-convert",
"-w", "20", "-h", "20", path).Output()` starts the tool, waits, and returns
its stdout as `[]byte`. Arguments are passed as separate strings, never
through a shell, so a file name can never be misread as extra commands. If
the program is not installed, `Output` returns an error, which is how
durtle-tui notices and skips the radical instead of crashing. The `ctx`
kills the tool if the load is cancelled.

**The `image` package is an interface.** `image.Image` is any type with
`Bounds()`, `ColorModel()` and `At(x, y)`. `png.Decode` returns one;
the tests build tiny ones with `image.NewAlpha`. `halfBlocks` only calls
`At(...).RGBA()` and reads the alpha channel, so it works on any image
from any source, which is interfaces paying off again.

**`bytes.NewReader` adapts a `[]byte` into an `io.Reader`.** `png.Decode`
wants a reader (a stream), `Output` gives bytes (a buffer); the adapter
bridges them without copying. The `io.Reader` interface (one method,
`Read`) is the most reused abstraction in Go: files, network bodies,
buffers and decompressors all speak it.

**`io.LimitReader`** caps a download at 1 MB, so a misbehaving server
cannot fill memory. Defensive, and one line.

**Passing a function as a parameter.** `buildItems` needs a picture for
image-only radicals but should stay free of network and disk work, so it
takes `art func(wanikani.Resource[wanikani.Subject]) image.Image`. The real
backend passes a closure over `ctx` that downloads and rasterizes; the test
passes `func(...) image.Image { return pic }`. A one-function dependency does
not need an interface; a function type is lighter.

**Named results as documentation.** `buildItems(...) (items
[]review.Item, skipped int)` says what each returned value means, and
`items` starts as a nil slice that `append` grows.

**`if x = f(); x == nil`** assigns and tests in one statement. It is used as
`else if it.Image = art(s); it.Image == nil { skipped++; continue }`.

**Versioning a cache.** Adding a field to a struct does not update JSON
files written before it existed; the old files simply lack the key. A
`Version` constant stored in the file, compared on load, turns "silently
missing data forever" into "one full resync".

## 12. Generic functions that save code, and growing an interface

Code: [dashboard/dashboard.go](../dashboard/dashboard.go), [backend.go](../backend.go) (`syncResources`), [ui/view.go](../ui/view.go) (`homeView`)

**One generic function replaces two copies.** Subjects and assignments are
cached the same way: load a file, fetch what changed since the last sync,
merge by ID, save. `syncResources[T any](path, version, fetch)` does that
once. Callers never write `[wanikani.Subject]`: Go *infers* `T` from the
`fetch` argument's type, `func(time.Time) ([]wanikani.Resource[wanikani.Subject], error)`.
The on-disk shape is generic too: `resourceCache[T]`.

**Closures adapt a method to a function type.** `fetch` needs one argument
(`since`), but `b.client.Subjects` needs two (`ctx`, `since`). The wrapper
`func(since time.Time) (...) { return b.client.Subjects(ctx, since) }`
captures `ctx` and presents the one-argument shape. No adapter types.

**Growing an interface finds every implementation for you.** Adding
`Dashboard` to `ui.Backend` made the build fail in exactly two places: the
real `*backend` and the test's `fakeBackend`. That is the compiler doing
the "did I update everything?" check that a dynamic language leaves to you.

**A pure package as a seam.** `dashboard.Build` takes data and returns
numbers: no network, no disk, no screen. The tests hand it tiny maps and
check the result, including awkward cases (a summary entry exactly at
`now`, hidden subjects, lessons not started) in microseconds.

**Sorting with `slices.SortFunc`** (Go 1.21+) takes a comparison returning
negative, zero or positive. `time.Time.Compare` (Go 1.20+) returns exactly
that, so the sort is one line.

**Comparing structs and arrays with `==`.** `Progress` and `SRS` hold only
numbers, so tests compare whole values: `d.SRS != SRS{1, 21, 1, 0, 1}`.
Slices and maps are not comparable, which is why the forecast test loops.

**Integer ceiling division.** 90% of 33 kanji, rounded up, is
`(33*9 + 9) / 10 = 30` without touching floating point: adding
`denominator - 1` before dividing rounds up.

**Format verbs for columns.** `%-18s` left-aligns in 18 cells, `%5d`
right-aligns a number in 5, `%+5d` always shows the sign (`+12`).
`t.Local().Format("Mon 15:04")` uses Go's reference time (Mon Jan 2
15:04:05 2006) as the layout: you write the example date the way you want
it printed.

**A test that measures, not just reads.** The first real run showed a line
running off a 100-column screen, which the content test could not see.
`TestDashboardFitsWidth` now checks `lipgloss.Width` of every line, so a
layout change that overflows fails in CI instead of on your screen.

## 13. Turning special cases into data

Code: [ui/view.go](../ui/view.go) (`bigGlyph`, `bigGlyphs`, `bigCharsSeq`)

The big-text code started with one hard-coded case: the review kanji. Adding
the dashboard counts could have meant a second copy with different rows and
colors. Instead, each screen now *describes* what it wants drawn large as
a `[]bigGlyph` (row, column, width, scale, text, color), and one function
draws any list. Adding a third big thing later is one more entry, not one
more function.

**Describe, then act.** `bigGlyphs()` only computes positions (pure, easy
to test); `bigCharsSeq()` only turns them into escape codes. Tests check
the description and the sequence separately.

**`fmt.Sprintf("%v", glyphs)` as a change detector.** `%v` prints every
field of every struct in the slice, so the string changes exactly when
anything worth redrawing changes. It replaced a hand-built key that had to
list the fields that mattered, and could drift out of date.

**`lipgloss.JoinHorizontal`** places multi-line blocks side by side and pads
shorter ones to the same height, which is how the two tiles and the
two-space gap between them line up.
