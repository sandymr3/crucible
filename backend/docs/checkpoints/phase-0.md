# Phase 0 checkpoint — Foundations

**Status: complete.** All exit criteria met.

## What exists now

| Resource | Value |
|---|---|
| GCP project | `crucible-hack-0725` (project number `103350253775`) |
| Billing | linked to `01DEC4-171F73-487162`, budget `$50` with alerts at 50/80/100% |
| Firestore | Native mode, `us-central1` |
| GCS bucket | `gs://crucible-hack-0725-media`, uniform access, lifecycle applied |
| Runtime service account | `crucible-backend@crucible-hack-0725.iam.gserviceaccount.com` |
| Local credentials | `backend/secrets/key.json` — gitignored **and** gcloudignored |
| Cloud Run service | `crucible-backend`, `us-central1` |
| **Service URL** | **https://crucible-backend-103350253775.us-central1.run.app** |

IAM on the runtime SA is exactly three grants — `roles/aiplatform.user`,
`roles/datastore.user` at project level, and `roles/storage.objectAdmin` scoped
to the single bucket. No Editor, no default compute SA.

## Exit criteria

- ✅ `GET /health` → `200 {"status":"ok"}` from Cloud Run
- ✅ `GET /readyz` → `200`, performing a real Vertex inference call. Reports
  `credential_source: application-default-credentials` on Cloud Run and
  `service-account-key-file` locally — the single-code-path credential design
  verified in both environments.
- ✅ Vertex usage confirmed: **31 model invocations** recorded against the
  project via the monitoring API, across `global` and three Live regions. This
  is the proof that inference bills to Vertex and not to a stray API key.
- ✅ `min-instances=1` confirmed present on the deployed revision.
- ✅ `go vet ./...` and `go test ./...` clean.

## Findings that changed the plan

### 1. `genai.BackendVertexAI` is correct — my planning-time correction was wrong

pkg.go.dev prose showed `BackendEnterprise`. The pinned v1.65.0 source has
`BackendUnspecified`, `BackendGeminiAPI`, `BackendVertexAI`. The PRD was right.
This is exactly why the plan verified with `go doc` before writing code.

### 2. The Live SDK surface differs from the published docs in five ways

Full detail in [sdk-surface.md](../sdk-surface.md). The material ones:
`Session` is a struct not an interface; `Receive()` is a **blocking call**, not
an `iter.Seq2` iterator; the send methods take **values**, not pointers.
Anything written against the doc prose would not have compiled.

### 3. ⚠️ The two model families live in DIFFERENT Vertex locations

Measured with `cmd/regionprobe` (real bidi WebSocket handshakes, because a REST
`models.get` returns 404 for Live models even where they work):

| Model family | `us-central1` | `global` |
|---|---|---|
| Live native-audio | ✅ works (also us-east4, europe-west4) | ❌ close 1008 policy violation |
| Gemini 3.x text | ❌ 404 — nothing past the 2.5 family | ✅ works |

The PRD advises pinning one region for both to avoid latency asymmetry. **That
is no longer possible.** `config.Config` now carries `LiveLocation` and
`ReasoningLocation`, and `vertexai.Client` holds two SDK clients. The asymmetry
only touches post-turn calls, never the live conversation, so no user can
perceive it. PRD risk R11 is retired.

### 4. ⚠️ Model A/B reversed the plan's recommendation

Three warm runs each, same structured span-grading prompt:

| Model | Latency | Verdict |
|---|---|---|
| `gemini-3.5-flash` | 55s / 7.0s / 24s | **Disqualified** — variance blows the 4s evaluation budget |
| `gemini-3.6-flash` | 4.6s / 4.6s / 4.1s | **Selected** |

The plan recommended 3.5-flash on the reasoning that a 4-day-old model is a bad
demo dependency. The data overrides that: 3.5-flash is unusable here. Both
produced correct verdicts on the PRD §11.2 worked example and **neither emitted
a false red**. 3.6-flash grades somewhat harsher (marked a Kafka claim
`unsupported` where 3.5 said `validated`) — the Phase 4 calibration prompt is
where that gets tuned.

`gemini-3.5-flash-lite` measured ~1.5s and is kept for hints.

### 5. ⚠️ Google's frontend intercepts `/healthz` on `*.run.app`

`GET /healthz` returns Google's own HTML 404. The request never reaches the
container — no trace header, no entry in Cloud Run logs — while `/nonexistent`
correctly returns our Go mux's plain-text 404. Probed the alternatives:
`/health`, `/_health`, `/livez`, `/v1/healthz`, `/status`, `/ping` all pass
through; only `/healthz` is intercepted.

The PRD's API contract names `/healthz`, and that path works locally and behind
a custom domain or Firebase Hosting rewrite, so it stays registered. But
**`/health` is canonical** for anything probing the raw run.app URL, including
`deploy.sh` and any uptime check. This would have surfaced as a mystery health
check failure on demo day.

## Notes for Phase 1

- `cmd/regionprobe` already completed a **successful Live bidi handshake**
  (`setupComplete` received) against `gemini-live-2.5-flash-native-audio` in
  us-central1. The riskiest unknown in the project is substantially retired
  before Phase 1 formally starts. Reuse that connect code as the spike's base.
- Use `vx.RawLive()` for `Live.Connect` and `vx.RawText()` for everything else.
  Mixing them produces a confusing 404 rather than a clear error.
- `AutomaticActivityDetection.Disabled` and `ActivityStart`/`ActivityEnd` are
  confirmed present, so AD-2 (manual turn boundaries) is buildable as designed.
- `LiveServerContent.InterimInputTranscription` exists alongside
  `InputTranscription`. That is PRD §9.3's interim-opacity transcript effect for
  free — no extra work needed.
- `AudioStreamEnd` is only valid when automatic activity detection is **enabled**.
  In manual mode use `ActivityEnd` instead; sending both is a protocol error.
- The usage ledger interface (`vertexai.UsageRecorder`) is wired but no-op.
  Phase 2 implements the Firestore-backed one. `UsageFromLive` and
  `UsageFromGenerate` already normalise the two incompatible SDK usage types and
  split audio vs text tokens, which is what the cost model needs.

## Outstanding / deferred

- **CI is written but not active.** `.github/workflows/deploy.yml` expects
  `GCP_WIF_PROVIDER` and `GCP_DEPLOY_SA` secrets. The repo is not yet a git
  repo and has no remote, so Workload Identity Federation is unconfigured.
  Deploys currently run via `make deploy`.
- **Budget amount is a guess.** Set to $50 because the actual credit balance is
  unknown. Adjust in the console if that is wrong.
- Firestore security rules are Phase 2, per the plan.
