# Phase 2 checkpoint — Session lifecycle, persistence, auth, guardrails

**Status: complete.**

> **Exit criterion:** an authenticated user can create a session, it persists to
> Firestore, and every credit guardrail refuses to be exceeded. ✅ Met, verified
> locally and on the deployed service.

## What exists now

| Component | Purpose |
|---|---|
| `internal/store` | Firestore repositories and the full domain model (PRD §18). Evaluation embedded in the turn doc, `bandHistory` denormalised on the session. |
| `internal/authn` | Firebase ID token verification. Header for REST, query param for WebSocket. |
| `internal/guardrails` | Daily cap, concurrency cap, one-live-session-per-user, trivial-turn skip. 8 unit tests. |
| `internal/worker` | Buffered-channel pool, typed jobs, bounded retry, non-blocking submit. 6 unit tests. |
| `internal/httpapi` | REST surface: sessions, JD attach, end, usage, me. |
| `internal/vertexai/ledger.go` | Firestore-backed token ledger with audio/text split. |
| `deploy/firestore.rules` | Deployed. Clients read only their own data and write nothing. |

`go vet` clean, **27 unit tests passing**. Deployed revision verified.

## Firebase setup completed

Firebase was added to `crucible-hack-0725`, Identity Platform initialised, and
anonymous sign-in enabled. The Web API key lives in `secrets/webapikey.txt`
(gitignored, though a Firebase Web API key is not actually a secret — it ships
in frontend code). Real ID tokens can be minted for testing with:

```bash
curl -s -X POST "https://identitytoolkit.googleapis.com/v1/accounts:signUp?key=$(cat secrets/webapikey.txt)" \
  -H "Content-Type: application/json" -d '{"returnSecureToken":true}'
```

That is what made the auth path testable end to end without a frontend.

## Verified behaviour

Measured against the running service, not asserted:

| Check | Result |
|---|---|
| Unauthenticated REST | `401` |
| Garbage bearer token | `401` |
| Daily cap (5/user) | attempts 1–5 → `201`, attempts 6–7 → `429` |
| Cross-user session read | `404` (**not** 403 — see below) |
| Cross-user live socket | `404` |
| Invalid persona | `400` |
| Idle timeout | fires, closes the socket, closes the Vertex session, releases the slot, moves status to `evaluating` |
| Authenticated voice interview, deployed `wss://` | 1045 ms turn boundary, 0 sequence gaps, transcripts both directions |
| Per-session cost ledger | `total=378 audio_in=127 audio_out=163 calls=1` |

## Bugs found and fixed

### 1. Live session token usage was never recorded

The relay drives the SDK session object directly through `RawLive()`, which
bypasses the wrapper methods that book usage. The WebSocket was reporting
`total=455` tokens to the client while the ledger read **all zeros**.

This was the worst possible thing to miss: live audio is the dominant cost in
the system and everything else is rounding error (PRD §21.1), so a ledger
without it would have looked authoritative while capturing almost none of the
spend. Fixed with `Client.RecordLiveUsage`, called from the relay's dispatch.

### 2. The idle timeout did not actually close the billing connection

`stop()` closed the `done` channel, but `readFromClient` blocks in
`conn.ReadMessage()`, which does not observe channels. On an idle session — by
definition, no incoming messages — the read stayed blocked, so `run()` never
returned, its deferred `liveSession.Close()` never fired, and **the Vertex
connection kept billing after we had decided to end the session.**

That defeats the entire purpose of the guardrail the plan calls the single most
important one. `stop()` now closes the client connection, which unblocks the
read and lets teardown complete. Confirmed in the logs: `client disconnected`
now appears at the *same millisecond* as `closing idle session`.

### 3. A flaky test of my own making

`TestSubmitDropsWhenQueueFullRatherThanBlocking` assumed the worker had
dequeued exactly one job before the queue filled, which is a scheduler race. It
passed, then failed on a later run. Rewritten with an explicit barrier — the
handler signals that it is running before the test fills the queue — and
confirmed with `-count=15`.

## A correction worth recording

While chasing bug 2 I also reported that the post-teardown status update "never
ran". That was wrong: it was my *test* racing the Firestore write. Adding a
three-second settle showed the transition working correctly all along. The
teardown fix was real; the status bug was not. Two failures that look identical
from the outside — "status still says live" — had completely different causes,
and I conflated them before instrumenting.

## Design notes

- **`ErrForbidden` renders as 404, never 403.** A 403 confirms that a session ID
  exists, which turns the endpoint into an enumeration oracle. The real cause is
  logged.
- **Guardrails fail open on infrastructure errors.** If Firestore is unreachable,
  a session is allowed rather than blocked. The hard duration and idle caps
  still bound the damage, and a demo that refuses to start is worse than one
  that costs slightly more. This is a deliberate trade, and it is tested.
- **Firestore rules make the backend the only writer.** Clients read their own
  documents and write nothing. A client that could write its own session could
  set its own difficulty band, forge its own scores, or reset its daily cap.
- **`DEV_ALLOW_ANON` now refuses to start on Cloud Run** rather than warning. An
  unauthenticated WebSocket in front of a billing API only has to be found once.
- **The usage ledger writes on a detached context.** The common case is a
  session ending, which cancels its context immediately — writing on it would
  discard exactly the usage record for the turn we most want counted.

## Notes for Phase 3

- `SessionOpts{Voice, SystemInstruction}` is still the seam. Phase 3 fills it
  from the persona config and the digest; nothing else in the relay changes.
- `Session.Digest` is `map[string]any` on purpose, so Phase 3 can define the
  digest schema without touching the store.
- The worker pool has no handlers registered yet. Phase 4 registers
  `JobEvaluateTurn`; the pool is already started and draining.
- Turn documents are modelled but nothing creates them yet — that is Phase 4's
  turn engine. `turnCount` correctly reads 0 today.
- Connect time to the Live session is consistently ~2 s. Phase 3 should open it
  while the user is still reading the persona cards, so that cost is hidden
  behind a screen rather than a spinner.
