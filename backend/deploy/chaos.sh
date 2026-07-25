#!/usr/bin/env bash
#
# Chaos pass. Every one of these must degrade toward "the interview keeps
# working" rather than failing outright — the principle from PRD §16.6 that the
# conversation is the product and everything else is enrichment.
#
# Usage: bash deploy/chaos.sh
set -uo pipefail

BASE="${BASE:-http://localhost:8080}"
OUT="testdata/out"
cd "$(dirname "$0")/.."
mkdir -p "$OUT"

KEY=$(cat secrets/webapikey.txt)
mint() {
  curl -s --max-time 20 -X POST \
    "https://identitytoolkit.googleapis.com/v1/accounts:signUp?key=$KEY" \
    -H "Content-Type: application/json" -d '{"returnSecureToken":true}' \
  | python -c "import sys,json; print(json.load(sys.stdin)['idToken'])"
}

pass=0; fail=0
check() { # name expected actual
  if [ "$2" = "$3" ]; then
    printf "  PASS  %-52s %s\n" "$1" "$3"; pass=$((pass+1))
  else
    printf "  FAIL  %-52s got %s want %s\n" "$1" "$3" "$2"; fail=$((fail+1))
  fi
}

command -v taskkill >/dev/null 2>&1 && taskkill //F //IM crucible-server.exe >/dev/null 2>&1 || true
sleep 1
go build -o bin/crucible-server.exe ./cmd/server || exit 1
./bin/crucible-server.exe > "$OUT/chaos-server.log" 2>&1 &
SRV=$!
sleep 8

TOK=$(mint); A="Authorization: Bearer $TOK"

echo "=== auth and authorisation ==="
check "unauthenticated session create" "401" \
  "$(curl -s -o /dev/null -w '%{http_code}' --max-time 15 -X POST $BASE/v1/sessions -d '{}')"
check "garbage bearer token" "401" \
  "$(curl -s -o /dev/null -w '%{http_code}' --max-time 15 -X POST $BASE/v1/sessions -H 'Authorization: Bearer nonsense' -d '{}')"
check "unauthenticated websocket" "401" \
  "$(curl -s -o /dev/null -w '%{http_code}' --max-time 15 $BASE/v1/sessions/x/live)"

echo
echo "=== input validation ==="
check "invalid persona" "400" \
  "$(curl -s -o /dev/null -w '%{http_code}' --max-time 15 -X POST $BASE/v1/sessions -H "$A" -H 'Content-Type: application/json' -d '{"mode":"interview","persona":"cto"}')"
check "invalid mode" "400" \
  "$(curl -s -o /dev/null -w '%{http_code}' --max-time 15 -X POST $BASE/v1/sessions -H "$A" -H 'Content-Type: application/json' -d '{"mode":"telepathy"}')"
check "study mode with no topic" "400" \
  "$(curl -s -o /dev/null -w '%{http_code}' --max-time 15 -X POST $BASE/v1/sessions -H "$A" -H 'Content-Type: application/json' -d '{"mode":"study"}')"
check "replay mode with no fixture" "400" \
  "$(curl -s -o /dev/null -w '%{http_code}' --max-time 15 -X POST $BASE/v1/sessions -H "$A" -H 'Content-Type: application/json' -d '{"mode":"replay"}')"

SID=$(curl -s --max-time 20 -X POST $BASE/v1/sessions -H "$A" -H 'Content-Type: application/json' \
  -d '{"mode":"interview","persona":"tech_lead"}' | python -c "import sys,json;print(json.load(sys.stdin)['id'])")

check "digest before any resume" "400" \
  "$(curl -s -o /dev/null -w '%{http_code}' --max-time 20 -X POST $BASE/v1/sessions/$SID/digest -H "$A" -H 'Content-Length: 0')"
check "oversized job description" "400" \
  "$(python -c "import json;print(json.dumps({'text':'x'*20001}))" > $OUT/big.json; \
     curl -s -o /dev/null -w '%{http_code}' --max-time 20 -X POST $BASE/v1/sessions/$SID/jd -H "$A" -H 'Content-Type: application/json' --data-binary @$OUT/big.json)"
check "non-PDF resume upload" "400" \
  "$(echo 'not a pdf' > $OUT/fake.txt; \
     curl -s -o /dev/null -w '%{http_code}' --max-time 20 -X POST $BASE/v1/sessions/$SID/resume -H "$A" -F "file=@$OUT/fake.txt;type=text/plain")"

echo
echo "=== isolation ==="
TOK2=$(mint)
check "another user reading this session" "404" \
  "$(curl -s -o /dev/null -w '%{http_code}' --max-time 15 -H "Authorization: Bearer $TOK2" $BASE/v1/sessions/$SID)"
check "another user opening its socket" "404" \
  "$(curl -s -o /dev/null -w '%{http_code}' --max-time 15 -H "Authorization: Bearer $TOK2" $BASE/v1/sessions/$SID/live)"
check "nonexistent session" "404" \
  "$(curl -s -o /dev/null -w '%{http_code}' --max-time 15 -H "$A" $BASE/v1/sessions/doesnotexist)"

echo
echo "=== graceful states, not errors ==="
check "report before the session ended" "202" \
  "$(curl -s -o /dev/null -w '%{http_code}' --max-time 15 -H "$A" $BASE/v1/sessions/$SID/report)"
check "roadmap before the session ended" "202" \
  "$(curl -s -o /dev/null -w '%{http_code}' --max-time 15 -H "$A" $BASE/v1/sessions/$SID/roadmap)"
check "unknown replay fixture" "201" \
  "$(curl -s -o /dev/null -w '%{http_code}' --max-time 15 -X POST $BASE/v1/sessions -H "$A" -H 'Content-Type: application/json' -d '{"mode":"replay","fixtureId":"nope"}')"

echo
echo "=== credit guardrails ==="
CAPTOK=$(mint); CA="Authorization: Bearer $CAPTOK"
for i in 1 2 3 4 5; do
  curl -s -o /dev/null --max-time 20 -X POST $BASE/v1/sessions -H "$CA" -H 'Content-Type: application/json' -d '{"mode":"interview"}'
done
check "daily cap refuses the 6th session" "429" \
  "$(curl -s -o /dev/null -w '%{http_code}' --max-time 20 -X POST $BASE/v1/sessions -H "$CA" -H 'Content-Type: application/json' -d '{"mode":"interview"}')"

kill "$SRV" 2>/dev/null
command -v taskkill >/dev/null 2>&1 && taskkill //F //IM crucible-server.exe >/dev/null 2>&1 || true

echo
echo "  $pass passed, $fail failed"
[ "$fail" -eq 0 ] || exit 1
