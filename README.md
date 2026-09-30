# durtle-tui

**durtle-tui is an unofficial third-party app for WaniKani.** It is not made by
or affiliated with WaniKani or Tofugu.

Do your WaniKani reviews from the terminal. Written in Go.

Status: milestone 1 (reviews) is built and being tested against a real account.
Lessons, dashboard, and audio come later; see [TODO.md](TODO.md).

## Install

Requires Go 1.27 or newer.

```bash
go install github.com/ParkerSuzuki/durtle-tui@latest
durtle-tui
```

On first run, paste a WaniKani personal access token with the
`reviews:create` permission. It is stored in your OS keyring.

## Docs

- [Design](docs/superpowers/specs/2026-09-29-durtle-design.md) for milestone 1
- [Decision log](docs/decisions.md): why things are the way they are
- [Learning Go](docs/learning-go.md): the Go ideas behind each part of the code

## License

MIT
