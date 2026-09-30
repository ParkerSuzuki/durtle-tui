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
