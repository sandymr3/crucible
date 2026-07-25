# 03 · Architecture

> Evaluation criterion 3 — *Technical Implementation* (25 marks)
> Working implementation · code quality · technical complexity · technology integration

---

## System overview

```
   BROWSER                    CLOUD RUN — one Go binary                VERTEX AI
 ┌───────────┐              ┌──────────────────────────────┐      ┌──────────────────────┐
 │ Audio     │  wss://      │  httpapi   REST, 24 routes   │ bidi │ gemini-live-2.5-     │
 │ Worklet   │ ───────────► │  live      WebSocket relay   │─────►│ flash-native-audio   │
 │ PCM16     │  PCM16 16k   │  worker    grading pool      │◄─────│   the conversation   │
 │ 16 kHz    │ ◄─────────── │                              │      ├──────────────────────┤
 └───────────┘  PCM16 24k   │            ▲         │       │─────►│ gemini-3.6-flash     │
                + JSON      │            │  inject │ grade │      │   grading, digest    │
                            │            └─────────┘       │      ├──────────────────────┤
                            │                              │─────►│ gemini-3.5-flash-lite│
                            │  Firestore ── source of truth│      │   Socratic hints     │
                            │  Cloud Storage ── audio, PDFs│      └──────────────────────┘
                            └──────────────────────────────┘
```

**The relay is structurally mandatory, not an optimisation.** Vertex authenticates
with an OAuth2 bearer token minted from a service account, and there is no safe
way to put a service-account key in frontend code. Every audio frame must pass
through the backend. This is the single fact that determines the shape of the
whole system.

**The inner loop — `inject` / `grade` — is the product.** The grade for turn *N*
travels back into the *same open live session* before turn *N+1* is asked. That is
what makes the adaptation audible rather than a post-hoc number.

---

## Technology integration

| Layer | Technology | Why this one |
|---|---|---|
| Runtime | **Go 1.26**, single static binary | Goroutines and channels *are* the concurrency model this needs. One binary, one container, no runtime to install. |
| Hosting | **Cloud Run**, `min-instances=1`, `--no-cpu-throttling`, session affinity, 3600 s timeout | Long-lived WebSockets need all four of those flags. Scales to zero when idle, so credits are not burned overnight. |
| Live model | `gemini-live-2.5-flash-native-audio` | GA native-audio bidirectional. 30 HD voices, 24 languages. |
| Reasoning | `gemini-3.6-flash` | **Selected by measurement**, reversing our own plan — see below. |
| Cheap tier | `gemini-3.5-flash-lite` | ~1.5 s. Hints and Study Mode question generation. |
| SDK | `google.golang.org/genai` v1.65.0, pinned | Complete `Live` service: `Connect`, `SendRealtimeInput`, `SendClientContent`, `Receive`, session resumption. |
| Database | **Firestore** Native, `us-central1` | Source of truth (AD-5). Security rules make the backend the only writer. |
| Object storage | **Cloud Storage**, lifecycle-managed | Résumé PDFs and turn audio. `audio/**` auto-deletes after 7 days. |
| Auth | **Firebase Auth** — Google + anonymous | ID-token verification on REST and the socket. |
| Transport | `gorilla/websocket` | PCM16 binary frames + JSON control frames. |
| Container | Multi-stage → **distroless** | No shell, no package manager, minimal attack surface. |

### Two measurements that changed the design

**1 · The two model families are in different Vertex locations.** Probed with real
bidirectional handshakes, because a REST `models.get` returns 404 for Live models
even in regions where they work:

| Model family | `us-central1` | `global` |
|---|---|---|
| Live native-audio | ✅ works (also us-east4, europe-west4) | ❌ close 1008, policy violation |
| Gemini 3.x text | ❌ 404 | ✅ works |

Co-locating both was impossible. `config.Config` carries `LiveLocation` and
`ReasoningLocation`, and `vertexai.Client` holds two SDK clients. The asymmetry
only touches post-turn calls, never the live conversation, so no user can perceive
it.

**2 · Model selection was decided by data, against our own recommendation.** Three
warm runs each, identical span-grading prompt:

