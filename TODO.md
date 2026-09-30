# TODO

Deferred work. Each item gets its own design discussion before work starts.

Milestone 2: dashboard (done). Milestone 3: lessons (done).
Next candidate: review polish (audio, undo, wrap-up, the review-finding minors below).

- [ ] Sharper radical images in kitty via the graphics protocol (half-block art works everywhere; decision 20).
- [ ] Audio: play vocab pronunciation with `mpv` after a correct reading.
- [ ] Undo a typo'd answer with one key.
- [ ] Wrap-up mode: finish items already started, then stop.
- [ ] Status-bar module showing lessons and reviews due.

From the milestone 1 review (deferred minors):
- [ ] Missing reviews:create permission (403): answers now go to pending, but the app should say so and send the user back to onboarding.
- [ ] Slow links: drop the 30s http.Client timeout (per-call contexts already bound it) or save the subject cache page by page.
- [ ] External SIGINT/SIGTERM exits without waiting for in-flight submits; write to pending before sending and remove on success.
- [ ] A second Ctrl+C while "finishing submissions" should cancel them into pending instead of waiting up to 2 minutes.
- [ ] Use the collection's data_updated_at instead of the local clock for updated_after.
- [ ] flushPending: on a mid-run 401, drop the entries already sent before returning.
- [ ] Romaji gaps: shimbun (m before b/p/m), vu, dya/dyu, xtsu, wi.
- [ ] Spec wording: typo tolerance uses the accepted meaning's length, not the typed answer's.

From the milestone 2 review (deferred minors):
- [ ] A submit landing after the next session starts counts toward the new session's summary (cosmetic).
- [ ] Below about 60 columns the kanji progress line wraps; drop the counts suffix on narrow terminals.
- [ ] Incremental sync uses the local clock for updated_after (clock skew can skip updates); use the response's data_updated_at.

From the milestone 3 review (deferred minors):
- [ ] Settings with some fields hand-edited (e.g. only daily_cap) clamp batch size to 3 instead of seeding it from WaniKani; a corrupt settings.json is overwritten with defaults.
- [ ] Settings opened before the first dashboard load seed batch size 5, not the WaniKani value.
- [ ] If settings.json cannot be written, the whole dashboard fails to load.
- [ ] Enter on the lesson summary while starts are still in flight reloads the dashboard too early (stale count; `l` may re-teach the same items).
- [ ] A 401 on a lesson start counts as a failure instead of going to token entry.
- [ ] → on the last teaching page starts the quiz (only Enter should); space adds 1 on numeric settings rows; refusing to turn off the last type is silent.
- [ ] subjects.json is 14.9 MB after the version-4 resync (a full dashboard load takes about 3 s); consider trimming fields the dashboard does not need if loads feel slow.
