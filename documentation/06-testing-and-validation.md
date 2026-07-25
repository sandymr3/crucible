# 06 · Testing and validation

> Evaluation criteria 3 and 4 — *Technical Implementation* (25) and
> *Completeness & Functionality* (15)

Everything on this page is a measurement, taken at the time it was observed and
recorded in [`../backend/docs/checkpoints/`](../backend/docs/checkpoints/). Where a
result was worse than the design target, it says so.

---

## Test suite

```bash
cd backend
go test ./...                                    # 125 test functions
go test ./... -tags=integration -timeout=10m     # live Vertex; costs credits
go vet ./...                                     # clean
```

| | |
|---|---|
| Test functions | **125** |
| Packages with tests | 12 of 24 |
| Live integration tests | 7, behind `-tags=integration` |
| Non-test Go | 11,912 lines |
| Test Go | 3,010 lines |

**Where tests were written, and where they were not.** Pure logic with real
branching gets real tests: `difficulty` (18), `study/drill` (14), `anchor` (12),
`roadmap/rank` (12), `evaluator` (11), `report` (9), `guardrails` (8), `audio` (8),
`worker` (6), `live` (5). Thin I/O wrappers do not, because a test that mocks
Firestore and asserts Firestore was called proves nothing.

---

## The acceptance criteria that matter

### Grading is calibrated, and never falsely red

The single most important property: **a deliberately excellent answer must produce
zero red spans.** Falsely telling a candidate their correct answer was wrong is
the most damaging thing this product can do.

`go test ./internal/evaluator/ -tags=integration`

| Test | Verdicts | Score | Latency |
|---|---|---|---|
| Excellent answer | `validated` ×4, confidence 0.95–1.00 | **9.60** | 5.4 s |
| Vague answer | `incomplete`, `unsupported` ×2 | **3.55** | 6.9 s |
| Fabricated claims | `unsupported` | **1.60** | 3.5 s |

**Not one `incorrect` verdict across any test** — including the answer composed
entirely of unbacked numbers, which came back `unsupported`. That is the
four-verdict taxonomy doing exactly what it was designed for, backed by the
server-side confidence gate.

The scores also *separate*: 9.60 / 3.55 / 1.60. A grader that cannot discriminate
is useless for adaptation regardless of how kind it is.

### Highlights land on the right words

**Anchor drop rate: 0%** across all three transcripts.

The most important test in `internal/anchor` asserts that a **paraphrase is
dropped**. Anchoring a paraphrase onto unrelated text attaches a verdict to words
that never made the claim — the user sees "incorrect" highlighted over a sentence
that was never wrong. A missing highlight is invisible; a misplaced one is a
visible bug.

### The first question is grounded in the actual résumé

Deployed, over `wss://`, from a real PDF upload:

> *"Hello. In your internship at Netlyx, how did you structure the worker pool to
> guarantee idempotent tasks?"*

It names the employer and a specific technical concern from the résumé — and did
**not** read the bracketed system directive aloud.

### The interviewer never reads internal state aloud

Every AI transcript captured across all five build phases was grepped for
`COACH STATE`, `SESSION START`, `do not read`, `system directive`,
`difficulty band is now` and `concepts already proven`.

**Clean. Not one leak.**

### Difficulty visibly adapts

```
← [BAND CHANGE] BAND 3 -> 4  (Difficulty raised — you've proven the fundamentals.)
    server: rolling=9.36  from=3 to=4
```

### Delivery metrics are non-zero on a disfluent answer

```
delivery analysed  156 wpm (optimal), 13 fillers, hesitation 0.85
```

### Every roadmap link resolves

**7 of 7 verified HTTP 200**, checked independently of the server's own
verification. Ordering is by prerequisite, not score:

```
Day 1  buffer management strategies    docs.oracle.com/…/ArrayBlockingQueue.html
Day 2  queue level backpressure        nightlies.apache.org/flink/…/back_pressure/
Day 3  producer throttling mechanisms  kafka.apache.org/…#producerconfigs_max.block.ms
Day 4  consumer lag tracking           docs.aws.amazon.com/msk/…/monitoring-consumer-lag.html
Day 5  reactive stream flow control    github.com/reactive-streams/reactive-streams-jvm
Day 6  pipeline observability          opentelemetry.io/docs/concepts/signals/traces/
```

### Study Mode holds the archetype when the answer is weak

```
[recall      on s1]  score 7.70  unseen -> shaky   next: application   unlocked: [s2]
[application on s1]  score 7.10  shaky  -> shaky   next: edge_case
[edge_case   on s1]  score 6.20  shaky  -> shaky   next: edge_case     (held: below the 6.5 pass bar)
```

---

## Load — 10 concurrent sessions

`bash backend/deploy/loadtest.sh 10 10`

```
session 1..10   audio 18183 ms   gaps 0
streaming audio : 10 of 10
with gaps       : 0
server errors   : 0
refusals        : 0
dropped clients : 0
```

Every session received byte-identical audio with no sequence gaps, no client
dropped for a full outbound buffer, and no errors.

