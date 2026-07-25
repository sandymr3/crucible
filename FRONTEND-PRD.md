# CRUCIBLE — Frontend PRD

**Building the browser half of an adaptive, voice-native AI interview coach.**

| | |
|---|---|
| **Document version** | 1.0 |
| **Date** | 26 July 2026 |
| **Audience** | The engineer or agent implementing `frontend/` |
| **Backend status** | **Complete, deployed, and verified.** Nothing in this document is speculative — every endpoint and frame type below exists and has been exercised end to end |
| **Backend URL** | `https://crucible-backend-103350253775.us-central1.run.app` |
| **Repository** | https://github.com/sandymr3/crucible |

---

# PART 0 — READ THIS FIRST

## 0.1 What already exists

A complete Go backend on Cloud Run. It does the resume ingestion, the live
speech-to-speech relay to Vertex AI, the span-level grading, the adaptive
difficulty ladder, the report aggregation, the grounded roadmap, and Study Mode.
It has 125 unit tests and 7 live integration tests. It is deployed with
`min-instances=1`.

**You are building the browser half only.** Do not reimplement any backend
logic. If you find yourself computing a score, deciding a difficulty band, or
anchoring a highlight in JavaScript, stop — the backend already did it and sent
you the answer.

Read `backend/docs/checkpoints/phase-*.md` before starting. Ten documents, one
per build phase, recording what was built and — more usefully — what broke.
Several of those failures are things the frontend can reproduce.

## 0.2 Five rules that will cost you hours if you break them

These are not style preferences. Each one was discovered the hard way against
the real backend, and each has a corresponding entry in the checkpoints.

**1. Do not send audio until the server says `LISTENING`.**
The WebSocket upgrade completes in milliseconds; the Vertex session behind it
takes ~2 seconds. If you stream on connect, audio piles into the socket buffer,
the relay forwards it in a burst, and Vertex is still ingesting it when the turn
closes. Measured cost: audio arriving at **1.8× real time** and roughly **800 ms
added to turn-boundary latency** — the one number a judge can feel. Wait for
`{"type":"state","state":"LISTENING"}`.

**2. Send `Content-Length: 0` on bodyless POSTs.**
Google's frontend rejects a POST with no `Content-Length` header, returning its
own **HTTP 411** before the request reaches the container — so nothing appears in
the logs. `fetch()` with `method:'POST'` and no body sets this automatically, so
you are fine by default; but if you ever hand-roll a request or use a library
that omits it, this is what you will see. Affects `/digest`, `/end`, `/retest`,
`/syllabus`.

**3. Probe `/health`, never `/healthz`.**
Google's frontend **intercepts `/healthz`** on `*.run.app` and answers with its
own HTML 404. The request never reaches the app. `/health` and `/v1/healthz`
both work.

**4. Span offsets are BYTE offsets, not JavaScript string indices.**
The backend is Go. `span.start` and `span.end` index a **UTF-8 byte array**.
JavaScript strings are UTF-16. For pure ASCII they coincide; the moment a
transcript contains an accented character, an em dash, or an emoji they diverge
and every highlight after that point lands in the wrong place. **You must convert.**
See §9.4 for the exact function. This is the single most likely visual bug in
the product.

**5. Evaluation arrives 5–8 seconds AFTER the answer, not with it.**
This is by design — the conversation never waits on the grader. Your UI must
have a real `EVALUATING` state that survives the user starting their next answer,
and the heatmap must be able to attach to a turn the user has already scrolled
past. Do not build a flow that assumes the grade is synchronous.

## 0.3 Skills you must invoke

**Before writing any component, invoke `anthropic-skills:frontend-design`.**
It calibrates visual quality and steers away from generic AI-generated
aesthetics. This product lives or dies on the Live Room feeling alive; a
default-shadcn dashboard will read as a class project regardless of how good the
backend is.

**Before building the radar chart, the band sparkline, or the pace dial, invoke
the `dataviz` skill.** It provides the colour formula, accessibility
constraints, and mark specifications so the three charts read as one system
rather than three libraries. The report screen has three distinct visualisations
on it; without a shared system they will clash.

Invoke both by name via the Skill tool. Do not skip them and do not summarise
them from memory.

---

# PART I — PRODUCT

## 1. The pitch, and what the frontend is responsible for

> You upload your resume and the job description you are actually chasing, pick
> who is grilling you, and then you *talk*. The AI talks back in its own voice,
> in real time, asking questions rooted in the projects on your resume. When you
> finish an answer, your own words light up on screen: green where you nailed
> the concept, amber where you were vague, blue where you claimed something you
> could not support.

The backend makes that true. **The frontend makes it felt.** Specifically it
owns:

- The microphone, and turning it into 16 kHz PCM16 frames
- Playing 24 kHz PCM16 back without clicks or drift
- The visualiser, driven by real amplitude
- The transcript, streaming in as the user speaks
- The heatmap reveal — the most screenshot-able frame in the product
- Making adaptation *visible* when the difficulty band changes
- Never leaving the user unsure whether the system is listening, thinking, or dead

## 2. The three moments that matter

If you are short on time, protect these in this order.

**Moment 1 — 0:40. The AI speaks a question about the user's own project.**
The single strongest beat. Requires: audio playback working, the question
rendering as text simultaneously, and the visualiser moving with the voice. If
only one thing works, make it this.

**Moment 2 — 1:25. The heatmap reveal.**
The transcript sits in plain text for a beat, then spans illuminate left to
right over ~600 ms. Staggered. This costs about twenty lines of CSS and it is
what people screenshot.

**Moment 3 — 1:45. The band changes on camera.**
A toast, an animated indicator, and then an audibly harder next question.
Adaptation the user cannot perceive is worthless.

## 3. Non-functional targets

| Requirement | Target | Why |
|---|---|---|
| Time to first AI audio after entering the Live Room | < 2.5 s | Slower reads as broken on stage |
| Transcript delta on screen | < 400 ms behind speech | Creates the "it is listening" effect |
| Heatmap reveal after evaluation frame | < 200 ms | The frame arrives late already; do not add to it |
| Audio playback underruns | 0 in a 10-minute session | Track and log them; best early warning of network trouble |
| First contentful paint | < 1.5 s | Judges click before you finish talking |
| Works at | 1280×800 minimum | Projector resolution. Test at this size, not on a 4K laptop |

