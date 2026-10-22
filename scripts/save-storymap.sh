#!/usr/bin/env bash
# save-storymap.sh — Validate and save story-map YAML via API.
#
# Usage:
#   cat story-map.yaml | save-storymap.sh <epic-id>
#   save-storymap.sh <epic-id> < story-map.yaml
#   save-storymap.sh <epic-id> /path/to/story-map.yaml
#
# Exit codes:
#   0  — saved successfully
#   1  — validation failed (errors printed to stderr)
#   2  — usage error or API unreachable

set -euo pipefail

PIPER_URL="${PIPER_URL:-http://localhost:8622}"

if [ $# -lt 1 ]; then
  echo "Usage: save-storymap.sh <epic-id> [yaml-file]" >&2
  exit 2
fi

EPIC_ID="$1"
TMPFILE=$(mktemp /tmp/storymap-XXXXXX.yaml)
trap 'rm -f "$TMPFILE"' EXIT

# Read YAML from file argument or stdin
if [ $# -ge 2 ] && [ -f "$2" ]; then
  cp "$2" "$TMPFILE"
else
  cat > "$TMPFILE"
fi

# Check content is not empty
if [ ! -s "$TMPFILE" ]; then
  echo "Error: empty YAML content" >&2
  exit 2
fi

# Send to API
HTTP_CODE=$(curl -s -o /tmp/storymap-response.json -w "%{http_code}" \
  -X PUT \
  -H "Content-Type: text/yaml" \
  --data-binary "@${TMPFILE}" \
  "${PIPER_URL}/api/epics/${EPIC_ID}/story-map" 2>/dev/null) || {
  echo "Error: cannot reach piper API at ${PIPER_URL}" >&2
  exit 2
}

RESPONSE=$(cat /tmp/storymap-response.json)
rm -f /tmp/storymap-response.json

if [ "$HTTP_CODE" = "200" ]; then
  echo "OK: story-map saved for ${EPIC_ID}"
  exit 0
elif [ "$HTTP_CODE" = "400" ]; then
  echo "Validation failed for ${EPIC_ID}. Errors:" >&2
  echo "$RESPONSE" | python3 -c "
import sys, json
try:
    data = json.load(sys.stdin)
    for e in data.get('errors', []):
        print(f'  - {e}', file=sys.stderr)
except:
    print(sys.stdin.read(), file=sys.stderr)
" 2>&1 >&2
  exit 1
elif [ "$HTTP_CODE" = "404" ]; then
  echo "Error: epic '${EPIC_ID}' not found" >&2
  exit 2
else
  echo "Error: unexpected HTTP ${HTTP_CODE}" >&2
  echo "$RESPONSE" >&2
  exit 2
fi
