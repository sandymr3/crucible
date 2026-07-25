# 08 · Setup and deployment

> Evaluation criterion 7 — *Documentation & Submission Quality* (5 marks)
> Complete submission · readable README · deployment available · well-organized documentation

---

## Try it without installing anything

The service is live:

```bash
curl https://crucible-backend-103350253775.us-central1.run.app/health
```

```json
{"status":"ok","version":"dev"}
```

`/readyz` goes further — it performs a real one-token Vertex inference, so a 200
there means the model path works, not just that the process is up. It also reports
which credential source resolved (`application-default-credentials` on Cloud Run,
`service-account-key-file` locally).

> Use **`/health`**, not `/healthz`. Google's frontend intercepts `/healthz` on
> `*.run.app` and returns its own HTML 404 — the request never reaches the
> container. See [04-api-reference.md](04-api-reference.md).

---

## Prerequisites

- **Go 1.26.4+**
- **gcloud CLI**, authenticated
- A GCP project with billing enabled

### Enable the APIs

```bash
gcloud services enable \
  aiplatform.googleapis.com firestore.googleapis.com storage.googleapis.com \
  run.googleapis.com artifactregistry.googleapis.com cloudbuild.googleapis.com \
  identitytoolkit.googleapis.com
```

### Create the resources

```bash
# Firestore, Native mode, same region as the Live models
gcloud firestore databases create --location=us-central1

# Bucket with uniform access
gsutil mb -l us-central1 gs://<your-bucket>
gsutil uniformbucketlevelaccess set on gs://<your-bucket>

# Auto-delete interview audio after 7 days
gsutil lifecycle set backend/deploy/lifecycle.json gs://<your-bucket>
```

### Service account — exactly three grants, no more

```bash
gcloud iam service-accounts create crucible-backend

PROJECT=<your-project>
SA=crucible-backend@$PROJECT.iam.gserviceaccount.com

gcloud projects add-iam-policy-binding $PROJECT \
  --member=serviceAccount:$SA --role=roles/aiplatform.user
gcloud projects add-iam-policy-binding $PROJECT \
  --member=serviceAccount:$SA --role=roles/datastore.user

# Storage is scoped to the ONE bucket, not the project
gsutil iam ch serviceAccount:$SA:objectAdmin gs://<your-bucket>
```

Do not use the default compute service account, and do not grant Editor.

### Firebase Auth

Add Firebase to the project, initialise Identity Platform, and enable Google and
anonymous sign-in. Then deploy the Firestore rules — **these matter**, they are
what stops a client writing its own difficulty band:

```bash
gcloud firestore databases update --type=firestore-native
firebase deploy --only firestore:rules   # backend/deploy/firestore.rules
```

---

## Run it locally

```bash
cd backend
cp .env.example .env        # then fill in your project and bucket
```

Place a service-account key at `backend/secrets/key.json`.

> `secrets/` is gitignored **and** gcloudignored, so the key can reach neither
> GitHub nor Cloud Build. Verify before your first commit — this repository is
> public.

```bash
make run      # start on :8080
make check    # go vet + unit tests
make test     # unit tests only
```

| Target | Does |
|---|---|
| `make run` | Run the server locally |
| `make build` | Build the binary |
| `make test` | `go test ./...` — 125 test functions |
| `make test-int` | Live integration tests. **Costs credits.** |
| `make check` | `go vet` + unit tests |
| `make deploy` | Deploy to Cloud Run with every required flag |
| `make smoke` | Hit the deployed health endpoints |
| `make regionprobe` | Report which Vertex regions serve which models |
| `make logs` | Tail Cloud Run logs |

### Configuration

Everything is env-driven; no magic number lives outside `internal/config`.