---

# PART II — THE CONTRACT

Everything in this part is verified against the deployed backend. Treat it as
authoritative.

## 4. Environment configuration

```ts
// src/config.ts
export const CONFIG = {
  // Cloud Run service. In production, serve the frontend from Firebase Hosting
  // with a rewrite so this becomes same-origin and CORS disappears entirely.
  API_BASE: import.meta.env.VITE_API_BASE
    ?? 'https://crucible-backend-103350253775.us-central1.run.app',

  // ws:// for local, wss:// for deployed. Derive rather than duplicating.
  get WS_BASE() { return this.API_BASE.replace(/^http/, 'ws') },

  firebase: {
    apiKey: 'AIzaSyCSQ70M3P6dXXGymDdRLAmLOZKRTu9oAv8',
    authDomain: 'crucible-hack-0725.firebaseapp.com',
    projectId: 'crucible-hack-0725',
  },
} as const
```

**On the Firebase API key.** It is not a secret. Firebase web API keys are
designed to ship in client bundles; they identify the project, they do not
authorise anything. The real protection is Firestore security rules (already
deployed, backend-only writes) plus authorised domains. Do not hide it in a way
that breaks the build, and do not panic that it is in a public repo.

**One honest caveat:** anonymous auth is enabled, so in principle someone could
farm anonymous accounts to bypass the per-user daily session cap. For a
hackathon this is acceptable. If it matters later, the fix is Firebase App Check
plus restricting the key to authorised domains — not hiding the key.

## 5. Authentication

Firebase Auth, Google sign-in as the primary path, anonymous as a fallback so a
judge can try it without a Google account.

```ts
import { initializeApp } from 'firebase/app'
import { getAuth, signInWithPopup, GoogleAuthProvider, signInAnonymously }
  from 'firebase/auth'

const app  = initializeApp(CONFIG.firebase)
export const auth = getAuth(app)

export const signInGoogle    = () => signInWithPopup(auth, new GoogleAuthProvider())
export const signInAsGuest   = () => signInAnonymously(auth)
```

**Every request carries a fresh ID token.** Firebase tokens expire after an
hour; `getIdToken()` refreshes transparently, so call it per request rather than
caching the string.

```ts
async function authHeader(): Promise<Record<string, string>> {
  const user = auth.currentUser
  if (!user) throw new Error('not signed in')
  return { Authorization: `Bearer ${await user.getIdToken()}` }
}
```

**The WebSocket takes the token as a query parameter**, because browsers cannot
set headers on a WebSocket handshake. This is a deliberate, documented trade —
the backend never logs full URLs to limit the exposure. Do not log them either.

## 6. REST API — complete reference

Base path `/v1`. Every route below requires `Authorization: Bearer <firebase-id-token>`
except the health endpoints.

### 6.1 Identity and catalogue

#### `GET /v1/me`
```json
{ "uid": "abc123", "email": "", "displayName": "", "anonymous": true }
```

#### `GET /v1/personas`
The persona selection cards. `punishes` is the field that makes a user pick the
one that scares them — show it prominently.
```json
{ "personas": [
  { "id": "tech_lead", "name": "The Tech Lead",
    "tagline": "Wants the mechanism, not the vocabulary.",
    "punishes": "Hand-waving, buzzwords without mechanism, \"it just works\".",
    "weights": { "technicalAccuracy": 0.5, "depth": 0.25, "structure": 0.15, "communicationClarity": 0.1 } },
  { "id": "architect", "name": "The System Architect",
    "tagline": "Will let you build the wrong design, then probe the crack.",
    "punishes": "Premature detail, unexamined defaults, no mention of tradeoffs.",
    "weights": { "structure": 0.35, "technicalAccuracy": 0.3, "depth": 0.2, "communicationClarity": 0.15 } },
  { "id": "pm", "name": "The Product Manager",
    "tagline": "Warm, curious, and will ask who the user was.",
    "punishes": "Jargon without translation, no user in the story, blaming teammates.",
    "weights": { "communicationClarity": 0.45, "structure": 0.25, "technicalAccuracy": 0.2, "depth": 0.1 } }
] }
```

### 6.2 Session lifecycle

#### `POST /v1/sessions` → `201`
```jsonc
// Interview mode
{ "mode": "interview", "persona": "tech_lead" }   // persona optional at creation
// Study mode
{ "mode": "study", "topic": "Transformer attention" }
// Replay mode — the demo safety net
{ "mode": "replay", "fixtureId": "demo-ml-engineer" }
```
Returns the full `Session` object (§9.2). Errors: `400 invalid_mode`,
`400 invalid_persona`, `400 missing_topic`, `400 missing_fixture`,
**`429 daily_cap_reached`** (5 sessions/user/day; replay sessions are exempt).

#### `GET /v1/sessions/{id}` → full `Session`
Poll target for the configuring screen. `404` if it does not exist **or belongs
to someone else** — the backend deliberately does not distinguish these, so do
not write UI that assumes a 404 means "deleted".

#### `GET /v1/sessions` → `{ "sessions": Session[] }`
Paginated history, newest first, max 20.

#### `POST /v1/sessions/{id}/end` → `202 { "status": "ending" }`
Triggers report generation. Returns immediately; poll `/report`. **Idempotent** —
calling it twice returns `200 { "status": "already_complete" }`, so a
mis-clicked End plus an unload handler is safe.

**Always show a confirmation dialog on manual end.** A mis-clicked end button
mid-demo with no confirmation is a bad afternoon.

#### `GET /v1/sessions/{id}/usage`
```json
{ "sessionId": "...", "turnCount": 3,
  "cost": { "totalTokens": 13455, "promptAudioTokens": 127, "responseAudioTokens": 233,
            "promptTextTokens": 42, "responseTextTokens": 53, "calls": 4 } }
```
Worth a small, tasteful "this session cost N tokens" readout. "Here are our
actual per-session unit economics" is a genuinely strong answer to a judge.

### 6.3 Interview configuration

#### `POST /v1/sessions/{id}/resume` — `multipart/form-data`
Field name **`file`**, PDF only, ≤ 10 MB.
```json
{ "gcsUri": "gs://crucible-hack-0725-media/resumes/uid/sessionId.pdf" }
```
Errors: `400 not_pdf`, `413 file_too_large`.

