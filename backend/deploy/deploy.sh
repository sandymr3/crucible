#!/usr/bin/env bash
#
# Deploy the Crucible backend to Cloud Run.
#
# Every flag here is load-bearing; the defaults are wrong for this workload in
# ways that only show up during a live demo. See the comments before changing
# any of them.
set -euo pipefail

PROJECT="${GOOGLE_CLOUD_PROJECT:-crucible-hack-0725}"
REGION="${CLOUD_RUN_REGION:-us-central1}"
SERVICE="${CLOUD_RUN_SERVICE:-crucible-backend}"
BUCKET="${GCS_BUCKET:-${PROJECT}-media}"
SA="crucible-backend@${PROJECT}.iam.gserviceaccount.com"
VERSION="$(git rev-parse --short HEAD 2>/dev/null || echo dev)"

cd "$(dirname "$0")/.."

echo "==> Deploying ${SERVICE} to ${PROJECT}/${REGION} (version ${VERSION})"

gcloud run deploy "${SERVICE}" \
  --project="${PROJECT}" \
  --region="${REGION}" \
  --source=. \
  --service-account="${SA}" \
  --allow-unauthenticated \
  --port=8080 \
  \
  `# 3600s: the default 300s severs the WebSocket mid-interview. Cloud Run` \
  `# still caps WS connections at 60 min regardless, which comfortably` \
  `# contains the 12-minute session cap.` \
  --timeout=3600 \
  \
  `# Without affinity a reconnect can land on an instance with no in-memory` \
  `# session. Firestore is authoritative so this is recoverable, but affinity` \
  `# makes the common case free.` \
  --session-affinity \
  \
  `# A cold start during judging is unforgivable. Costs a few dollars.` \
  --min-instances=1 \
  `# Credit protection.` \
  --max-instances=5 \
  \
  `# "CPU only during request" throttles the background evaluation workers` \
  `# between requests, which is exactly when they need to run.` \
  --cpu=2 \
  --no-cpu-throttling \
  --memory=2Gi \
  \
  `# Each live session holds a WebSocket pair plus audio buffers.` \
  --concurrency=20 \
  \
  --set-env-vars="^@^GOOGLE_CLOUD_PROJECT=${PROJECT}@GCS_BUCKET=${BUCKET}@VERTEX_LIVE_LOCATION=us-central1@VERTEX_REASONING_LOCATION=global@MODEL_LIVE=gemini-live-2.5-flash-native-audio@MODEL_REASONING=gemini-3.6-flash@MODEL_CHEAP=gemini-3.5-flash-lite@APP_ENV=cloudrun@LOG_LEVEL=info@LIVE_ACTIVITY_MODE=manual@INJECTION_DEADLINE_MS=9000@EVAL_RED_CONFIDENCE_MIN=0.75@EVAL_MIN_WORDS=15@EVAL_THINKING_BUDGET=512@ROADMAP_HORIZON_DAYS=7@SESSION_MAX_DURATION_SEC=720@SESSION_IDLE_TIMEOUT_SEC=90@MAX_HINTS_PER_TURN=2@MAX_CONCURRENT_SESSIONS=10@DAILY_SESSION_CAP_PER_USER=5"

URL="$(gcloud run services describe "${SERVICE}" --project="${PROJECT}" --region="${REGION}" --format='value(status.url)')"

echo
echo "==> Deployed: ${URL}"
echo "==> Verifying"
# Probe /health, NOT /healthz: Google's frontend intercepts "/healthz" on
# *.run.app and answers it with its own 404 before the request reaches the
# container. See the routing comment in cmd/server/main.go.
curl -fsS "${URL}/health" && echo
curl -fsS "${URL}/readyz" && echo

echo
echo "==> Confirm min-instances survived the deploy (Appendix C checklist item)"
gcloud run services describe "${SERVICE}" --project="${PROJECT}" --region="${REGION}" \
  --format='value(spec.template.metadata.annotations["autoscaling.knative.dev/minScale"])'
