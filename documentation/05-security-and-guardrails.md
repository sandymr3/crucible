# 05 · Security and guardrails

> Evaluation criterion 3 — *Technical Implementation* (25 marks), security dimension

Two distinct threat models are addressed here. **Security** protects one user's
data from another. **Guardrails** protect the project's cloud credits from a
forgotten browser tab — which, on a metered speech API, is the more likely way to
lose everything.

---

## Why the backend relay exists at all

Vertex AI authenticates with an **OAuth2 bearer token minted from a service
account**, not an API key. There is no safe way to place a service-account key in
frontend code — it would be extractable from the bundle by anyone who opened
devtools, and it grants direct billable access to the project's models.

So every audio frame passes through our backend. This is not an architectural
preference; it is the security constraint that determines the entire shape of the
system.

---

## Authentication

**Firebase Auth**, Google and anonymous sign-in, verified with
`firebase.google.com/go/v4/auth`.

| Surface | How the token travels |
|---|---|
| REST | `Authorization: Bearer <id-token>` header |
| WebSocket | `?token=` query parameter — browsers cannot set headers on a WS handshake |

Both are verified identically. **Full URLs are never logged**, because the socket
URL carries the token.

`DEV_ALLOW_ANON` exists for local development and **refuses to start the process
on Cloud Run** rather than merely warning. An unauthenticated public WebSocket in
front of a billing API only has to be found once.

---

## Isolation

**`ErrForbidden` renders as 404, never 403.** A 403 tells the caller that a
session ID exists, turning the endpoint into an enumeration oracle. Cross-user
reads and cross-user socket connections both return `404`; the true cause is
logged server-side.

Verified against the running service:

| Check | Result |
|---|---|
| Unauthenticated REST | `401` |
| Garbage bearer token | `401` |
| Unauthenticated WebSocket | `401` |
| Cross-user session read | `404` |
| Cross-user live socket | `404` |

## Firestore rules — the backend is the only writer

Deployed in [`backend/deploy/firestore.rules`](../backend/deploy/firestore.rules).
Clients may read documents where `uid == request.auth.uid`, and **write nothing**.

This matters more than it first appears. A client that could write its own session
document could set its own difficulty band, forge its own scores, or reset its own
daily cap. Every mutation goes through the backend, which is the only party that
can enforce the rules in [`internal/difficulty`](../backend/internal/difficulty)
and [`internal/guardrails`](../backend/internal/guardrails).

## Least-privilege IAM

The runtime service account holds **exactly three grants**:

| Role | Scope |
|---|---|
| `roles/aiplatform.user` | project |
| `roles/datastore.user` | project |
| `roles/storage.objectAdmin` | **one bucket**, not project-wide |

No Editor. No default compute service account.

## Credential handling

- `secrets/key.json` is gitignored **and** gcloudignored, so it can reach neither GitHub nor Cloud Build. The repository is public; the root `.gitignore` carries deliberate belt-and-braces patterns (`secrets/`, `**/secrets/`, `*.key.json`, `key.json`, `serviceaccount*.json`).
- **One credential code path for both environments.** `credentials.DetectDefault` reads `GOOGLE_APPLICATION_CREDENTIALS` locally and falls through to the attached service account on Cloud Run. Same binary, no branching. `/readyz` reports which source was used, and both were verified.
- The Firebase **web** API key is not a secret — it ships in client code by design and is protected by Firestore rules, not obscurity.

## Container and data hygiene

- Multi-stage build into **distroless** — no shell, no package manager.
- Cloud Storage lifecycle deletes `audio/**` after **7 days**. Interview audio is sensitive and there is no reason to keep it.
- Uploads are bounded by a **streamed** size check, so an oversized file is rejected while being read rather than after being buffered.

---

## The nine credit guardrails

All server-enforced. A client-enforced cap is not a cap.

| # | Guardrail | Value |
|---|---|---|
| 1 | Hard session duration | 720 s, warning at 600 s |
| 2 | **Idle timeout** | **90 s** |
| 3 | Daily sessions per user | 5, transactional on `users/{uid}/counters/{date}` |
| 4 | Concurrent sessions | 10, in-process atomic counter mirrored to Firestore |
| 5 | One live session per user | Prevents parallel billing connections |
| 6 | Hints per turn | 2 |
| 7 | Trivial-turn skip | Answers under 15 words are not sent for grading |
| 8 | Digest bounds | Max 8 claims, max 6 plan areas |
| 9 | Live session closed on **every** teardown path | `defer`, never GC |

### Guardrail 2 is the important one, and it was broken

A forgotten open tab is a continuous drain on the most expensive component in the
system. The idle timeout is what stops it — and for a while it did not.

`stop()` closed a `done` channel, but `readFromClient` blocks in
`conn.ReadMessage()`, which does not observe channels. On an idle session — by
definition, no incoming messages — the read stayed blocked, `run()` never
returned, its deferred `liveSession.Close()` never fired, and **the Vertex
connection kept billing after we had decided to end the session.**

`stop()` now closes the client connection, which unblocks the read and lets
teardown complete. Confirmed in the logs: `client disconnected` now appears at the
*same millisecond* as `closing idle session`.

### Two related failures worth recording

**Live token usage was never recorded.** The relay drives the SDK session directly
through `RawLive()`, bypassing the wrapper methods that book usage. The socket
reported `total=455` tokens to the client while the ledger read **all zeros**.
This was the worst possible thing to miss: live audio is the dominant cost and
everything else is rounding error, so the ledger would have looked authoritative
while capturing almost none of the spend.

**A crashed instance locked users out permanently.** Sessions are marked `live` on
connect and cleared on teardown; kill the process in between and the document says
`live` forever, so the one-live-session-per-user rule blocks that user from ever
starting another. Fixed with a deterministic staleness rule rather than a
heartbeat: the relay enforces a hard 12-minute cap, so anything claiming to be
live past 15 minutes is provably stale. `ReapStaleLiveSessions` clears them at
startup — an instance that has just come up holds no live sessions of its own, so
any it finds are remnants.

---

## Deliberate trade-off: guardrails fail open

If Firestore is unreachable, a session is **allowed** rather than blocked.

The hard duration and idle caps still bound the damage, and a demo that refuses to
start is worse than one that costs slightly more. This is a conscious decision,
and it is covered by a test so it cannot change silently.

---

## Chaos verification — 17 of 17

`bash backend/deploy/chaos.sh`

| Category | Checks |
|---|---|
| Auth | unauthenticated create, garbage token, unauthenticated socket → all `401` |
| Validation | invalid persona, invalid mode, study without topic, replay without fixture, digest before résumé, oversized JD, non-PDF upload → all `400` |
| Isolation | cross-user read, cross-user socket, nonexistent session → all `404`, never `403` |
| Graceful states | report and roadmap before session end → `202`, not an error |
| Guardrails | daily cap refuses the 6th session → `429` |

Every one degrades toward "the interview keeps working" rather than failing
outright.

Next: [06-testing-and-validation.md](06-testing-and-validation.md).