#### `POST /v1/sessions/{id}/jd`
```json
{ "text": "ML Engineer, mid-level. Must have: streaming pipelines..." }
```
≤ 20,000 chars. Errors: `400 missing_text`, `400 jd_too_long`.

#### `POST /v1/sessions/{id}/digest` — **no body, takes 15–20 s**
This is the ingestion call. Show a real progress experience, not a spinner —
"Reading your resume…", "Finding claims to probe…", "Building your interview
plan…". The PRD's own budget was 4–8 s; measured reality is **15–20 s**, so this
screen needs to be genuinely interesting or it will feel broken.

```jsonc
{
  "digest": {
    "candidate": {
      "seniority_estimate": "entry|junior|mid|senior",
      "primary_stack": ["Python", "Go", "PyTorch"],
      "gaps_vs_jd": ["No evidence of distributed training experience."],
      "claims": [{
        "id": "c1",
        "text": "Built an async Python ingestion proxy processing ~2000 req/s",
        "artifact": "DataMesh - Streaming feature pipeline",
        "verifiable_depth": "high|medium|low",
        "probe_angles": [
          "How were false positives in the bloom filter handled?",
          "How was the 2000 requests per second figure measured?"
        ]
      }]
    },
    "role": {
      "title": "ML Engineer",
      "must_haves": ["..."], "nice_to_haves": ["..."],
      "implied_seniority": "mid",
      "domain_areas": ["Streaming Data Pipelines", "Feature Stores", "..."]
    },
    "interview_plan": [{
      "area": "Streaming Feature Pipelines",
      "why": "JD demands it; resume claims it; depth unverified",
      "opening_question_seed": "In your DataMesh project, ...",
      "target_band": 4,
      "dropped": false          // set by PATCH /plan
    }]
  },
  "meta": { "model": "gemini-3.6-flash", "promptVersion": "822de17a",
            "durationMs": 16908, "claims": 4, "planAreas": 5 }
}
```
Error `422 empty_digest` — the resume was probably an image scan. Show the
message the backend returns; it tells the user what to do.

**The digest reveal is a demo beat.** Show the extracted claims with their
`probe_angles`. "It has already found three claims I would struggle to defend"
is a strong line, and the probe angles are genuinely uncomfortable to read.

#### `PATCH /v1/sessions/{id}/plan`
```json
{ "droppedAreas": ["Batch Processing & Production Operations"] }
```
→ `{ "remainingAreas": 3 }`. Errors: `400 empty_plan` (must keep at least one).

Areas are **marked** `dropped: true`, not removed, so unchecking restores them.
Render as a checklist. This converts the tool from something happening *to* the
user into something they configured.

### 6.4 Post-session

#### `GET /v1/sessions/{id}/report`
**`202` while generating**, `200` when ready. Poll every 3 s.
```json
{ "status": "generating", "sessionStatus": "evaluating" }
```
`status` is `"not_started"` if the session was never ended — do not poll forever
on that.

Ready shape → §9.5.

#### `GET /v1/sessions/{id}/turns`
```json
{ "turns": [ /* full Turn objects with evaluation + delivery embedded */ ] }
```
This is what the per-turn accordion renders from. One call gets everything.

#### `GET /v1/sessions/{id}/roadmap`
Same 202-polling contract. Ready shape → §9.6. Generated **after** the report,
so it is legitimately later — the user reads the report while it builds.

#### `POST /v1/sessions/{id}/retest` — no body
```json
{ "sessionId": "new-id", "persona": "tech_lead", "band": 4,
  "focusAreas": ["buffer management", "queue level backpressure"] }
```
Creates a **new** session inheriting the digest, JD, and resume — no re-upload —
starting one band higher. This is the button that closes the loop. It consumes a
daily allocation, so it can return `429`.

### 6.5 Study Mode

#### `POST /v1/sessions/{id}/syllabus`
```json
{ "depth": "survey" | "exam_ready" | "interview_ready", "syllabusText": "optional paste" }
```
Takes ~15 s. Returns `{ "syllabus": Syllabus, "mastery": Stats }` → §9.7.

#### `GET /v1/sessions/{id}/study/next`
```json
{ "complete": false, "subtopicId": "s1", "subtopic": "Query, Key, and Value representations",
  "archetype": "recall", "archetypeLabel": "Recall",
  "question": "State the exact matrix operations used to derive Q, K, and V.",
  "mastery": { "total": 8, "unseen": 7, "attempted": 0, "shaky": 1, "solid": 0 }, "band": 3 }
```
When finished: `{ "complete": true, "mastery": {...}, "message": "..." }`

#### `POST /v1/sessions/{id}/study/answer`
```json
{ "subtopicId": "s1", "question": "<the question you were given>", "answer": "..." }
```
→
```json
{ "evaluation": { /* same Evaluation shape as an interview turn */ },
  "masteryFrom": "unseen", "masteryTo": "shaky", "passed": true,
  "nextArchetype": "application", "unlocked": ["s2"],
  "mastery": {...}, "complete": false }
```
**`unlocked` is a moment.** When answering s1 unlocks s2, animate the edge
lighting up in the dependency graph.

#### `GET /v1/sessions/{id}/mastery`
```json
{ "topic": "...", "subtopics": Subtopic[], "mastery": Stats, "complete": false }
```

### 6.6 Health

`GET /health` → `{"status":"ok","version":"dev"}` — unauthenticated, cheap.
`GET /readyz` → performs a real Vertex call; use it for a status indicator, not
a poll loop.

---

## 7. WebSocket protocol — complete reference

**Endpoint:** `wss://<host>/v1/sessions/{id}/live?token=<firebase-id-token>`

Optional `&voice=<name>` overrides the persona's voice — for A/B only, do not
expose it in the product UI.

### 7.1 The connection lifecycle, in order

```
1. POST /v1/sessions            -> sessionId
2. (interview) upload resume, JD, run digest, pick persona, edit plan
3. Open WebSocket with the token in the query string
4. RECEIVE  {"type":"state","state":"CONNECTING"}
5. RECEIVE  {"type":"state","state":"LISTENING"}   <- NOW you may send
6. SEND     {"type":"begin"}                       <- once audio playback is ready
7. RECEIVE  audio frames + {"type":"transcript","side":"ai",...}
8. SEND     {"type":"activity_start"}              <- mic hot
9. SEND     binary PCM16 frames, paced at real time
10. SEND    {"type":"activity_end"}                <- user clicked Done
11. RECEIVE {"type":"transcript","side":"user","final":true,...}
12. RECEIVE audio + transcript for the next question
13. RECEIVE {"type":"evaluation","turnId":...,"payload":{...}}   <- 5-8s later
14. RECEIVE {"type":"band","from":3,"to":4,...}                  <- sometimes
15. loop from 8
16. SEND    {"type":"end_session"} then close
```

