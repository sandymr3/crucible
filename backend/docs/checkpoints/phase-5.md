# Phase 5 checkpoint — Adaptive difficulty and the injection loop

**Status: complete. This is the cut line — everything above it now ships.**

> **Exit criteria:** the band visibly moves, the next question is measurably
> sharper, and the coach state is never read aloud. ✅ All three met.

Verified end to end, client-observed:

```
← [BAND CHANGE] BAND 3 -> 4  (Difficulty raised — you've proven the fundamentals.)
    server: rolling=9.36  from=3 to=4
← [hint -0.5] When you scale to hundreds of sources, how do you map those
              topics to your consumer groups without hitting partition limits?
```

## What exists now

| Component | Purpose |
|---|---|
| `internal/difficulty` | Pure band ladder + coverage sets. **18 unit tests.** |
| `internal/grading/adapt.go` | Folds a grade into band and coverage, persists, emits the band frame. |
| `internal/grading/injection.go` | AD-3's deadline race between the grader and a deterministic fallback. |
| `internal/prompts/assets/injection_state.md` | The bracketed coach-state directive. |
| `internal/prompts/assets/hint_socratic.md` | Socratic hints that never give the answer. |
| `Relay.InjectCoachState` / `SetHintProvider` | The two new relay seams. |

`go vet` clean, **87 unit tests passing**.

## The bug that mattered most

**The adaptive engine was completely inert in production, and all 16 unit tests
passed anyway.**

`adapt()` rebuilt `difficulty.State` from Firestore each turn but carried only
band, coverage, and turn index — **not the streak counters or the previous
score**. So `StrongStreak` reset to zero on every turn and could never reach the
two consecutive turns a promotion requires. The rolling average never had a
prior value either.

The unit tests could not see it because they keep a single `State` in memory
across calls. Production does not: each turn is graded by a worker that
reconstructs state from Firestore, possibly on a different instance.

Fixed by persisting an `AdaptState` on the session document, and — more
usefully — by adding two tests that model the persistence boundary explicitly:

- `TestAdaptationSurvivesThePersistenceBoundary` round-trips `State` through
  only the fields actually stored, and asserts a promotion still happens.
- `TestPartialRoundTripBreaksAdaptation` documents that dropping the streak
  fields *must* leave the band stuck, so the guard above cannot rot into a
  tautology.

The lesson generalises: **a pure component with a persistence boundary needs a
test that crosses that boundary.** Testing the pure part proves the algorithm,
not the system.

## The injection deadline was sized wrong

The PRD's 3.5 s figure assumes a ~3 s grader. Measured evaluation latency here
is 6–7 s, so **the fallback won every single race** and the grader's
`followup_probe` — the sharpest question available, aimed exactly where the
answer thinned out — never reached the conversation at all. AD-3's entire point
was being silently defeated.

The reframing that fixed it: **the deadline is not a silence budget.** The
interviewer is already acknowledging the answer during that window; the deadline
governs how quickly we can steer its *next* question. So it must exceed real
evaluation latency. Raised to **9 s**, after which the fallback still bounds a
genuinely hung call.

Confirmed by the logs: no more `injection deadline fired`, and the injected
question length now varies per turn (117/125/134/142/156 chars) instead of being
a constant — the signature of the grader's probe rather than the canned
fallback.

## R9 — the interviewer never reads the directive aloud

Grepped **every AI transcript captured across all five phases** for `COACH
STATE`, `SESSION START`, `do not read`, `system directive`, `difficulty band is
now`, and `concepts already proven`.

**Clean.** Not one leak. The bracketed do-not-read-aloud convention is holding
across the begin directive, the coach state, and the band-change note.

## Design notes

- **Hints are delivered as text, never injected into the model's context.**
  Injecting one would have the interviewer read the hint aloud, which both gives
  the game away and violates the persona's "never supply the answer" rule.
- **Hint penalties are applied after grading.** The grader never learns how much
  help the candidate took, so it cannot be influenced by it.
- **The injection travels the same ordered upstream queue as audio**, so a coach
  state can never overtake the tail of an answer still being transmitted.
- **An ungraded turn moves nothing.** A grader outage is not evidence about the
  candidate, in either direction.
- **Proving a concept clears it from shaky and missing.** Otherwise the roadmap
  tells the candidate to study something they have since demonstrated.
- **Band 1 is unreachable and the cooldown is two turns.** Both are anti-demo
  rules as much as pedagogical ones: demoting an adult to definitional questions
  is demoralising, and a band oscillating every turn reads as confusion.

## Honest caveats

- **Scripted multi-turn verification is unreliable, by design.** Because the
  injection steers the interviewer to a new question each turn, canned answers
  drift out of alignment and score progressively worse. That is the adaptation
  working. The band-change path is therefore verified with a seeded prior turn
  plus one strong answer — deterministic, and exercising the identical code
  path. Genuine multi-turn promotion needs a human answering what is actually
  asked, which is what the demo rehearsal is for.
- **Evaluation latency remains 5–8 s with one observed 17.6 s outlier.** The
  conversation never waits on it, but the heatmap reveal is slower than the
  PRD's 4 s target.
- **The reconnect path (session resumption) is not built.** `SessionResumption`
  is enabled on the Live connect so the server emits handles, but nothing
  consumes them yet. A dropped socket currently ends the session rather than
  silently reconnecting. Deferred to Phase 9 hardening.

## Notes for Phase 6

- `Turn.AudioGCSURI` is populated and unused — it is the input to delivery
  metrics. **Read PRD §13.1 before building that**: Google's speech recognition
  normalises disfluencies out of the transcript, so a filler counter built on
  the transcript reads zero forever. It must analyse the audio.
- `Session.BandHistory` is populated and is the report's sparkline source, with
  no aggregation query needed.
- `Coverage.Missing` is accumulating well (8–10 concepts over three turns) and
  is the roadmap's input.
- The worker pool has `JobFinalize` and `JobBuildRoadmap` declared but
  unregistered; Phase 6 and 7 register them.
- `POST /v1/sessions/{id}/end` currently just marks the session complete. Phase
  6 dispatches finalization from there.
