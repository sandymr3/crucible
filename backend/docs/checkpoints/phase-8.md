# Phase 8 checkpoint — Study Mode

**Status: complete.**

> **Exit criterion:** the PRD §2 compliance matrix has no gaps. ✅ **8 of 8.**

## The compliance matrix, closed

| Required capability | How it is satisfied |
|---|---|
| Takes a topic **or** job role | Interview: resume PDF + JD → digest. Study: topic → dependency-ordered syllabus |
| Generates relevant practice questions | Live persona conditioned on digest, probe angles, band, coverage. Study: four archetypes per subtopic |
| Evaluates **spoken** answers | Native-audio bidi; input transcription → the evaluator. Verified end to end |
| Evaluates **written** answers | `text_answer` frame and Study Mode share the identical path (AD-6) |
| Structured feedback: strengths | `concepts_demonstrated` → `coverage.proven` → radar + strengths list |
| Structured feedback: gaps | `concepts_missing` plus the four-verdict span heatmap, anchored server-side |
| Suggested resources | Grounded roadmap; every URL fetched and verified, 7/7 resolved |
| Adapts difficulty | Five-band ladder with rolling score. Verified live: `BAND 3 → 4` |

## What Study Mode actually added

Almost nothing, which was the point. AD-6 said a transport-agnostic turn engine
would make Study Mode "three uses of one code path rather than three
implementations", and this is where that claim was tested. Study Mode reuses the
evaluator, the span anchoring, the coverage sets, and the roadmap **unchanged**.

Two things are genuinely new:

**A real dependency graph.** Decomposing "Transformer attention" produced eight
subtopics with branching prerequisites, not a chain:

```
s1 Query, Key, Value representations          (no prereqs)
s2 Scaled Dot-Product Attention calculation   <- s1
s3 Why scale by 1/sqrt(d_k)                   <- s2
s4 Causal masking in decoder attention        <- s2
s5 Multi-Head Attention mechanics             <- s2
s7 Injecting Positional Encodings             <- s2
s6 Self-Attention vs Cross-Attention          <- s5
s8 Key-Value (KV) Caching in inference        <- s4
```

s2 fans out to four independent subtopics. That branching matters: it is real
prerequisite information, and it is strictly better than the vocabulary
heuristic `roadmap.orderByPrerequisite` uses.

**A mastery model where solid has to be earned.** `solid` is reachable only
through a teach-back scoring ≥ 7.5. A subtopic can ace recall, application, and
edge-case and still not be solid — because reciting is not understanding, and a
mastery map that cannot tell those apart is decoration.

Verified live:

```
[recall     on s1]  score 7.70  unseen -> shaky   next: application   unlocked: [s2]
[application on s1] score 7.10  shaky  -> shaky   next: edge_case
[edge_case  on s1]  score 6.20  shaky  -> shaky   next: edge_case      (held: below the 6.5 pass bar)
```

The third answer scoring 6.20 correctly **held** the archetype rather than
advancing. And `s2` unlocked the moment `s1` reached shaky.

## What exists now

| Component | Purpose |
|---|---|
| `internal/study/syllabus.go` | Topic → graph, with cycle breaking and dangling-edge removal. |
| `internal/study/drill.go` | Archetype cycle, mastery state machine, availability gating. Pure. **14 unit tests.** |
| `internal/study/question.go` | Per-archetype question generation on the cheap model. |
| `internal/grading/study.go` | Bridges Study Mode to the interview evaluator. |
| `internal/httpapi/study.go` | Syllabus, next, answer, mastery map. |

`go vet` clean, **132 unit tests across 12 packages**.

## The bug worth keeping: retries that could not survive a 429

Decomposition failed with a 429, and the log showed why the retry did not save
it:

```
attempt 1  delay_ms: 226
attempt 2  delay_ms: 129
-> permanent failure
```

The whole retry budget was spent inside one second. **A 429 means "slow down",
and retrying 130 ms later is not slowing down.** Two causes, both real:

1. **Full jitter draws from zero.** `rand(0, window)` can return a near-zero
   delay, so an exponential backoff was not reliably backing off at all.
   Changed to decorrelated jitter — drawn from the upper half of the window —
   which keeps the anti-lockstep property while guaranteeing the wait grows.
2. **One base for every error.** A dropped connection and a rate limit want
   very different waits. 429s now use a 2 s base rather than 250 ms, and the
   budget rose to four attempts.

After the fix, the same condition produced delays of **1981 ms, 2111 ms,
7705 ms** and the call succeeded.

This had been latent since Phase 0 and would have surfaced during the demo, when
several judges hitting the service at once is exactly the scenario that produces
429s. It is worth noting that the retry *looked* correct in review — exponential,
jittered, bounded — and was ineffective against the specific error it most
needed to handle.

## Design notes

- **Study Mode is text-first and REST-only.** Drilling is faster and cheaper
  typed, and there is no conversational state to hold open. Voice is a
  per-question toggle the frontend can add over the existing relay — which is
  exactly where teach-back belongs.
- **Grading is synchronous here**, unlike an interview turn. The learner is
  sitting on a form waiting; there is no conversation to keep moving.
- **A teach-back is graded with the PM's rubric weighting.** It is by definition
  a communication test — whether they can make someone else understand — so
  grading it on technical accuracy alone would miss what is being examined.
- **Cycles are broken at decomposition.** A cycle makes every node in it
  permanently unreachable and the drill loop stalls with questions remaining.
  Detecting it costs nothing; discovering it as "the app stopped asking
  questions" costs a demo.
- **A shaky prerequisite is enough to proceed.** Requiring solid everywhere
  would make a session unfinishable, since every solid needs its own teach-back.
- **A poor answer steps back to recall.** Failing an edge-case question means
  the earlier ground was weaker than it looked.

## Honest caveats

- **The full four-archetype run to solid was not completed live.** The
  edge-case answer scored 6.20 because my canned answer addressed symmetric
  weight matrices while the generated question asked about gradient flow — the
  same scripted-answer divergence seen in Phase 5. The transition itself is
  covered by `TestSolidRequiresTeachBack` and `TestWeakTeachBackDoesNotReachSolid`.
- **The syllabus graph is not yet fed into the roadmap.** Study Mode produces
  real prerequisite edges and `roadmap.orderByPrerequisite` approximates them
  with vocabulary. Wiring the real graph through would strictly improve
  ordering for study sessions. Small change, not yet made.
- **Study sessions do not produce a report.** `handleFinalize` builds from
  turns, and Study Mode does not create turn documents. The mastery map is the
  equivalent artifact, and the roadmap works from `coverage.missing`, which
  Study Mode does populate.

## Notes for Phase 9

- Replay Mode (AD-7) is the highest-value remaining item: it is the demo's
  insurance against venue wifi, and the WebSocket protocol it needs already
  exists.
- The four observability metrics are all emitted already —
  `turn_boundary_latency_ms`, `evaluation_duration_ms`, `anchor_drop_rate`, and
  live connect success/failure. What is missing is a load test at the PRD's ten
  concurrent sessions.
- The reconnect path is still unbuilt (noted in Phase 5): session-resumption
  handles are emitted but nothing consumes them.