**Step 6 is not optional.** In manual activity mode the model never speaks
unprompted — it waits for a turn boundary that, at session start, has not
happened. Without `begin` the room sits silent forever. Send it once your
AudioContext is running and the playback worklet is loaded, so the opening
question is never spoken into a page that cannot play it.

### 7.2 Client → Server

| Frame | Format | Payload | Notes |
|---|---|---|---|
| Audio | **Binary** | Raw PCM16, 16 kHz, mono, 20 ms frames (640 bytes) | Only between `activity_start` and `activity_end` |
| `begin` | Text JSON | `{"type":"begin"}` | Once, when playback is ready |
| `activity_start` | Text JSON | `{"type":"activity_start"}` | Mic goes hot |
| `activity_end` | Text JSON | `{"type":"activity_end"}` | User clicked Done. **Starts the latency clock** |
| `text_answer` | Text JSON | `{"type":"text_answer","text":"..."}` | Accessibility path and demo safety net |
| `request_hint` | Text JSON | `{"type":"request_hint"}` | Max 2 per turn |
| `end_session` | Text JSON | `{"type":"end_session"}` | Clean close |
| `ping` | Text JSON | `{"type":"ping","t":1234567890}` | **Every 20 s.** Cloud Run closes idle connections |

**Send the ping.** A demo that dies during a thoughtful pause is a demo that dies.

### 7.3 Server → Client

Binary frames are audio: **4-byte big-endian sequence number**, then PCM16 at
**24 kHz** mono.

```ts
function decodeAudioFrame(buf: ArrayBuffer): { seq: number; pcm: Int16Array } {
  const view = new DataView(buf)
  const seq  = view.getUint32(0, false)          // big-endian
  return { seq, pcm: new Int16Array(buf.slice(4)) }
}
```

Track sequence gaps. A gap is your earliest warning of network trouble, well
before it is audible.

Text frames are JSON with a `type` discriminator:

| `type` | Fields | Meaning |
|---|---|---|
| `state` | `state`, `turnIndex` | Lifecycle: `CONNECTING`, `ASKING`, `LISTENING`, `CLOSING`, `EVALUATING`, `SETTLED`, `ERROR` |
| `transcript` | `side`, `text`, `final` | `side`: `"user"` \| `"ai"`. **`text` is a DELTA — append it, do not replace** |
| `evaluation` | `turnId`, `payload` | The graded turn. Arrives 5–8 s after the answer |
| `ungraded` | `turnId`, `message` | Could not grade. Say so inline; do not leave a shimmer spinning |
| `band` | `from`, `to`, `message`, `text` | Difficulty changed. `message` is the toast copy |
| `hint` | `text`, `penalty` | Socratic hint, `penalty` is 0.5 |
| `usage` | `totalTokens`, `audioTokensIn`, `audioTokensOut` | Running cost |
| `turn_complete` | — | Model finished speaking |
| `interrupted` | — | **Discard queued playback immediately** |
| `error` | `code`, `recoverable`, `message` | See §7.5 |
| `pong` | `t` | Keepalive reply |

### 7.4 Transcript frames are deltas

The most common integration bug. `text` is an incremental chunk, not the full
transcript.

```ts
// WRONG — will show only the last word
setTranscript(frame.text)

// RIGHT
setTranscript(prev => prev + frame.text)
```

`final: false` means an interim update — render it at reduced opacity and let it
stabilise. `final: true` is confirmed text.

> **Note from the backend build:** `InterimInputTranscription` did not fire in
> manual activity mode during testing — only final transcriptions arrived. The
> relay forwards interim frames if they appear, but **do not make the
> reduced-opacity effect load-bearing.** Design so the transcript looks right
> with final frames only, and treat interim as an enhancement.

### 7.5 Error codes

| `code` | `recoverable` | What to do |
|---|---|---|
| `live_connect_failed` | false | Vertex unreachable. Offer retry, or switch to text mode |
| `live_stream_lost` | true | Connection dropped. See below |
| `live_going_away` | true | Server about to hang up |
| `idle_timeout` | true | 90 s of no activity. Session is resumable — offer to reconnect |
| `hint_limit` | true | Both hints used this turn |
| `hint_failed` | true | Hint generation failed; keep going |

**The reconnect path is not implemented on the backend.** Session-resumption
handles are emitted but nothing consumes them, so a dropped socket ends the
session. The frontend must degrade honestly: show "Connection lost", offer
**"Continue in text mode"** (which works — `text_answer` goes through the same
path) and **"End and see my report"** (the turns already graded are persisted).
Do not pretend to reconnect.

---

## 8. Replay Mode — your demo insurance

```ts
await createSession({ mode: 'replay', fixtureId: 'demo-ml-engineer' })
```

Then connect the WebSocket **exactly as normal**. The backend serves a recorded
27-second session over the identical protocol with **zero Vertex calls**. Same
frame types, same sequence numbers, same timings.

**The frontend needs no replay-specific code at all.** That is the whole point.
If your Live Room works live, it works in replay.

Verified on the deployed service: 27,294 ms of audio, 0 sequence gaps, 0 tokens.

Put a discreet way to launch it — a keyboard shortcut, or a "Demo" item in a
menu. On stage, if the venue wifi is bad, this is what saves you. Test it in
rehearsal so the path is warm.

---

## 9. TypeScript types — paste these in

### 9.1 Enumerations
```ts
export type Mode      = 'interview' | 'study' | 'replay'
export type Persona   = 'tech_lead' | 'architect' | 'pm'
export type Verdict   = 'validated' | 'incomplete' | 'unsupported' | 'incorrect'
export type Mastery   = 'unseen' | 'attempted' | 'shaky' | 'solid'
export type Archetype = 'recall' | 'application' | 'edge_case' | 'teach_back'
export type LiveState = 'CONNECTING' | 'ASKING' | 'LISTENING' | 'CLOSING'
                      | 'EVALUATING' | 'SETTLED' | 'ERROR'
export type SessionStatus = 'configuring' | 'live' | 'evaluating' | 'complete' | 'abandoned'
```

