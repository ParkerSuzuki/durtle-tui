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
