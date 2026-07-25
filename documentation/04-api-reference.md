# 04 · API reference

> Evaluation criterion 3 — *Technical Implementation* (25 marks)

Base URL: `https://crucible-backend-103350253775.us-central1.run.app`

All `/v1/**` routes require `Authorization: Bearer <firebase-id-token>`.
The health routes are unauthenticated.

---

## Two things that will waste your time if you don't know them

Both were found against the deployed service, and both fail *before* the request
reaches the container — so nothing appears in Cloud Run logs.

**1 · Probe `/health`, never `/healthz`.** Google's frontend intercepts `/healthz`
on `*.run.app` and returns its own HTML 404. `/health`, `/_health`, `/livez`,
`/v1/healthz`, `/status` and `/ping` all pass through; only `/healthz` is taken.
It stays registered because it works locally and behind a custom domain, but
**`/health` is canonical** for anything probing the raw run.app URL.

**2 · Send `Content-Length: 0` on bodyless POSTs.** Google's frontend answers
**HTTP 411** — *"POST requests require a Content-length header"* — to a POST with
no body. Browser `fetch()` does this automatically so the frontend is unaffected;
`curl -X POST` without `-d` does not, which is how it surfaced.

---

## REST — 24 routes

### Health

| Method | Path | Notes |
|---|---|---|
| `GET` | `/health` | **Canonical.** `{"status":"ok"}` |
| `GET` | `/healthz` | Registered, but intercepted by GFE on `*.run.app` |
| `GET` | `/v1/healthz` | Passes through |
| `GET` | `/readyz` | Performs a real 1-token Vertex call, so readiness means "Vertex works". Reports `credential_source`. |

### Session lifecycle

| Method | Path | Body / notes |
|---|---|---|
| `POST` | `/v1/sessions` | `{mode, persona?, topic?, fixtureId?}` — `mode` is `interview`, `study` or `replay` |
| `GET` | `/v1/sessions` | Paginated history for the caller |
| `GET` | `/v1/sessions/{id}` | Poll target for the configuring screen |
| `POST` | `/v1/sessions/{id}/end` | Returns immediately, dispatches finalization |
| `GET` | `/v1/sessions/{id}/usage` | Per-session token and cost breakdown |
| `GET` | `/v1/me` | The caller's identity |
| `GET` | `/v1/personas` | The three persona cards |

### Ingestion

| Method | Path | Body / notes |
|---|---|---|
| `POST` | `/v1/sessions/{id}/resume` | multipart, PDF only, ≤ 10 MB |
| `POST` | `/v1/sessions/{id}/jd` | `{text}`, ≤ 20 k chars |
| `POST` | `/v1/sessions/{id}/digest` | Bodyless — **send `Content-Length: 0`**. Takes 15–17 s. Returns 422 on a scanned/non-résumé PDF. |
| `PATCH` | `/v1/sessions/{id}/plan` | `{droppedAreaIds:[]}` — drop interview-plan areas. Dropping every area is refused. |

### Turns and results

| Method | Path | Body / notes |
|---|---|---|
| `GET` | `/v1/sessions/{id}/turns` | All turns with embedded evaluations |
| `GET` | `/v1/sessions/{id}/report` | **202 `{status:"generating"}`** until ready |
| `GET` | `/v1/sessions/{id}/roadmap` | Same 202 pattern. Generation takes 30–60 s. |
| `POST` | `/v1/sessions/{id}/retest` | Materialises `retest_plan` into a configured session, inheriting digest, JD and résumé URI |

### Study Mode

| Method | Path | Body / notes |
|---|---|---|
| `POST` | `/v1/sessions/{id}/syllabus` | Topic → dependency-ordered subtopic graph |
| `GET` | `/v1/sessions/{id}/study/next` | Next subtopic + archetype |
| `POST` | `/v1/sessions/{id}/study/answer` | Graded **synchronously** — the learner is waiting on a form |
| `GET` | `/v1/sessions/{id}/mastery` | Mastery map: `unseen \| attempted \| shaky \| solid` |

### Live

| Method | Path | Notes |
|---|---|---|
| `GET` | `/v1/sessions/{id}/live` | WebSocket upgrade. Token as a **query parameter**, because browsers cannot set handshake headers. Full-URL logging is suppressed. |

### Status codes