### 9.2 Session
```ts
export interface Session {
  id: string; uid: string; mode: Mode; status: SessionStatus
  persona?: Persona; topic?: string
  createdAt: string; startedAt?: string; endedAt?: string; durationMs: number
  difficultyBand: number
  bandHistory: { turnIndex: number; band: number; reason: string; at: string }[]
  coverage: { proven: string[]; shaky: string[]; missing: string[] }
  digest?: Record<string, any>          // untyped; see §6.3 for its shape
  jdText?: string; resumeGcsUri?: string
  turnCount: number
  costEstimate: {
    totalTokens: number; promptAudioTokens: number; responseAudioTokens: number
    promptTextTokens: number; responseTextTokens: number; calls: number
  }
  fixtureId?: string
}
```

### 9.3 Evaluation — note the snake_case
The evaluation mirrors the model's schema, so several fields are snake_case
while the surrounding session object is camelCase. This is deliberate and you
must match it exactly.
```ts
export interface Span {
  excerpt: string          // the transcript's own wording at [start,end)
  verdict: Verdict
  concept: string
  explanation: string
  correction?: string      // present for incorrect and incomplete
  confidence: number       // 0..1
  start: number            // BYTE offset — see §9.4
  end: number
}

export interface Scores {
  technical_accuracy: number       // 1..10
  communication_clarity: number
  depth: number
  structure: number
}

export interface Evaluation {
  turn_id: string
  question_intent: string
  scores: Scores
  verdict_summary: string
  spans: Span[]
  concepts_demonstrated: string[]
  concepts_missing: string[]
  ideal_answer_outline: string[]
  followup_probe: string
  difficulty_recommendation: 'raise' | 'hold' | 'lower'
  turnScore: number                // persona-weighted, 0..10, hint penalty applied
  spansDropped: number
  redsDowngraded: number
  promptVersion?: string
  model?: string
  gradedAt: string
  durationMs: number
}
```

### 9.4 ⚠️ Byte offsets → JavaScript indices

**You must convert `span.start` / `span.end` before slicing the transcript.**

```ts
/**
 * Go sends BYTE offsets into a UTF-8 string. JavaScript strings are UTF-16.
 * These agree for ASCII and diverge the moment the transcript contains an
 * accented character, an em dash, a curly quote, or an emoji — after which
 * every highlight is misplaced.
 *
 * Build the map once per transcript, not once per span.
 */
export function byteToCharMap(text: string): number[] {
  const map: number[] = []          // map[byteOffset] = charIndex
  const encoder = new TextEncoder()
  let byte = 0
  for (let ch = 0; ch < text.length; ch++) {
    // Surrogate pairs must be measured as one code point.
    const cp = text.codePointAt(ch)!
    const str = String.fromCodePoint(cp)
    const len = encoder.encode(str).length
    for (let b = 0; b < len; b++) map[byte + b] = ch
    byte += len
    if (cp > 0xffff) ch++           // skip the low surrogate
  }
  map[byte] = text.length           // one past the end
  return map
}

export function sliceByBytes(text: string, map: number[], start: number, end: number) {
  return text.slice(map[start] ?? 0, map[end] ?? text.length)
}
```

**Test this with a transcript containing "naïve" and an em dash.** If you skip
it, the bug will not appear until a real user says something the transcriber
renders with a non-ASCII character — quite likely mid-demo.

### 9.5 Report
```ts
export interface Report {
  sessionId: string; status: 'generating' | 'ready' | 'failed'
  aggregateScores: Scores
  overallScore: number
  domainScores: { domain: string; score: number; turnCount: number }[]  // radar axes
  bandTrajectory: number[]; startBand: number; endBand: number          // sparkline
  strengths: string[]      // max 6
  gaps: string[]           // max 5 — do not render more
  turns: {
    turnId: string; index: number; question: string; score: number
    hintsUsed: number; band: number; graded: boolean
    spanCounts: Record<Verdict, number>
  }[]
  delivery: {
    wpm: number; paceBand: 'hesitant' | 'optimal' | 'rushed' | 'too fast'
    fillerTotal: number; fillerPerMinute: number
    speakingTimeMs: number; hesitationScore: number
    observation?: string; drill?: string; turnsWithAudio: number
  }
  turnsGraded: number; durationMs: number; generatedAt: string
}
```

### 9.6 Roadmap
```ts
export interface Roadmap {
  session_id: string; horizon_days: number; summary: string
  days: {
    day: number; focus_concept: string; why_this_matters: string
    estimated_minutes: number
    resources: { title: string; url: string; type: string; minutes: number; verified: boolean }[]
    practice_task: string; self_check: string
  }[]
  retest_plan: {
    after_day: number; focus_areas: string[]
    recommended_persona: Persona; recommended_band: number
  }
  grounded: boolean; note?: string
  linksFound: number; linksDropped: number
  generatedAt: string
}
```
**Every resource has already been fetched and verified server-side.** Dead links
are dropped before you see them, so `verified` is always `true` on what arrives.
Link out with `target="_blank" rel="noopener noreferrer"`. If `grounded` is
false, show `note` — the plan is still useful without links.

### 9.7 Study
```ts
export interface Subtopic {
  id: string; name: string; prereqs: string[]; depth: number; why: string
  mastery: Mastery; archetype: Archetype; attempts: number; teachBackPassed: boolean
}
export interface Syllabus {
  topic: string; depth: 'survey' | 'exam_ready' | 'interview_ready'
  subtopics: Subtopic[]; createdAt: string
}
export interface MasteryStats {
  total: number; unseen: number; attempted: number; shaky: number; solid: number
}
```

---

# PART III — THE AUDIO PIPELINE

This is the hardest part of the frontend and the part most likely to eat a day.
Budget accordingly and build it first, behind everything else.

## 10. Capture

```
getUserMedia({ audio: {
  channelCount: 1,
  echoCancellation: true,     // ESSENTIAL — the AI's voice is on your speakers
  noiseSuppression: true,
  autoGainControl: true,
}})
  → AudioContext({ sampleRate: 16000 })
  → AudioWorkletNode (NOT the deprecated ScriptProcessor)
  → Float32 → Int16 PCM
  → 320-sample frames (20 ms @ 16 kHz = 640 bytes)
  → ws.send(arrayBuffer)
```

