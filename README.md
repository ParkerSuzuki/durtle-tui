# durtle-tui

**durtle-tui is an unofficial third-party app for WaniKani.** It is not made by
or affiliated with WaniKani or Tofugu.

Do your WaniKani reviews from the terminal. Written in Go.

Status: milestones 1 (reviews) and 2 (dashboard) work. The dashboard shows
lessons and reviews available, a 24-hour forecast, level progress, and SRS
counts; reviews start from it. Reviews: sync, back-to-back reviews graded like the
website, submissions that are never lost, big kanji in kitty, and image radicals.
Lessons, dashboard, and audio come later; see [TODO.md](TODO.md).

## Install

Requires Go 1.27 or newer.

```bash
go install github.com/ParkerSuzuki/durtle-tui@latest
durtle-tui
```

On first run, paste a WaniKani personal access token with the
`reviews:create` permission. It is stored in your OS keyring.

Optional: install `rsvg-convert` (package `librsvg`, or `librsvg2-bin` on
Debian/Ubuntu) to review the radicals that have no Unicode character. Without
it they are skipped and left for the website.

## Docs

- [Design](docs/superpowers/specs/2026-09-29-durtle-design.md) for milestone 1
- [Decision log](docs/decisions.md): why things are the way they are
- [Learning Go](docs/learning-go.md): the Go ideas behind each part of the code

## License

MIT
