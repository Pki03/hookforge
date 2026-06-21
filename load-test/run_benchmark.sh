#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"

if ! command -v k6 &> /dev/null; then
  echo "Error: k6 is not installed."
  echo "Install it: brew install k6  (macOS)"
  echo "  or: https://grafana.com/docs/k6/latest/get-started/installation/"
  exit 1
fi

echo "Checking HookForge at $BASE_URL..."
if ! curl -sf "$BASE_URL/health" > /dev/null 2>&1; then
  echo "Error: HookForge is not running at $BASE_URL"
  echo "Start it: docker-compose up -d"
  exit 1
fi
echo "HookForge is running."

echo ""
echo "Running k6 load test..."
echo "========================"
k6 run "$(dirname "$0")/k6_load_test.js" --out json=/dev/stdout 2>/dev/null | \
  grep '"type":"Point"' | tail -1 | \
  python3 -c "
import sys, json
for line in sys.stdin:
    try:
        d = json.loads(line.strip().rstrip(','))
        if d.get('type') == 'Point':
            m = d.get('metric', '')
            v = d.get('data', {}).get('value', '')
            print(f'{m}: {v}')
    except: pass
" 2>/dev/null || true

echo ""
echo "Post-benchmark stats:"
echo "====================="
curl -s "$BASE_URL/api/v1/stats" | python3 -m json.tool 2>/dev/null || curl -s "$BASE_URL/api/v1/stats"

echo ""
echo "========================"
TOTAL=$(curl -sf "$BASE_URL/api/v1/stats" | python3 -c "import sys,json; print(json.load(sys.stdin).get('total_sent', 0))" 2>/dev/null || echo "?")
RATE=$(curl -sf "$BASE_URL/api/v1/stats" | python3 -c "import sys,json; print(json.load(sys.stdin).get('delivery_rate_percent', 0))" 2>/dev/null || echo "?")
echo "Benchmark complete. Total events: $TOTAL | Delivery rate: ${RATE}%"