**Echo cancellation is not optional.** Without it the AI hears itself through
your speakers, interprets it as user speech, and the session degrades. Use
headphones for the demo regardless.

```js
// public/worklets/capture.js
class CaptureProcessor extends AudioWorkletProcessor {
  constructor() { super(); this.buf = new Int16Array(320); this.n = 0 }
  process(inputs) {
    const ch = inputs[0]?.[0]
    if (!ch) return true
    for (let i = 0; i < ch.length; i++) {
      // Clamp before scaling: values outside [-1,1] wrap catastrophically.
      const s = Math.max(-1, Math.min(1, ch[i]))
      this.buf[this.n++] = s < 0 ? s * 0x8000 : s * 0x7fff
      if (this.n === 320) {
        this.port.postMessage(this.buf.slice()); this.n = 0
      }
    }
    return true
  }
}
registerProcessor('capture', CaptureProcessor)
```

**Cost optimisation worth doing:** compute RMS per frame and skip sending frames
below a silence threshold. Live audio is the single largest cost in the system
and a 10-minute session is mostly silence. The backend's manual activity mode
exists partly to make this safe.

## 11. Playback

Output is **24 kHz** — note the asymmetry with 16 kHz input. Do not resample one
to match the other by accident.

**Do not create an `AudioBufferSourceNode` per chunk.** You will get an audible
click at every boundary. Use a ring buffer consumed by a playback worklet.

```js
// public/worklets/playback.js — ring buffer, no allocation in process()
class PlaybackProcessor extends AudioWorkletProcessor {
  constructor() {
    super()
    this.ring = new Float32Array(24000 * 10)   // 10 s
    this.r = 0; this.w = 0; this.underruns = 0
    this.port.onmessage = (e) => {
      if (e.data === 'flush') { this.r = this.w = 0; return }  // on interruption
      const pcm = e.data
      for (let i = 0; i < pcm.length; i++) {
        this.ring[this.w] = pcm[i] / 32768
        this.w = (this.w + 1) % this.ring.length
      }
    }
  }
  process(_, outputs) {
    const out = outputs[0][0]
    for (let i = 0; i < out.length; i++) {
      if (this.r === this.w) { out[i] = 0; this.underruns++ }
      else { out[i] = this.ring[this.r]; this.r = (this.r + 1) % this.ring.length }
    }
    return true
  }
}
registerProcessor('playback', PlaybackProcessor)
```

Keep **2–3 chunks of jitter buffer**. More adds perceptible latency; less
produces dropouts on flaky wifi. **Track underruns and surface them in a dev
overlay** — it is the best early warning of a network problem before the demo.

**On `interrupted`: flush the ring buffer immediately.** A model that keeps
talking for two seconds after being interrupted feels broken.

## 12. The visualiser

Drive it from **actual output amplitude** — RMS over each PCM chunk — not a
decorative loop. Judges notice the difference.

Idle state must be a **slow breathing pulse, never static**. A frozen visualiser
reads as a crashed app.

---

# PART IV — SCREENS

## 13. Screen map

```
/                     Landing + sign-in
/setup                Resume upload, JD paste
/setup/digest         Digest reveal — claims and probe angles
/setup/persona        Three persona cards
/setup/plan           Interview plan checklist
/room/:id             THE LIVE ROOM
/report/:id           Report — radar, sparkline, per-turn accordion, delivery
/roadmap/:id          Day-by-day plan
/study/:id            Study Mode drill loop + dependency graph
/history              Past sessions
```

## 14. The Live Room — the product

```
┌──────────────────────────────────────────────────────────────────────┐
│  CRUCIBLE          ML Engineer · Tech Lead        Band 3/5   09:42 ⏱ │
├────────────────────────────────┬─────────────────────────────────────┤
│                                │   Q3 · Feature pipeline design      │
│      ╭──────────────╮          │   ─────────────────────────────     │
│      │   ((( • )))  │          │                                     │
│      ╰──────────────╯          │   So the ingestion layer used a     │
│         THE TECH LEAD          │   Kafka topic per source, and we    │
│                                │   deduplicated downstream using     │
│  "Walk me through how you      │   a bloom filter before the         │
│   handled backpressure in      │   feature store write...            │
│   that proxy layer."           │                                     │
│                                │   ▌                                 │
│  ┌──────────────────────────┐  │                                     │
│  │  ◈ Request a hint        │  │   ─────────────────────────────     │
│  │  ⏎ I'm done answering    │  │   ▮▮▮▮▮▮▮▯▯▯  148 wpm              │
│  │  ⌨ Type instead          │  │                                     │
│  │  ⏹ End interview         │  │                                     │
│  └──────────────────────────┘  │                                     │
├────────────────────────────────┴─────────────────────────────────────┤
│  Turn 3 of ~6      ● Listening                     Hints used: 1     │
└──────────────────────────────────────────────────────────────────────┘
```

**Left — the interviewer.** Persona card with name and current band. The
amplitude-driven visualiser. The question as **text** (source it from the `ai`
transcript stream — in a noisy demo hall nobody can hear the audio). Controls.

**Right — the candidate.** The transcript is the emotional centre of the
product: the user watching their own thoughts appear. **Large type, generous
line height. Do not make it a cramped chat bubble.** Below it, a live pace bar —
deterministic WPM over a rolling 15-second window, computed client-side from
word count over elapsed audio time. No AI call needed.

### 14.1 State machine — every state needs a distinct visual signature

| State | Visualiser | Transcript | Mic | Controls |
|---|---|---|---|---|
| `CONNECTING` | slow grey pulse | skeleton | off | all disabled |
| `ASKING` | active, amplitude-driven | dimmed, empty | muted | Done + Hint disabled |
| `LISTENING` | soft idle breathing | live, cursor visible | **hot** | Hint + Done enabled |
| `CLOSING` | active | frozen, final text | muted | disabled |
| `EVALUATING` | idle | shimmer overlay | off | disabled |
| `SETTLED` | idle | **heatmap revealed** | off | Next enabled |
| `ERROR` | red ring | error card + retry | off | Retry, End |

**The most common failure in a voice UI is that the user cannot tell whether the
system is listening, thinking, or dead.** Make each of these unmistakable.