| Model | Latency | Verdict |
|---|---|---|
| `gemini-3.5-flash` | 55.0 s / 7.0 s / 24.0 s | **Disqualified** — variance blows the evaluation budget |
| `gemini-3.6-flash` | 4.6 s / 4.6 s / 4.1 s | **Selected** |

The plan recommended 3.5-flash on the reasoning that a four-day-old model is a bad
demo dependency. The data overrode it.

---

## The turn lifecycle, end to end

This is the path that matters. Everything else is setup or reporting.

1. **Connect.** Client opens `wss://…/v1/sessions/{id}/live?token=…`. Token verified, ownership checked, guardrails consulted. The server establishes the Vertex Live session behind it — consistently ~2 s.
2. **`LISTENING`.** The server signals readiness. ⚠️ **The client must not send audio before this frame.** Streaming early fills the socket buffer during the ~2 s connect, the relay then drains it in a burst, and Vertex is still ingesting the backlog when the turn closes — adding ~800 ms of pure latency that appears nowhere in the logs. Measured: `drift_ms=-2354`, i.e. 1.8× real time.
3. **`begin`.** A bracketed do-not-read-aloud directive triggers the opening question. With manual activity detection the model never speaks unprompted, so without this the session connects and sits in silence forever.
4. **The interviewer speaks.** Audio streams down as PCM16 @ 24 kHz with a 4-byte big-endian sequence prefix; the output transcript streams word by word alongside it.
5. **`activity_start` → PCM16 @ 16 kHz, 20 ms frames → `activity_end`.** Audio and control share **one ordered upstream queue** — their order is load-bearing. An `activity_end` that overtook the tail of the audio would close the turn on a truncated answer. Two channels would race.
6. **Boundary.** `activity_end` sets `boundaryPending`; the turn closes when the *transcription* lands, not on `activity_end` itself. Transcription arrives asynchronously and later, from a different goroutine — closing early snapshots an empty transcript. `TurnComplete` is the backstop for silence or a recognition failure.
7. **Persist and dispatch.** Transcript snapshot, audio flushed to GCS as a WAV, turn document written, `EvaluateTurn` job queued. **Not awaited.**
8. **Grade.** Span-level evaluation with `responseSchema`, temperature 0.2, `thinkingBudget=512`. Confidence gating applied. Spans anchored to the transcript in Go.
9. **Adapt.** The grade folds into the rolling score, the band ladder and the coverage sets, and is persisted.
10. **Inject.** The deadline race (AD-3) resolves; a coach-state directive travels the same ordered upstream queue, so it can never overtake the tail of an answer still being transmitted.
11. **Next question** — measurably shaped by step 8.

Measured turn-boundary latency on the deployed service, over `wss://`:
**966 / 1130 / 1213 / 1420 ms** against a 1200 ms target, with a direct-to-Vertex
floor of 892 ms. Zero audio sequence gaps across every run.

---

## Package map — 24 internal packages

```
backend/
  cmd/
    server/       the one binary: REST + WebSocket relay + workers
    livespike/    standalone Vertex Live proof, kept as a smoke test
    wsprobe/      CLI stand-in for the browser; speaks the real protocol
    regionprobe/  which Vertex regions serve which models
  internal/
```

| Package | Role |
|---|---|
| `config` | Every env var, model ID, cap and timeout. **No magic number lives outside it.** |
| `logging` | Structured JSON to stdout with request/session correlation IDs |
| `vertexai` | SDK wrapper: two clients, retry, structured generation, usage ledger |
| `authn` | Firebase ID-token verification — header for REST, query param for the socket |
| `httpapi` | Router and 24 handlers, including the 202-polling helpers |
| `live` | WebSocket upgrade, the relay goroutine pair, turn boundaries, replay |
| `turn` | Transport-agnostic turn FSM and buffers (AD-6) |
| `audio` | PCM16 helpers: WAV read/write, resampling, RMS, frame splitting |
| `persona` | Three interviewers: rubric weights, probe doctrine, voice, temperature |
| `ingest` | Résumé PDF + JD → Session Digest |
| `evaluator` | Span-level grading: schema, prompt, validation, confidence gating |
| `anchor` | Four-tier span resolver + drop-rate metric |
| `difficulty` | Band ladder and coverage sets — **pure, no I/O** |
| `grading` | Turn sink, worker handlers, the adaptation and injection loops |
| `delivery` | Pace and disfluency, computed from **answer audio** |
| `report` | Deterministic aggregation: radar, sparkline, strengths, gaps |
| `roadmap` | Cluster → rank → prerequisite-order → ground → verify links |
| `study` | Syllabus decomposition, four-archetype drill loop, mastery |
| `replay` | Ghost Session driver (AD-7) |
| `store` | Firestore repositories and the domain model |
| `blob` | GCS upload/download with streamed size enforcement |
| `guardrails` | Session, daily and concurrency caps |
| `worker` | Buffered-channel pool, typed jobs, bounded retry |
| `prompts` | 11 prompt assets, `go:embed`, content-hashed |

