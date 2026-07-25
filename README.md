# Crucible

**An adaptive, voice-native AI interview and study coach, built entirely on Vertex AI.**

InnovaHack — Gen AI Problem Statement 2: Personalized AI Study / Interview Coach.

Most interview prep is a quiz generator with a chat window bolted on. Crucible is
a **live conversation**. You upload your resume and the job description you're
actually chasing, pick who's grilling you, and then you talk. The AI talks back
in its own voice, in real time, asking questions rooted in the projects on your
resume. When you finish an answer, your own words light up on screen: green
where you nailed it, amber where you were vague, blue where you claimed
something you couldn't support.

---

## Problem statement coverage

| Required capability | How Crucible satisfies it |
|---|---|
| Takes a topic **or** job role as input | Interview Mode ingests a resume PDF + job description. Study Mode decomposes a bare topic into a dependency-ordered syllabus |
| Generates relevant practice questions | Questions are generated live by an interviewer persona conditioned on the resume digest, the JD's requirements, the current difficulty band, and the concepts already proven |
| Evaluates **spoken** answers | Native-audio bidirectional streaming; speech transcribed by the Live model and graded span by span |
| Evaluates **written** answers | Typed answers and Study Mode share the identical evaluation path |
| Structured feedback: strengths | `concepts_demonstrated` aggregated into a radar chart across the role's domains |
| Structured feedback: gaps | `concepts_missing` plus a span-level heatmap with four verdicts |
| Suggested resources | Roadmap generated with Google Search grounding; **every URL is fetched and verified** before it is shown |
| Adapts difficulty | Five-band ladder with promotion and demotion rules, injected back into the live session so the *next* question genuinely changes |

## Architecture

```
BROWSER  ──WSS──►  CLOUD RUN (single Go service)  ──►  VERTEX AI
                   ├── httpapi    REST                 gemini-live-2.5-flash-native-audio
                   ├── live       WebSocket relay       gemini-3.6-flash
                   └── workers    evaluation pool       gemini-3.5-flash-lite
                          │
                   Firestore + Cloud Storage
```

**The relay is structurally mandatory, not an optimisation.** Vertex
authenticates with an OAuth2 bearer token minted from a service account, and
there is no safe way to put a service account key in frontend code — so every
audio frame passes through the backend.

### Design decisions worth knowing

- **Manual activity detection.** The client owns the turn boundary via explicit
  `activity_start` / `activity_end` signals rather than server-side VAD. This
  makes boundaries deterministic, structurally prevents the model from hearing
  its own voice, and lets the client skip transmitting silence — live audio is
  the dominant cost in the system.
- **Deadline-bounded injection.** A grade computed *after* a turn reaches the
  *next* question by being injected as context between them. That injection has
  a hard deadline: past it, a coach state built from deterministic data is used
  instead, so the interviewer never sits silent.
- **Server-side confidence gating.** An `incorrect` span below a configurable
  confidence threshold is rewritten to `unsupported`. Falsely flagging a correct
  answer red is the most damaging thing this product can do, and a prompt
  instruction alone is not a reliable defence.
- **Span anchoring, never character offsets.** The evaluator returns verbatim
  excerpts; a four-tier resolver locates them in the transcript server-side and
  silently drops what it cannot place. A missing highlight is invisible; a
  misplaced one is a visible bug.
- **Firestore is the source of truth.** In-memory session state is a cache. That
  is what makes reconnects, instance restarts, and idempotent finalization work
  without three separate mechanisms.
- **Replay Mode.** A recorded session is served over the *identical* WebSocket
  protocol with zero Vertex calls — indistinguishable from live, and immune to
  venue wifi.

## Layout

```
backend/
  cmd/
    server/       the one binary: REST + WebSocket relay + workers
    livespike/    standalone Vertex Live proof, kept as a smoke test
    wsprobe/      CLI stand-in for the browser; speaks the real protocol
    regionprobe/  which Vertex regions serve which models
  internal/
    live/         WebSocket relay, turn boundaries, replay
    evaluator/    span-level grading with confidence gating
    anchor/       four-tier span resolver
    difficulty/   band ladder and coverage sets (pure)
    grading/      turn sink, worker handlers, injection loop
    study/        syllabus decomposition, drill loop, mastery
    roadmap/      cluster, rank, ground, verify links
    report/       deterministic aggregation for the report screen
    delivery/     pace and disfluency from ANSWER AUDIO
    persona/      three interviewers with distinct rubrics and voices
    prompts/      every prompt, embedded and content-hashed
  docs/checkpoints/   what was built each phase, and what broke
```

## Running it

Requires a GCP project with Vertex AI, Firestore, and Cloud Storage enabled, and
a service account with `roles/aiplatform.user`, `roles/datastore.user`, and
`roles/storage.objectAdmin` scoped to one bucket.

```bash
cd backend
cp .env.example .env          # then fill in your project and bucket
# place a service account key at secrets/key.json (gitignored)

make run                      # start locally
make check                    # go vet + unit tests
make deploy                   # deploy to Cloud Run
```

Test the live interview without a frontend:

```bash
go run ./cmd/wsprobe -session <id> -token <firebase-id-token> -wav answer.wav
```

Load test and chaos pass:

```bash
bash deploy/loadtest.sh 10 10
bash deploy/chaos.sh
```

## Testing

```bash
go test ./...                                    # 125 unit tests
go test ./... -tags=integration -timeout=10m     # live Vertex; costs credits
```

The integration suite guards the acceptance criteria that matter most: an
excellent answer must produce **zero red spans**, a vague one must be marked
thin rather than wrong, unbacked numbers must read as `unsupported` rather than
`incorrect`, and a disfluent answer must produce a non-zero filler count.

## Status

All nine build phases complete. Deployed on Cloud Run with `min-instances=1`.

Known gaps are documented honestly in `backend/docs/checkpoints/phase-9.md` —
most notably the WebSocket reconnect path, which is not built: session
resumption handles are emitted but nothing consumes them yet.

## Credits

Model IDs, region pinning, and every measured latency figure in the checkpoints
were verified against a live Vertex project rather than taken from
documentation. Where the PRD's assumptions turned out to be wrong — model
availability, region co-location, and whether speech recognition strips
disfluencies — the checkpoints record what was actually measured.