**Replay sessions are used deliberately.** They exercise the entire relay path —
upgrade, auth, ownership, guardrails, the write pump, audio framing and sequence
numbering — without opening ten simultaneous Vertex connections. That keeps the
test free and repeatable. **It does mean this is a test of our concurrency, not of
Vertex's.**

## Ghost Session — the demo's insurance

Verified on deployed Cloud Run, over `wss://`:

```
audio received     1,310,142 bytes (27,294 ms @ 24000 Hz)
sequence gaps      0
user transcript    We used a bounded queue between ingestion workers and…
ai transcript      Hello. In DataMesh, you mentioned handling 2000 requests…

Vertex cost of this session: 0 tokens, 0 calls
```

A full 27-second interview for **zero** Vertex calls, over the identical protocol.

---

## Latency

| Path | Measured | Target |
|---|---|---|
| Turn boundary, deployed `wss://` | **966 / 1130 / 1213 / 1420 ms** | < 1200 ms |
| Turn boundary, authenticated run | 1045 ms | |
| Direct to Vertex (the floor) | 892 ms | |
| Live session connect | ~2 s | < 2.5 s to first audio |
| Evaluation | **5.4 s median**, 3.5–6.9 s | 4 s ❌ |
| Digest | 15–17 s | 4–8 s ❌ |
| Roadmap | 30–60 s | — |

**Two targets are missed, and both are stated rather than hidden.**

*Evaluation* exceeds its 4 s budget at the median. Acceptable rather than ideal,
because the deadline-bounded injection means the **conversation** never waits on
the grader — the heatmap simply reveals a beat later. Root cause was measured, not
guessed: holding the prompt constant and varying `thinkingBudget` showed it was
thinking tokens, not output size. The first implementation ran **17.3 s**;
`EVAL_THINKING_BUDGET=512` brought it to 3.5–6.9 s. One 17.6 s outlier has been
observed.

*Digest* sits behind a "Reading your résumé…" screen, so 15–17 s is tolerable, but
it is over budget.

---

## Bugs the tests and instrumentation caught

The most useful evidence that a test suite works is what it found.

**The adaptive engine was inert in production while 16 unit tests passed.** State
was rebuilt from Firestore each turn without the streak counters, so a promotion
could never trigger. Fixed by persisting `AdaptState`, and guarded by two new
tests that cross the persistence boundary explicitly — including one asserting
that dropping the streak fields *must* break adaptation, so the guard cannot rot
into a tautology.

**Voice turns were graded not at all.** `closeTurn()` fired on `activity_end`, but
transcription arrives asynchronously and later. Every voice turn was snapshotted
with an empty transcript and skipped as *"answer too short, words: 0"* — while
text answers worked fine and masked it. Fixed with a `boundaryPending` flag.

**`domainMatches` was documented as word-level but implemented with
`strings.Contains`,** so "serving" matched "observing" and a turn about metrics
would have been attributed to a *model serving* radar axis. The test asserting the
documented behaviour failed on its first run.

**Retry could not survive the error it existed for.** Full jitter draws from zero,
producing delays of 226 ms and 129 ms against a 429 — the whole budget spent
inside one second. Decorrelated jitter plus a separate 2 s base for rate limits
now yields 1981 / 2111 / 7705 ms, and the call succeeds. This had been latent
since Phase 0 and *looked* correct in review: exponential, jittered, bounded.

**A test-only `os.Chdir` made three integration tests silently skip** rather than
fail — and a skip reads as a pass at a glance.

**The relay added ~800 ms of phantom latency.** Two plausible hypotheses were
wrong before instrumentation found it (`drift_ms=-2354`). The lesson recorded:
**instrument before optimising.**

---

## Observability

Structured JSON to stdout, ingested by Cloud Logging at no cost. Four metrics,
chosen because they are the four things that actually break:

| Metric | Where |
|---|---|
| `turn_boundary_latency_ms` | `internal/live/relay.go` |
| `evaluation_duration_ms` | `internal/evaluator/evaluator.go` |
| `anchor_drop_rate` | `internal/evaluator/evaluator.go` |
| Live connect success / failure | `internal/live/relay.go` |

---

## What is *not* done

Stated plainly, because a submission that names its gaps is more trustworthy than
one that does not.

- **The WebSocket reconnect path is not built.** `SessionResumption` is enabled and the server emits resumption handles, but nothing consumes them. A dropped socket ends the session. This is the largest remaining gap.
- **Vertex concurrency is unproven.** The load test exercises our relay, not ten simultaneous Vertex connections.
- **The radar chart is flat below three graded turns.** With one unattributable turn every axis shows the same score.
- **`fillerPerMinute` overstates on short answers** — 51.9/min extrapolated from a 15-second answer is arithmetically correct and practically misleading.
- **The frontend is incomplete**, so there is no browser demo yet. `cmd/wsprobe` exercises the identical protocol in the meantime.
- **One replay fixture exists.** A second showing a weak answer and a band demotion would make the safety net more complete.

Next: [07-scalability-and-impact.md](07-scalability-and-impact.md).