```bash
GOOGLE_CLOUD_PROJECT=…
GCS_BUCKET=…
GOOGLE_APPLICATION_CREDENTIALS=./secrets/key.json   # local only — UNSET on Cloud Run

# The two model families are served from DIFFERENT locations and cannot be collapsed.
VERTEX_LIVE_LOCATION=us-central1        # Live; "global" closes with a policy violation
VERTEX_REASONING_LOCATION=global        # Gemini 3.x; us-central1 serves nothing past 2.5

MODEL_LIVE=gemini-live-2.5-flash-native-audio
MODEL_REASONING=gemini-3.6-flash
MODEL_CHEAP=gemini-3.5-flash-lite

SESSION_MAX_DURATION_SEC=720
SESSION_IDLE_TIMEOUT_SEC=90
MAX_HINTS_PER_TURN=2
MAX_CONCURRENT_SESSIONS=10
DAILY_SESSION_CAP_PER_USER=5
LIVE_ACTIVITY_MODE=manual
INJECTION_DEADLINE_MS=9000
EVAL_RED_CONFIDENCE_MIN=0.75
EVAL_MIN_WORDS=15
EVAL_THINKING_BUDGET=512
ROADMAP_HORIZON_DAYS=7
```

**One credential code path for both environments.** `credentials.DetectDefault`
reads `GOOGLE_APPLICATION_CREDENTIALS` when set and otherwise falls through to
Cloud Run's attached service account. Same binary, no branching. Leave the
variable unset in production.

---

## Deploy

```bash
cd backend
make deploy
```

Every flag is load-bearing:

```bash
gcloud run deploy crucible-backend \
  --region=us-central1 \
  --timeout=3600 \          # long-lived WebSockets
  --session-affinity \      # best-effort; correctness doesn't depend on it
  --min-instances=1 \       # no cold start on the first demo interaction
  --max-instances=5 \
  --cpu=2 --memory=2Gi \
  --no-cpu-throttling \     # REQUIRED: a throttled instance stops relaying audio
  --concurrency=20
```

**`DEV_ALLOW_ANON` must never be set in production.** The server refuses to start
on Cloud Run if it is — an unauthenticated public WebSocket in front of a billing
API only has to be found once.

### Verify the deployment

```bash
curl https://<service-url>/health   # {"status":"ok"}
curl https://<service-url>/readyz   # 200 only if a real Vertex call succeeds
bash deploy/chaos.sh                # 17 checks
bash deploy/loadtest.sh 10 10       # 10 concurrent sessions
```

---

## Exercise the live interview without a frontend

`cmd/wsprobe` speaks the real WebSocket protocol — streams a WAV at wall-clock
pace, prints every frame, writes received audio to a WAV.

```bash
# Mint a test ID token
curl -s -X POST "https://identitytoolkit.googleapis.com/v1/accounts:signUp?key=$WEB_API_KEY" \
  -H "Content-Type: application/json" -d '{"returnSecureToken":true}'

go run ./cmd/wsprobe -session <id> -token <id-token> -wav answer.wav
```

---

## Repository layout

```
backend/            the Go service — see 03-architecture.md
  cmd/              server, livespike, wsprobe, regionprobe
  internal/         24 packages
  deploy/           Dockerfile, deploy.sh, firestore.rules, lifecycle.json,
                    loadtest.sh, chaos.sh
  docs/checkpoints/ per-phase build records: what was built, what broke
frontend/           React + TypeScript + Vite (in progress, `feat/frontend`)
documentation/      this folder
  deck/             the pitch deck and its generator
```

## Troubleshooting

| Symptom | Cause |
|---|---|
| Health check 404s with an HTML body | You probed `/healthz`. Use `/health`. |
| `HTTP 411` on a POST | Bodyless POST without `Content-Length: 0`. |
| First question takes forever | The client sent audio before `state: LISTENING`. |
| Session connects, then silence | The client never sent the `begin` frame. |
| `404` on a session you own | Check the uid on the ID token — cross-user access renders as 404 by design. |
| `429` on session create | Daily cap (5/user/day) or concurrency cap (10). |
| Highlights land on wrong words | Span offsets are Go **byte** offsets, not JS string indices. |

Next: [09-demo-script.md](09-demo-script.md).
