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
