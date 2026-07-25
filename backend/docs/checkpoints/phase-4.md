# Phase 4 checkpoint — Turn engine, evaluation, span anchoring

**Status: complete.**

> **Exit criterion:** an excellent answer produces zero red spans. ✅ Met,
> locally and on the deployed service.

Deployed, over `wss://`, a strong spoken-style answer produced:
`validated, validated, validated` — scores 10/10/9/8, turn score **9.35**,
**zero spans dropped**, turn persisted with cost attribution.

## PRD §5.1 acceptance tests — all three pass

Run with `go test ./internal/evaluator/ -tags=integration`:

| Test | Verdicts | Score | Latency |
|---|---|---|---|
| Excellent answer | validated ×4 (conf 0.95–1.00) | 9.60 | 5.4 s |
| Vague answer | incomplete, unsupported ×2 | 3.55 | 6.9 s |
| Fabricated claims | unsupported | 1.60 | 3.5 s |

**No false reds anywhere.** Not one `incorrect` verdict across any test,
including the answer made entirely of unbacked numbers. The four-verdict
taxonomy is doing exactly what PRD §12.1 designed it for: unbacked assertion is
`unsupported`, not `incorrect`.

**Anchor drop rate: 0%** across all three transcripts. The verbatim-quoting
instruction is holding, so no highlight is being silently lost.

## What exists now

| Component | Purpose |
|---|---|
| `internal/anchor` | Four-tier span resolver: exact → normalised → fuzzy → drop. 12 unit tests. |
| `internal/evaluator` | Schema, prompt, validation, AD-4 confidence gating. 11 unit + 4 live tests. |
| `internal/turn` | Transport-agnostic turn buffer and boundary snapshot. |
| `internal/live/turnflow.go` | Capture and close, wired into the relay. |
| `internal/grading` | The `TurnSink` and the evaluation worker handler. |
| `internal/live/registry.go` | Session registry so background work can push frames into a live socket. |

`go vet` clean, **60 unit tests passing**, plus 4 live integration tests.

## Latency: honest numbers

The first working implementation took **17.3 s per evaluation** against the
PRD's 4 s budget. Root cause was **thinking tokens**, not output size — measured
by holding the prompt constant and varying `thinkingBudget`:

| Thinking budget | Latency |
|---|---|
| model default (unbounded) | 7.4 s (short prompt) / 17.3 s (full prompt) |
| 512 | 3.5 – 6.9 s |
| 0 | 4.5 s |

Settled on **512** as `EVAL_THINKING_BUDGET`, tunable by env var. Span-level
judgement genuinely benefits from reasoning, so this is a dial rather than a
switch.

**It still exceeds the 4 s budget at the median (~5.4 s).** That is acceptable
rather than ideal, because AD-3's injection deadline means the *conversation*
never waits on the grader — the heatmap simply reveals a beat later. If it needs
to come down further the lever is trimming what the prompt asks for in the
output, not changing models.

## Bugs found and fixed

### 1. ⚠️ Structured calls bypassed retry AND the usage ledger

`evaluator` and `ingest` both called `vx.RawText().Models.GenerateContent(...)`
directly, which skips `withRetry`. A live 429 during testing turned into a
**failed evaluation** rather than a retried one.

This is the **second time** this exact mistake has been made: Phase 2 lost all
Live token accounting the same way, by reaching for `RawLive()` instead of the
wrapper. The pattern is now named explicitly in the code — a
`GenerateStructured` helper does retry plus accounting, and reaching for
`RawText()` is documented as the mistake it is.

Worth internalising: **a `Raw*()` accessor is an escape hatch, and every use of
one silently opts out of everything the wrapper exists to provide.**

### 2. A test-only chdir made three tests silently skip

`liveEvaluator` called `os.Chdir("../..")` per test. The first call worked; every
subsequent one moved again relative to the already-moved directory, so the key
file no longer resolved and those tests **skipped rather than failed**. A skip
reads as a pass at a glance, which is the dangerous part. Moved to `TestMain`
with a shared, once-initialised client.

## Design notes

- **Anchoring drops rather than guesses.** The most important test in
  `internal/anchor` asserts that a *paraphrase* is dropped. Anchoring a
  paraphrase onto unrelated text attaches a verdict to words that never made the
  claim — the user sees "incorrect" highlighted over a sentence that was never
  wrong. A missing highlight is invisible; a misplaced one is a visible bug.
- **The transcript's wording wins.** Spans store `m.Text` — the transcript's own
  characters at the resolved offsets — not the model's quotation of them, so the
  UI highlights a real range.
- **A dropped span's concept is kept**, but only when the verdict was not
  `validated`. Losing the concept would cost the roadmap a real gap; adding a
  validated one would tell the candidate to study something they got right.
- **Hint penalties are applied after grading**, never shown to the model. The
  grader must not know how much help the candidate took.
- **The evaluation job is idempotent.** A retry against an already-complete turn
  returns immediately rather than re-grading and re-charging.
- **Turn closure runs on its own goroutine with a detached context.** Ending a
  session cancels the request context instantly, and the final turn is precisely
  the one most worth grading.
- **Empty turns are never persisted.** Clicking Done without speaking would
  otherwise put a blank row in the report.

## Notes for Phase 5

- `Evaluation.FollowupProbe` and `.DifficultyRecommendation` are populated and
  unused. They are the two inputs the injection loop needs.
- `persona.Weights.Score` already produces the persona-weighted turn score, and
  `Evaluation.TurnScore` carries it with the hint penalty applied — the
  difficulty ladder can consume it directly.
- `Relay.Publish` is the mechanism for pushing a band change to the client, and
  is already proven by the evaluation frame.
- The session's `Coverage` sets are modelled but never updated; Phase 5 owns
  populating them from `concepts_demonstrated` and `concepts_missing`.
- Injection needs a new upstream message kind alongside `upText`, carrying the
  bracketed coach-state directive. `beginDirective` in `protocol.go` is the
  pattern to copy — including the do-not-read-aloud marker, which is working:
  no test has yet seen the interviewer narrate a system directive.
