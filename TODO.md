# TODO

Deferred work. Each item gets its own design discussion before work starts.

Milestone 2: dashboard (done). Milestone 3: lessons (done).
Next candidate: review polish (audio, undo, wrap-up, the review-finding minors below).

- [ ] Sharper radical images in kitty via the graphics protocol (half-block art works everywhere; decision 20).
- [ ] Audio: play vocab pronunciation with `mpv` after a correct reading.
- [ ] Undo a typo'd answer with one key.
- [ ] Wrap-up mode: finish items already started, then stop.
- [ ] Status-bar module showing lessons and reviews due.

- [ ] Keep synced caches in memory between dashboard refreshes if they feel slow (reading subjects.json takes ~0.25 s; unchanged syncs no longer rewrite it).

From the cleanup review (deferred minors):
- [ ] The dashboard's resend of saved answers can race a submit still in flight; the refused copy may be counted as "refused" in the summary.
- [ ] A 401 on a lesson start during a pending quit goes to token entry instead of finishing the quit.
- [ ] Enter on the lesson summary also waits for unrelated review submits still in flight.
- [ ] flushPending holds the pending lock across network calls, so new submits wait behind a slow resend.
