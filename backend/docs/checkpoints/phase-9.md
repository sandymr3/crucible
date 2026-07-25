# Phase 9 checkpoint — Hardening, replay, load, chaos

**Status: complete. The backend build plan is finished.**

## Ghost Session (AD-7) — the demo's insurance

Verified on **deployed Cloud Run**, over `wss://`:

```
audio received     1310142 bytes (27294 ms @ 24000 Hz)
sequence gaps      0
user transcript    We used a bounded queue between ingestion workers and…
ai transcript      Hello. In DataMesh, you mentioned handling 2000 requests…

Vertex cost of this session: 0 tokens, 0 calls
```

A full 27-second interview — audio, both transcripts, frame timings — with
**zero Vertex calls**. It drives the identical WebSocket protocol, so the
frontend cannot tell the difference and needs no replay-specific code.

PRD risk R3 rated venue wifi degrading the audio stream as Medium/Critical, and
offered only "record a backup video" as mitigation. A video is visibly an
admission of failure. This is not: it costs nothing, cannot be broken by a
network, a rate limit, or a Vertex outage, and exercises the real UI.

Recording is client-side, in `wsprobe -record`. What the client received is
exactly what a replay must emit, so a fixture built this way is faithful by
construction rather than through a separate serialisation that could drift.

## Load test — 10 concurrent sessions

PRD §4.4 targets ten, because "judges may all click at once".

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

Run with `bash deploy/loadtest.sh 10 10`.

**Replay sessions are used deliberately.** They exercise the entire relay path —
upgrade, auth, ownership, guardrails, the write pump, audio framing and sequence
numbering — without opening ten simultaneous Vertex connections. That keeps the
test free and repeatable, and the Vertex path is already proven by every other
phase. It does mean this is a test of *our* concurrency, not of Vertex's.

## Chaos pass — 17 of 17

`bash deploy/chaos.sh`

| Category | Checks |
|---|---|
| Auth | unauthenticated create, garbage token, unauthenticated socket → all 401 |
| Validation | invalid persona, invalid mode, study without topic, replay without fixture, digest before resume, oversized JD, non-PDF upload → all 400 |
| Isolation | cross-user read, cross-user socket, nonexistent session → all 404 (never 403) |
| Graceful states | report and roadmap before session end → 202, not an error |
| Guardrails | daily cap refuses the 6th session → 429 |

Every one degrades toward "the interview keeps working" rather than failing
outright.

## Observability — the four things that break

All emitted as structured JSON to stdout, which Cloud Logging ingests for free:

| Metric | Where |
|---|---|
| `turn_boundary_latency_ms` | `internal/live/relay.go` |
| `evaluation_duration_ms` | `internal/evaluator/evaluator.go` |
| `anchor_drop_rate` | `internal/evaluator/evaluator.go` |
| live connect success / failure | `internal/live/relay.go` |

## Tooling bugs worth recording

Three shell-level problems cost real time this phase, and all three looked like
product failures:

1. **`pkill` does not exist here** (exit 127), so every "cleanup" between test
   runs silently did nothing. Stale servers accumulated holding port 8080, and
   the next run's requests hung. `taskkill //F //IM` is the working form. This
   is why two load-test attempts timed out before anything was wrong with the
   server.
2. **Ten concurrent `go run` invocations each recompile the binary.** The
   bottleneck was the Go toolchain, not the service. Build once, run the binary.
3. **Backgrounded subshells with `wait` are unreliable here** — the load test
   completed (10 replays started, 10 finished, results on disk) while the
   wrapper appeared to hang. Reading the artifacts directly was faster than
   trusting the harness.

None of these were application defects. Worth remembering the pattern: **when a
test harness fails on Windows, suspect the harness before the code.**

## Honest caveats — what is NOT done

- **The reconnect path is still unbuilt.** `SessionResumption` is enabled on the
  Live connect so the server emits resumption handles, but nothing consumes
  them. A dropped socket ends the session rather than silently reconnecting.
  This has been carried since Phase 5 and is the largest remaining gap against
  PRD §16.6.
- **The load test does not exercise Vertex under concurrency.** Ten simultaneous
  live sessions would cost credits and, given the 429s already seen this build,
  would likely rate-limit. Concurrency of the relay is proven; concurrency of
  the model path is not.
- **The binary is 53 MB**, of which the embedded fixture is ~2 MB and the rest is
  the Go runtime plus the GCP SDKs. Fine for Cloud Run; worth knowing before
  adding more fixtures.
- **One fixture exists.** A second, showing a weak answer and a band demotion,
  would make the replay a more complete demo safety net.
- **`ROADMAP_HORIZON_DAYS` is fixed at 7** and Study Mode's real dependency
  graph is still not fed into the roadmap's ordering — both carried from
  Phase 7 and 8.

## Final state

| | |
|---|---|
| Packages | 12 with tests, 18 total |
| Unit tests | 125 |
| Live integration tests | 7 (`-tags=integration`) |
| Prompt assets | 11, content-hashed |
| Deployed | `https://crucible-backend-103350253775.us-central1.run.app` |
| `go vet` | clean |

All nine phases complete. The PRD §2 compliance matrix is 8/8, and the five
acceptance criteria in §5.1 that this backend can satisfy on its own are met:
resume-grounded first question, zero red spans on an excellent answer, amber on
a vague one, blue on a fabricated claim, a visible band change, and a roadmap
whose links all resolve.
