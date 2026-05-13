#!/usr/bin/env bash
# PostToolUse hook: when Bash output exceeds the threshold, upload it to the
# paste service and replace the tool result with a compact URL reference.
# Exit 2 tells Claude Code to substitute our message for the raw tool output.

set -euo pipefail

INPUT=$(cat)

TOOL_NAME=$(echo "$INPUT" | jq -r '.tool_name // ""')
[[ "$TOOL_NAME" != "Bash" ]] && exit 0

OUTPUT=$(echo "$INPUT" | jq -r '.tool_response.output // ""')
OUTPUT_LEN=${#OUTPUT}
THRESHOLD=5000

[[ $OUTPUT_LEN -le $THRESHOLD ]] && exit 0

# Bail silently if paste credentials aren't set
[[ -z "${PASTE_URL:-}" || -z "${PASTE_USER_ID:-}" || -z "${PASTE_API_KEY:-}" ]] && exit 0

COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // "bash"')
TITLE="Bash: $(echo "$COMMAND" | head -c 80)"

RESPONSE=$(curl -sf -X POST "${PASTE_URL}/api/create" \
  -H "Content-Type: application/json" \
  -H "Origin: ${PASTE_URL}" \
  -H "X-PASTE-USERID: ${PASTE_USER_ID}" \
  -H "X-PASTE-API-KEY: ${PASTE_API_KEY}" \
  -d "$(jq -n --arg c "$OUTPUT" --arg t "$TITLE" \
    '{content: $c, visibility: "logged_in", expiration: "1day", title: $t}')" 2>/dev/null) || exit 0

SLUG=$(echo "$RESPONSE" | jq -r '.slug // empty' 2>/dev/null) || exit 0
[[ -z "${SLUG:-}" ]] && exit 0

FULL_URL="${PASTE_URL}/p/${SLUG}"
PREVIEW=$(printf '%s' "$OUTPUT" | head -c 400)

printf 'Output was large (%d chars) — auto-pasted at: %s\n\nFirst 400 chars:\n%s\n...\n' \
  "$OUTPUT_LEN" "$FULL_URL" "$PREVIEW"
exit 2