### 14.2 The heatmap reveal

Because grading is post-turn, you get a genuine moment. The transcript sits in
plain text for a beat, then the spans illuminate **in sequence, left to right,
over roughly 600 ms**. Stagger the animation.

| Verdict | Colour | Icon | Meaning |
|---|---|---|---|
| `validated` | green | ✓ | Correct and substantive |
| `incomplete` | amber | ~ | Directionally right, thin |
| `unsupported` | blue | ? | Asserted without basis |
| `incorrect` | red | ! | Factually wrong |

**Never encode a verdict by colour alone — pair each with its icon.** Contrast
ratios must meet WCAG AA. Roughly 8% of men have some colour vision deficiency,
and red/green is the worst possible pair to rely on.

Hover or tap a span → popover with the concept, the explanation, and — for
`incorrect` and `incomplete` — the correction. **Keep the popover under forty
words.** Anything longer belongs in the report.

### 14.3 Making adaptation visible

On a `band` frame:
1. Animate the band indicator in the header
2. Toast with `message` — *"Difficulty raised — you've proven the fundamentals."*
3. The next question will audibly change register

## 15. Report screen

- **Radar chart** across `domainScores`. Recharts `RadarChart`. This is the
  visual the problem statement is implicitly asking for.
  ⚠️ **With fewer than three graded turns the radar is flat** — every axis shows
  the same score, because an unattributable turn is spread across all of them.
  Handle this: show a "complete more turns" state rather than a shape that looks
  broken.
- **Band sparkline** from `bandTrajectory`.
- **Per-turn accordion**: question, the answer with heatmap intact, four scores,
  verdict summary, ideal answer outline, hints used.
- **Delivery panel**: WPM dial with the optimal band shaded, filler count,
  hesitation, the observation and its drill.
  ⚠️ **`fillerPerMinute` overstates on short answers** — 51.9/min extrapolated
  from a 15-second answer is arithmetically correct and misleading. **Lead with
  the raw count**; show the rate as secondary.
- **Two lists side by side**: "You proved" and "You need to close". The backend
  caps gaps at five. Do not render more even if you receive more.

## 16. Roadmap screen

Day-by-day cards. Each: focus concept, why it matters, estimated minutes,
verified resource links, practice task, self-check.

Footer: **"Retest on day N"** button → `POST /retest` → navigate straight into
the new session's persona screen. This is the loop closing.

## 17. Study Mode

**Dependency graph render.** `subtopics[]` with `prereqs[]` is a real DAG — use
`depth` for layout rows. Colour nodes by mastery. This is cheap to render and
looks considered.

The drill loop: fetch `study/next`, show the archetype badge prominently
(**Recall / Application / Edge case / Teach it back**), take a typed answer, POST
it, show the evaluation with the same heatmap component as the interview.

**Teach-back deserves special treatment.** It is the highest-signal question type
in the product and the only path to `solid`. Give it a distinct visual weight,
and when `masteryTo` becomes `solid`, celebrate it.

When `unlocked` is non-empty, animate those graph edges lighting up.

---

# PART V — VISUAL SYSTEM

## 18. Direction

**Invoke `anthropic-skills:frontend-design` before writing components.**

The register to aim for: a serious instrument, not a consumer app. This is a
tool that tells people uncomfortable truths about their own competence. It
should feel like a well-made piece of professional software — closer to a
cockpit or a studio tool than to a learning app with rounded corners and
illustrations.

Suggested direction, to adapt rather than follow literally:

- **Dark by default.** The Live Room is the product and it should feel like a
  room. Offer light for the report and roadmap, which are read documents.
- **One accent colour**, used sparingly, plus the four verdict colours which are
  functional rather than decorative and must not compete with it.
- **Typography does the work.** A confident text face at generous size for the
  transcript; a tight, technical face for metrics and labels.
- **Motion is diegetic.** The visualiser moves because there is sound. The
  heatmap reveals because grading finished. Nothing animates for decoration.

**Avoid:** default shadcn card grids, purple gradients, glassmorphism, emoji as
iconography, and any layout that reads as a generic AI dashboard.

## 19. Charts

**Invoke the `dataviz` skill before the radar, sparkline, or pace dial.** Three
visualisations on one screen must read as one system. That skill provides the
palette formula, accessibility validation, and mark specs.

## 20. Assets to obtain

Fetch or generate these; do not hand-wave them.

| Asset | Source | Notes |
|---|---|---|
| **Fonts** | Google Fonts, self-hosted | Suggested: *Inter* or *Geist* for UI, *Newsreader* or *Source Serif* for the transcript, *JetBrains Mono* for metrics. **Self-host via `@fontsource/*`** — do not hotlink, and subset to latin |
| **Icons** | `lucide-react` | Consistent, tree-shakeable, no emoji |
| **Charts** | `recharts` | Radar and sparkline |
| **Graph layout** | `d3-hierarchy` or `elkjs` | Study Mode DAG. `depth` gives you rows; you need x-positioning within a row |
| **Audio worklets** | Write them (§10, §11) | No library does this well; the code above is complete |
| **Persona avatars** | Generate | Abstract geometric marks, one per persona, distinguishable at 64 px. Do **not** use stock photos of people — an illustrated or generated face implies a specific human and is a claim you do not want to make |
| **Favicon** | Generate | The Crucible mark |
| **Demo resume PDF** | `backend/testdata/make_resume.py` | Regenerate the same fixture the backend tests use, so demo and tests agree |
| **Sample JD text** | Write one | Keep it in the repo so the demo never depends on the clipboard |

**Licensing:** everything above is OFL or MIT. Do not pull in an asset you
cannot license for a public repo.

---

# PART VI — EXECUTION

## 21. Stack

React 19 + TypeScript + Vite. Tailwind for layout. Zustand for session state —
the state machine in §14.1 wants a single store and Redux is overkill.
`recharts` for charts. **No component library** — the Live Room is custom
anyway, and shadcn-flavoured defaults are exactly the generic look to avoid.

## 22. Build order — riskiest first

**Phase F0 — Skeleton (1 h).** Vite + TS + Tailwind + router + Firebase Auth.
Exit: you can sign in and `GET /v1/me` returns your uid.