| Code | Meaning here |
|---|---|
| `202` | Report or roadmap still generating — poll, this is not an error |
| `400` | Validation: bad persona, bad mode, study without a topic, replay without a fixture, oversized JD, non-PDF upload |
| `401` | Missing or invalid ID token |
| `404` | Not found **or not yours** — see below |
| `422` | Digest produced nothing; the PDF was probably a scan. User-actionable message. |
| `429` | Daily cap (5/user) or concurrency cap (10) reached |

**`403` is never returned.** A 403 confirms that a session ID exists, which turns
the endpoint into an enumeration oracle. Cross-user access renders as `404`; the
real cause is logged server-side.

---

## WebSocket protocol

`wss://…/v1/sessions/{id}/live?token=<firebase-id-token>`

### Client → server

**Text frames** (JSON, `{"type": …}`):

| Type | When |
|---|---|
| `begin` | Once, after the audio pipeline is ready. Triggers the opening question. |
| `activity_start` | Microphone goes hot |
| `activity_end` | User clicks **Done**. Closes the turn. |
| `text_answer` | `{text}` — typed answer, identical evaluation path |
| `request_hint` | Socratic hint, −0.5 score, max 2 per turn |
| `end_session` | Graceful teardown |
| `ping` | Every 20 s — Cloud Run closes idle connections, and a demo that dies during a thoughtful pause is a demo that dies |

**Binary frames:** PCM16 mono @ **16 kHz**, 20 ms / 640-byte frames.

### Server → client

**Text frames:**

| Type | Payload |
|---|---|
| `state` | `CONNECTING \| ASKING \| LISTENING \| CLOSING \| EVALUATING \| SETTLED \| ERROR` |
| `question` | The interviewer's question text |
| `transcript` | Incremental transcript, both directions |
| `evaluation` | Spans with verdicts and byte offsets, scores, concepts |
| `ungraded` | Grading failed permanently — **the interview continues** |
| `band` | Difficulty changed, with a human-readable reason |
| `hint` | Hint text and the penalty applied |
| `usage` | Running token counts |
| `turn_complete` | Model finished speaking |
| `interrupted` | Model turn was cut short |
| `error` | Code and message |
| `pong` | Ping reply |

**Binary frames:** 4-byte big-endian sequence number, then PCM16 mono @ **24 kHz**.
Note the asymmetry — 16 kHz up, 24 kHz down.

### Three client rules

1. **Do not send audio until `state: LISTENING`.** Costs ~800 ms of turn-boundary latency otherwise, invisibly. See [03-architecture.md](03-architecture.md#the-turn-lifecycle-end-to-end).
2. **Send `begin` once ready.** In manual activity mode the model never speaks unprompted; without it the session connects and stays silent.
3. **⚠️ Span offsets are Go BYTE offsets into UTF-8, not JavaScript string indices.** They agree for ASCII and diverge on the first accented character, em dash or emoji — after which *every* highlight on that turn is misplaced. Convert before slicing:

```ts
export function byteToCharMap(text: string): number[] {
  const map: number[] = []; const encoder = new TextEncoder(); let byte = 0
  for (let ch = 0; ch < text.length; ch++) {
    const cp = text.codePointAt(ch)!
    const len = encoder.encode(String.fromCodePoint(cp)).length
    for (let b = 0; b < len; b++) map[byte + b] = ch
    byte += len
    if (cp > 0xffff) ch++
  }
  map[byte] = text.length
  return map
}
```

### The four verdicts

| Verdict | Meaning |
|---|---|
| `validated` | Backed and correct |
| `incomplete` | True but thin |
| `unsupported` | Claimed, not evidenced |
| `incorrect` | Confidently wrong |

`incorrect` below the confidence floor is rewritten to `unsupported` server-side
before it is ever persisted or sent.

---

## Testing without a frontend

`cmd/wsprobe` is a CLI client that speaks this exact protocol — streams a WAV at
wall-clock pace, prints every frame, writes received audio to a WAV:

```bash
go run ./cmd/wsprobe -session <id> -token <firebase-id-token> -wav answer.wav
```

Mint a test ID token:

```bash
curl -s -X POST "https://identitytoolkit.googleapis.com/v1/accounts:signUp?key=$WEB_API_KEY" \
  -H "Content-Type: application/json" -d '{"returnSecureToken":true}'
```

Next: [05-security-and-guardrails.md](05-security-and-guardrails.md).
