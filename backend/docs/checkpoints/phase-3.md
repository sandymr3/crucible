# Phase 3 checkpoint — Ingestion, digest, personas

**Status: complete.**

> **Exit criterion:** the AI's first spoken question references a specific
> project from an uploaded resume. ✅ Met, locally and on the deployed service.

Deployed, over `wss://`, from a real PDF upload:

> *"Hello. In your internship at Netlyx, how did you structure the worker pool
> to guarantee idempotent tasks?"*

It names the employer and a specific technical concern from the resume — and it
did **not** read the bracketed system directive aloud, which is PRD risk R9.

## What exists now

| Component | Purpose |
|---|---|
| `internal/prompts` | `go:embed` assets, content-hashed. Four prompts shipped. |
| `internal/persona` | Three interviewers: rubric weights, probe doctrine, distinct voice, temperature. 10 unit tests. |
| `internal/blob` | GCS upload/download with streamed size enforcement. |
| `internal/ingest` | Resume PDF + JD → Session Digest via controlled generation. |
| `internal/httpapi/ingest.go` | Resume upload, digest, plan editing, persona cards. |
| `testdata/make_resume.py` | Generates the resume fixture — no binary in the repo, no build dependency. |

`go vet` clean, **37 unit tests passing**.

## The personas are genuinely different

Same resume, same JD, three personas — the opening questions diverge in exactly
the way the rubric weights predict:

| Persona | Voice | Opening question |
|---|---|---|
| Tech Lead | Charon | "…how did you structure the queue-driven worker pool to **safely handle billing retries without duplicate charges**?" |
| Architect | Orus | "…how was the queue-driven worker pool **structured to reliably process** monolithic billing jobs?" |
| PM | Aoede | "**Hi there, welcome!** …how was the queue-driven worker pool **designed to reduce the billing job runtime**?" |

The Tech Lead goes at the failure mode, the Architect at structure, the PM opens
warmly and asks about outcome. Voices are distinct and a unit test enforces that
they stay that way.

## Digest quality

15–17 s, 4–5 claims, 5 plan areas with varied target bands (2/3/4/4/5). The
probe angles are the part that matters, and they are specific enough to be
uncomfortable:

- "How were false positives in the bloom filter handled during downstream
  deduplication to avoid dropping real feature updates?"
- "How was the 2000 requests per second figure measured, and what was the
  average payload size per request?"
- "How did log compaction interact with active client writes during snapshot
  generation?"

**Duration is over the PRD's 4–8 s budget.** It sits behind a "Reading your
resume…" screen so it is tolerable, but if it needs to come down the lever is
trimming the prompt rather than changing models — `gemini-3.6-flash` was already
selected on latency in Phase 0.

Plan editing works end to end and demonstrably reshapes the interview: dropping
"Streaming Feature Pipelines" moved the opening question from DataMesh to
RecSys-Lite's Redis caching layer. Dropping every area is refused.

## Bugs found and fixed

### 1. Nothing triggered the interviewer's first question

With manual activity detection the model never speaks unprompted — it waits for
a turn boundary, which at session start has not happened. The session would
connect, report LISTENING, and sit in silence forever.

Added a `begin` client control frame carrying a bracketed do-not-read-aloud
directive. The client sends it once its audio pipeline is ready, so the opening
question — the strongest moment in the product — is never spoken into a page
that cannot yet play it.

### 2. A crashed instance locked users out permanently

Sessions are marked `live` on connect and cleared on teardown. Kill the process
in between and the document says `live` forever, so the
one-live-session-per-user rule blocks that user from ever starting another
session. I hit this by killing the server mid-test, and on demo day a crash or a
revision swap would do the same.

Fixed with a deterministic staleness rule rather than a heartbeat: the relay
enforces a hard 12-minute cap, so anything claiming to be live past 15 minutes
is provably stale. `CountActiveSessions` ignores those, and
`ReapStaleLiveSessions` clears them at startup — an instance that has just come
up holds no live sessions of its own, so any it finds are remnants. No extra
writes, nothing to keep alive, and it cannot itself get stuck.

### 3. ⚠️ Google's frontend rejects bodyless POSTs with 411

`POST /v1/sessions/{id}/digest` takes no request body. Against Cloud Run that
returns **HTTP 411** from Google's own error page — *"POST requests require a
Content-length header"* — and the request never reaches the container, so
nothing appears in the logs. It works perfectly locally.

This is the second time GFE has intercepted a request before our code saw it
(the first was `/healthz` in Phase 0). Worth internalising as a pattern: **when
a deployed endpoint misbehaves with no corresponding log entry, suspect the
frontend, not the application.**

Clients MUST send `Content-Length: 0` on bodyless POSTs. Browser `fetch()` does
this automatically, so the frontend is unaffected; `curl -X POST` without `-d`
does not, which is how this surfaced.

## Design notes

- **No PDF text-extraction library.** Gemini reads the PDF natively, including
  two-column layouts and tables. A text extractor would be slower to build and
  worse at the job, and would return empty for a scanned resume rather than
  reading it.
- **Empty digest is a 422, not a 500.** A scanned or non-resume PDF is a
  user-actionable problem, and the message says what to do about it rather than
  "try again".
- **Dropped plan areas are marked, not removed.** The user's choice stays
  auditable and reversible, and `renderPlan` filters them out on the way to the
  interviewer.
- **A missing prompt asset aborts startup.** Prompts are compiled into the
  binary, so a missing one is a build defect that must not be discovered
  mid-interview.
- **Digest bounds are enforced server-side** — max 8 claims, max 6 plan areas.
  An over-eager digest would inflate every live session's system instruction,
  and the Live API charges for that on every turn.
- **Unknown persona falls back to Tech Lead** rather than erroring. A bad enum
  must never abort an interview in progress.

## Notes for Phase 4

- `Turn` documents are modelled but still nothing creates them. Phase 4's turn
  engine owns that.
- The worker pool is running with no handlers registered; Phase 4 registers
  `JobEvaluateTurn`.
- `Evaluation.PromptVersion` and `.Model` fields exist and are unset — the
  evaluator should stamp them, matching what ingest already does.
- Live connect is consistently ~2 s. The persona-selection screen is the natural
  place to hide it: start connecting when the user picks a card, not when they
  click Enter.
- `internal/persona.Weights.Score` is already the persona-weighted turn score
  the difficulty ladder needs in Phase 5. It is tested against the 0–10 scale
  invariant, so Phase 5 can rely on it.
