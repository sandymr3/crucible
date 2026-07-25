#!/usr/bin/env bash
#
# Load test at the PRD's concurrency target (§4.4: 10 concurrent sessions —
# "judges may all click at once").
#
# Uses REPLAY sessions deliberately. They exercise the whole relay path —
# WebSocket upgrade, auth, ownership check, guardrails, the outbound write pump,
# audio framing and sequence numbering — without opening ten simultaneous Vertex
# connections. That keeps the test repeatable and free, and the Vertex path is
# already proven by every other phase.
#
# Usage: bash deploy/loadtest.sh [concurrency] [seconds]
set -uo pipefail

CONC="${1:-10}"
WINDOW="${2:-10}"
BASE="${BASE:-http://localhost:8080}"
OUT="testdata/out"

cd "$(dirname "$0")/.."
mkdir -p "$OUT"

command -v taskkill >/dev/null 2>&1 && taskkill //F //IM crucible-server.exe >/dev/null 2>&1 || true
sleep 1

go build -o bin/crucible-server.exe ./cmd/server || exit 1
go build -o bin/wsprobe.exe ./cmd/wsprobe || exit 1

./bin/crucible-server.exe > "$OUT/loadtest-server.log" 2>&1 &
SRV=$!
sleep 8

if [ "$(curl -s --max-time 10 -o /dev/null -w '%{http_code}' "$BASE/health")" != "200" ]; then
  echo "server did not come up"
  kill "$SRV" 2>/dev/null
  exit 1
fi

echo "running $CONC concurrent sessions for ${WINDOW}s each"
rm -f "$OUT"/load-*.txt
START=$(date +%s)

for i in $(seq 1 "$CONC"); do
  (
    TOKEN=$(tr -d '\r\n' < "$OUT/tok-$i.txt" 2>/dev/null)
    [ -z "$TOKEN" ] && exit 0
    SID=$(curl -s --max-time 20 -X POST "$BASE/v1/sessions" \
      -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
      -d '{"mode":"replay","fixtureId":"demo-ml-engineer"}' \
      | python -c "import sys,json;print(json.load(sys.stdin).get('id',''))" 2>/dev/null)
    [ -z "$SID" ] && exit 0
    ./bin/wsprobe.exe -session="$SID" -token="$TOKEN" \
      -wait "${WINDOW}s" -out "$OUT/load-$i.wav" > "$OUT/load-$i.txt" 2>&1
  ) &
done
wait

ELAPSED=$(( $(date +%s) - START ))
kill "$SRV" 2>/dev/null
command -v taskkill >/dev/null 2>&1 && taskkill //F //IM crucible-server.exe >/dev/null 2>&1 || true

echo "wall clock: ${ELAPSED}s"
OK=0; GAPS=0
for i in $(seq 1 "$CONC"); do
  MS=$(grep -oE 'audio received +[0-9]+ bytes \([0-9]+ ms' "$OUT/load-$i.txt" 2>/dev/null | grep -oE '\([0-9]+' | tr -d '(')
  G=$(grep -oE 'sequence gaps +[0-9]+' "$OUT/load-$i.txt" 2>/dev/null | grep -oE '[0-9]+$')
  printf "  session %-3s audio %-8s gaps %s\n" "$i" "${MS:-0}" "${G:-?}"
  if [ -n "$MS" ] && [ "$MS" -gt 2000 ]; then OK=$((OK+1)); fi
  if [ -n "$G" ] && [ "$G" != "0" ]; then GAPS=$((GAPS+1)); fi
done

echo
echo "streaming audio : $OK of $CONC"
echo "with gaps       : $GAPS"
echo "server errors   : $(grep -c '"severity":"ERROR"' "$OUT/loadtest-server.log" 2>/dev/null || echo 0)"
echo "refusals        : $(grep -c 'live session refused' "$OUT/loadtest-server.log" 2>/dev/null || echo 0)"