**Phase F1 — The audio spike (3 h). ⚠️ HIGHEST RISK. DO THIS FIRST.**
No UI beyond two buttons. Capture worklet → WebSocket → playback worklet.
Connect to a **replay session** so you burn no credits while debugging.
Exit: you hear the recorded interviewer speak, and the transcript prints to the
console. **If this is not working in three hours, stop and reassess** — the rest
of the product assumes it.

**Phase F2 — The Live Room (4 h).** The full two-panel layout, the state machine
with all seven visual signatures, the visualiser, streaming transcript, controls.
Exit: a real interview from `begin` to `end_session`.

**Phase F3 — Heatmap + adaptation (3 h).** Span rendering with byte-offset
conversion, the staggered reveal, popovers, band toast, hint display.
Exit: an evaluation frame produces a correct, beautiful heatmap.

**Phase F4 — Setup flow (3 h).** Upload, JD, digest reveal with probe angles,
persona cards, plan checklist.
Exit: cold start → resume → digest → persona → room.

### ═══ CUT LINE ═══

**Phase F5 — Report (3 h).** Radar, sparkline, accordion, delivery panel.
**Phase F6 — Roadmap (2 h).** Day cards, verified links, retest button.
**Phase F7 — Study Mode (3 h).** Dependency graph, drill loop, mastery map.
**Phase F8 — Polish.** Error and empty states everywhere. Loading states that
explain what is happening ("Reading your resume…" beats a spinner). Mobile pass
on the report at minimum. Rehearse five times. Record a backup video.

## 23. Developing without burning credits

**Use replay sessions for everything except final verification.** A replay
session exercises the entire protocol at zero cost:

```ts
const s = await createSession({ mode: 'replay', fixtureId: 'demo-ml-engineer' })
```

**Run the backend locally** for faster iteration:
```bash
cd backend && make run           # http://localhost:8080
```
Point `VITE_API_BASE` at it. Note the local server needs `secrets/key.json`.

**`cmd/wsprobe` is a working reference client.** When a frame confuses you, run
it and compare — it prints every frame it receives:
```bash
cd backend && go run ./cmd/wsprobe -session <id> -token <token> -wait 30s
```

## 24. Testing

- **Byte-offset conversion** — unit test with "naïve", an em dash, and an emoji.
  This is the highest-value test in the frontend.
- **Transcript delta accumulation** — assert appending, not replacing.
- **Audio framing** — 320 samples, 640 bytes, correct Int16 conversion at the
  clamp boundaries.
- **Sequence gap detection** — feed out-of-order frames.
- **State machine** — every transition in §14.1 produces the documented visuals.
- **Manual:** the demo script, five times, same answers, same timings.

## 25. Risk register

| # | Risk | Mitigation |
|---|---|---|
| F1 | AudioWorklet fights you | Spike first, before any UI. Replay sessions make iteration free |
| F2 | Echo — AI hears itself | `echoCancellation: true`, headphones, always |
| F3 | Highlights land mid-word | §9.4 byte-offset conversion, tested with non-ASCII |
| F4 | Transcript shows only the last word | §7.4 — deltas, append |
| F5 | Room sits silent forever | Send `begin` (§7.1 step 6) |
| F6 | 800 ms of phantom latency | Wait for `LISTENING` before sending audio |
| F7 | Playback clicks at chunk boundaries | Ring buffer + worklet, never per-chunk buffer sources |
| F8 | Venue wifi | Replay mode, rehearsed and warm |
| F9 | Radar looks broken | It is flat under three turns; handle that state explicitly |
| F10 | Connection drops mid-answer | No reconnect exists — degrade to text mode honestly |

## 26. Demo script — what the frontend must support

| Time | Screen | Requirement |
|---|---|---|
| 0:00 | Landing | Sign-in in one click |
| 0:10 | Setup | Upload + paste, both fast |
| 0:20 | Digest | Claims and probe angles legible from the back of a room |
| 0:30 | Persona | Three cards, `punishes` prominent |
| 0:40 | Live Room | **Audio plays. Question renders. Visualiser moves.** |
| 0:50 | Live Room | Transcript streams visibly |
| 1:10 | Live Room | Hint appears with its penalty |
| 1:25 | Live Room | **Heatmap reveals, staggered. Hover shows the popover** |
| 1:45 | Live Room | **Band indicator animates, toast appears** |
| 2:10 | Report | Radar renders |
| 2:25 | Report | WPM and filler count |
| 2:40 | Roadmap | Day cards, a link that opens a real page |
| 2:55 | Roadmap | Retest button navigates into a new session |

---

## Appendix A — Definition of done

- [ ] Sign in with Google **and** anonymously
- [ ] Upload a resume, paste a JD, see a digest with probe angles
- [ ] Pick a persona, drop a plan area, see the interview change
- [ ] Hold a spoken conversation with audible replies
- [ ] Transcript streams under 400 ms behind speech
- [ ] Heatmap reveals with four verdict colours, each paired with an icon
- [ ] Highlights land on word boundaries with a non-ASCII transcript
- [ ] Hint arrives and shows its penalty
- [ ] Band change animates and toasts
- [ ] Type-instead works as a full alternative path
- [ ] End → report with radar, sparkline, accordion, delivery
- [ ] Roadmap with links that open real pages
- [ ] Retest creates a pre-configured session
- [ ] Study Mode: syllabus graph, drill loop, mastery map
- [ ] Replay mode runs the whole demo with no network dependency
- [ ] Every screen has an error state and an empty state
- [ ] Zero console errors during a full session
- [ ] Runs at 1280×800

## Appendix B — Where to look when something breaks

| Symptom | Look at |
|---|---|
| Room silent forever | Did you send `begin`? |
| First question feels slow | Are you sending audio before `LISTENING`? |
| Transcript shows one word | Deltas — append, do not replace |
| Highlights off by a few characters | Byte offsets (§9.4) |
| Playback clicks | Per-chunk `AudioBufferSourceNode` instead of a ring buffer |
| AI interrupts itself | Echo cancellation off, or no headphones |
| POST returns 411 | Missing `Content-Length: 0` |
| Health check 404s | You probed `/healthz` instead of `/health` |
| Evaluation never arrives | It takes 5–8 s. Check the `EVALUATING` state actually waits |
| 429 on session create | Daily cap, 5 per user. Use replay mode |

**The backend logs everything.** `cd backend && make logs` tails Cloud Run.
Search for your `session_id` — every line carries it.