**11,912 lines of non-test Go, 3,010 lines of test Go, 125 test functions.**

Two rules hold across the tree: **no model ID or magic number outside
`internal/config`**, and **no prompt text outside `internal/prompts`**.

---

## Data model

Firestore, with two deliberate denormalisations.

```
users/{uid}/counters/{date}          daily session count (transactional)
sessions/{sessionId}
  uid, mode, status, persona, topic
  digest{}                           claims, probe angles, interview plan
  difficultyBand, bandHistory[]      ← denormalised: the sparkline needs no query
  coverage{ proven[], shaky[], missing[] }
  adapt{}                            ← streak counters and previous score
  costEstimate, turnCount, fixtureId
  turns/{turnId}
    question, transcript, audioGcsUri, gradingStatus
    evaluation{}                     ← EMBEDDED: one read renders a turn
    delivery{}, hints[]
  report/{singleton}
  roadmap/{singleton}
usage/{date}                         token ledger, split by model and audio/text
```

**`adapt` exists because of the build's most valuable bug.** The adaptive engine
was completely inert in production while all 16 unit tests passed. `adapt()`
rebuilt `difficulty.State` from Firestore each turn but carried only band,
coverage and turn index — **not the streak counters**. So `StrongStreak` reset
every turn and could never reach the two consecutive turns a promotion requires.

The tests could not see it because they keep one `State` in memory across calls.
Production does not: each turn is graded by a worker that reconstructs state from
Firestore, possibly on a different instance.

The fix was persisting `AdaptState` — but the more useful fix was two new tests
that model the persistence boundary explicitly, one asserting a promotion survives
a round-trip through *only the stored fields*, and one asserting that dropping the
streak fields **must** leave the band stuck, so the guard cannot rot into a
tautology.

> **The generalisable lesson: a pure component with a persistence boundary needs a
> test that crosses that boundary.** Testing the pure part proves the algorithm,
> not the system.

---

## Code-quality practices worth naming

- **Every external call goes through one wrapper.** `vertexai.GenerateStructured` does retry plus usage accounting. Reaching for `RawText()` / `RawLive()` silently opts out of both — a mistake made twice during the build (losing all Live token accounting once, and turning a 429 into a failed evaluation once). It is now documented in the code as the named anti-pattern it is.
- **Retry that survives the error it exists for.** Full jitter draws from zero, so an "exponential backoff" produced delays of 226 ms and 129 ms against a 429 — the entire budget spent inside one second. Now decorrelated jitter drawn from the upper half of the window, with a separate 2 s base for rate limits: **1981 / 2111 / 7705 ms**, and the call succeeds.
- **A server message is a bag of parts.** Audio, input transcript and output transcript arrive together on one message. `dispatch` uses consecutive `if`s, never a `switch` — the single easiest way to silently drop transcript data.
- **Backpressure drops the client, not the model.** A wedged client must never apply backpressure all the way to a billing connection.
- **One writer goroutine per direction.** gorilla permits a single concurrent writer; several goroutines produce frames, so all funnel through a channel.
- **Idempotent, re-drivable jobs.** Re-running an evaluation on a complete turn returns immediately rather than re-grading and re-charging.
- **Detached contexts where it counts.** Ending a session cancels its request context instantly — and the final turn is precisely the one most worth grading, and the usage record most worth writing.
- **A missing prompt asset aborts startup.** Prompts are compiled in, so a missing one is a build defect that must not be discovered mid-interview.

Next: [04-api-reference.md](04-api-reference.md).
