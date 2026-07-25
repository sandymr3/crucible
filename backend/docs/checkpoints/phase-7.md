# Phase 7 checkpoint — Study roadmap with Search grounding

**Status: complete.**

> **Exit criterion:** every roadmap link resolves. ✅ **7 of 7 verified HTTP 200**,
> independently of the server's own check.

A real generated plan, from a session where the candidate gave a deliberately
vague answer about backpressure:

```
Day 1  buffer management strategies    docs.oracle.com/…/ArrayBlockingQueue.html
                                       kafka.apache.org/…#producerconfigs_buffer.memory
Day 2  queue level backpressure        nightlies.apache.org/flink/…/back_pressure/
Day 3  producer throttling mechanisms  kafka.apache.org/…#producerconfigs_max.block.ms
Day 4  consumer lag tracking           docs.aws.amazon.com/msk/…/monitoring-consumer-lag.html
Day 5  reactive stream flow control    github.com/reactive-streams/reactive-streams-jvm
Day 6  pipeline observability          opentelemetry.io/docs/concepts/signals/traces/

RETEST after day 3 · tech_lead · band 4 · focus: buffer management, backpressure, consumer lag
```

Note the ordering: buffers before backpressure before throttling before
observability. That is prerequisite order, not score order.

## What exists now

| Component | Purpose |
|---|---|
| `internal/roadmap/rank.go` | Clustering, JD-weighted scoring, prerequisite ordering. Pure. **12 unit tests.** |
| `internal/roadmap/roadmap.go` | One grounded call, then server-side HTTP verification of every URL. |
| `internal/prompts/assets/roadmap_build.md` | Resource rules and the allowlist intent. |
| `internal/httpapi/roadmap.go` | `GET /roadmap` (202 polling), `POST /retest`. |

`go vet` clean, **118 unit tests across 11 packages**.

## The design decision that earns the exit criterion

**Every URL is fetched over HTTP before anyone sees it, and dead ones are
dropped.** This is the same philosophy as span anchoring: verify server-side,
drop what fails, never show the user something we could not confirm.

That matters because grounding is not the guarantee the PRD assumes. Measured
directly:

- `googleSearch` **is** accepted alongside `responseSchema` — contrary to the
  historical constraint that tools and structured output are mutually exclusive.
- But a grounded call returned **`groundingChunks: 0`** in one test, meaning the
  model answered from parametric memory while the tool was enabled. The URLs it
  produced happened to be real, but nothing in the response distinguished that
  from invention.

So trusting the grounding metadata would have been trusting a signal that is
sometimes simply absent. Fetching the URL is deterministic and cheap: seven
parallel HEAD/GET checks add about a second to a job that already takes a
minute.

The check tries HEAD then falls back to GET, because plenty of documentation
sites answer 405 to HEAD while serving GET perfectly well — rejecting those
would silently discard good links.

## Degradation ladder

1. Grounded call succeeds → links verified, dead ones dropped.
2. Grounded call fails → retried **ungrounded**, plan generated without links,
   `grounded: false` and a note saying so.
3. All links fail verification → plan kept, note explains the concepts and
   practice tasks still apply.

A roadmap with no links beats no roadmap, and at every step the response says
honestly what it is.

## Design notes

- **One grounded call for the whole plan**, never one per day. PRD §14.2 budgets
  a single grounded call per roadmap.
- **Clustering collapses near-duplicates.** "flow control signalling" and
  "signalling for flow control" are one study day, not two — the cluster key
  sorts significant words so word order cannot split a concept, and strips
  parenthetical asides so "load shedding" and "load shedding (e.g. token bucket)"
  merge.
- **Ranking picks WHICH concepts, prerequisite order decides WHEN.** Score
  selects the top N ≈ 1.5 per day; the survivors are then re-sorted so
  foundations come first.
- **Severity derives from the turn score.** A gap in an answer that scored 2 is
  more urgent than the same gap in one that scored 8.
- **Proven concepts are excluded.** Telling a candidate to study something they
  demonstrated is the fastest way to lose their trust in the whole report.
- **The retest inherits the digest, JD, and resume URI**, so the candidate
  re-enters the room without re-uploading anything, and starts one band above
  where they finished — the point is to prove the gap closed, not to repeat the
  same difficulty.
- **A retest consumes a daily allocation.** Exempting it would make the cap
  trivially bypassable.

## Bugs found

### My own test over-merged, and exposed a real rule that was too aggressive

`TestConceptCountScalesWithHorizon` used fixture names like "concept a",
"concept b" — and `clusterKey` drops tokens shorter than three characters, so
all thirty collapsed into one cluster. The test was wrong, but the rule was too:
**"IO", "ML", "GC", "OS", "TS" are meaningful in this domain** and dropping them
would merge genuinely different concepts. Threshold lowered to two characters.

### Two shell artefacts that looked like product failures

Both worth recording because both initially read as "the exit criterion failed":

1. `curl` inside a `while read` loop consumes stdin, so every request after the
   first died. Fixed with `< /dev/null`.
2. Python on Windows wrote the URL list with CRLF, so each URL carried a
   trailing `\r` and every request returned HTTP 000. Fixed with `tr -d '\r'`.

The links were valid the entire time. Worth remembering that **HTTP 000 is a
connection failure, not a status code** — it means the request never completed,
so suspect the client before the target.

## Honest caveats

- **The prerequisite ordering is a heuristic, not a dependency graph.** Nothing
  in the pipeline produces real prerequisite edges, so ordering keys off
  foundational versus advanced vocabulary. It produced a sensible sequence here
  (buffers → backpressure → throttling → observability), but it is approximate.
  Study Mode's syllabus decomposition in Phase 8 *does* produce a real
  dependency graph; if a session has one, it should be preferred over this.
- **`ROADMAP_HORIZON_DAYS` is fixed at 7.** The PRD wants the candidate to say
  how long they have ("eleven days"). The plumbing takes a parameter; nothing
  asks the user for it yet.
- **Roadmap generation takes roughly 30–60 s** — a grounded call plus link
  verification. It runs on a worker after the report, so the user sees the
  report first and the roadmap arrives while they are reading it.

## Notes for Phase 8

- Study Mode reuses evaluation, adaptation, and this roadmap wholesale (AD-6).
  Only ingestion and question generation are new.
- Syllabus decomposition produces **real prerequisite edges** — the thing
  `orderByPrerequisite` currently approximates. Consider having Study Mode
  sessions pass their graph into `Rank`.
- `store.ModeStudy` already exists and `POST /v1/sessions` accepts it with a
  required topic; nothing consumes it yet.
- The four archetypes are recall → application → edge case → teach-back, and
  `solid` mastery requires a correct **teach-back**, not merely recall.
