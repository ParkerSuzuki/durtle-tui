# TODO

Deferred work. Each item gets its own design discussion before work starts.

Milestone 2: dashboard (in design).
Milestone 3 candidates: lessons, or review polish (audio, undo, wrap-up, the review-finding minors below).

- [ ] Sharper radical images in kitty via the graphics protocol (half-block art works everywhere; decision 20).
- [ ] Lessons: teaching screens plus `PUT /assignments/<id>/start`.
- [ ] Dashboard: level progress, upcoming review forecast, counts by SRS stage.
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
