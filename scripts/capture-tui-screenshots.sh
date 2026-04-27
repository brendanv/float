#!/usr/bin/env bash
# Capture all float TUI screenshots and save them to a directory.
# Usage:
#   ./scripts/capture-tui-screenshots.sh [OUTPUT_DIR]
#   ./scripts/capture-tui-screenshots.sh --output /tmp/my-screenshots
#
# If OUTPUT_DIR is omitted a temporary directory is created automatically.
# Requires FLOAT_DATA_DIR (or VAULT_DATA_DIR) to point to a float data directory.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

OUTPUT_DIR=""
FLOAT_DATA_DIR="${FLOAT_DATA_DIR:-${VAULT_DATA_DIR:-}}"
FLOAT_ADDR="${FLOAT_ADDR:-localhost:8080}"

while [[ $# -gt 0 ]]; do
  case $1 in
    -o|--output)
      OUTPUT_DIR="$2"
      shift 2
      ;;
    -d|--data-dir)
      FLOAT_DATA_DIR="$2"
      shift 2
      ;;
    *)
      OUTPUT_DIR="$1"
      shift
      ;;
  esac
done

if [[ -z "$OUTPUT_DIR" ]]; then
  OUTPUT_DIR="$(mktemp -d)"
  echo "No output directory specified, using: $OUTPUT_DIR"
fi

if [[ -z "$FLOAT_DATA_DIR" ]]; then
  echo "Error: FLOAT_DATA_DIR (or VAULT_DATA_DIR) must be set" >&2
  exit 1
fi

mkdir -p "$OUTPUT_DIR"

SESSION="float-tui-screenshots"
TUI_PANE="$SESSION:0.1"

# Build binaries once so restarts are fast.
echo "Building binaries…"
FLOATD_BIN="$(mktemp /tmp/floatd-XXXXXX)"
TUI_BIN="$(mktemp /tmp/float-tui-XXXXXX)"
cd "$REPO_ROOT"
go build -o "$FLOATD_BIN" ./cmd/floatd/
go build -o "$TUI_BIN" ./cmd/float/
echo "Build complete."
echo ""

cleanup() {
  tmux kill-session -t "$SESSION" 2>/dev/null || true
  rm -f "$FLOATD_BIN" "$TUI_BIN"
}
trap cleanup EXIT

# Wide terminal so the TUI renders at full fidelity.
tmux new-session -d -s "$SESSION" -x 220 -y 50

# floatd in the top pane.
tmux send-keys -t "$SESSION:0.0" \
  "FLOAT_DATA_DIR='$FLOAT_DATA_DIR' '$FLOATD_BIN' --data-dir '$FLOAT_DATA_DIR'" Enter

echo "Waiting for floatd to start…"
sleep 4

# Bottom pane for the TUI (~80% of height).
tmux split-window -t "$SESSION:0" -v -l 40

echo "Capturing TUI screenshots… (output: $OUTPUT_DIR)"
echo ""

count=0

start_tui() {
  # Interrupt any lingering process, then launch fresh.
  tmux send-keys -t "$TUI_PANE" C-c 2>/dev/null || true
  sleep 0.2
  tmux send-keys -t "$TUI_PANE" "'$TUI_BIN' --server $FLOAT_ADDR" Enter
  sleep 3
}

stop_tui() {
  tmux send-keys -t "$TUI_PANE" "q" ""
  sleep 1
}

capture() {
  local name="$1"
  tmux capture-pane -t "$TUI_PANE" -ep | \
    freeze --language ansi --theme charm --padding 20 \
      -o "$OUTPUT_DIR/${name}.png"
  echo "  captured: ${name}.png"
  ((count++)) || true
}

# ── Home tab ──────────────────────────────────────────────────────────────────
start_tui
sleep 2
capture "home"

# Split view
tmux send-keys -t "$TUI_PANE" "s" ""
sleep 0.5
capture "home-split"
tmux send-keys -t "$TUI_PANE" "s" ""  # toggle off

# Filtered view
tmux send-keys -t "$TUI_PANE" "/" ""
sleep 0.3
tmux send-keys -t "$TUI_PANE" "expenses" ""
sleep 0.2
tmux send-keys -t "$TUI_PANE" "" Enter
sleep 1
capture "home-filter"

stop_tui

# ── Accounts tab ──────────────────────────────────────────────────────────────
start_tui
tmux send-keys -t "$TUI_PANE" "" Tab   # → Accounts
sleep 1
capture "accounts"

tmux send-keys -t "$TUI_PANE" "" Enter  # drill into first account
sleep 1
capture "accounts-register"

stop_tui

# ── Trends tab ────────────────────────────────────────────────────────────────
start_tui
tmux send-keys -t "$TUI_PANE" "" Tab   # → Accounts
tmux send-keys -t "$TUI_PANE" "" Tab   # → Trends
sleep 2
capture "trends"

stop_tui

# ── Manage tab — all 6 sub-tabs ───────────────────────────────────────────────
start_tui
tmux send-keys -t "$TUI_PANE" "" Tab   # → Accounts
tmux send-keys -t "$TUI_PANE" "" Tab   # → Trends
tmux send-keys -t "$TUI_PANE" "" Tab   # → Manage
sleep 2
capture "manage-rules"

tmux send-keys -t "$TUI_PANE" "]" ""
sleep 1
capture "manage-imports"

tmux send-keys -t "$TUI_PANE" "]" ""
sleep 1
capture "manage-tags"

tmux send-keys -t "$TUI_PANE" "]" ""
sleep 1
capture "manage-snapshots"

tmux send-keys -t "$TUI_PANE" "]" ""
sleep 1
capture "manage-prices"

tmux send-keys -t "$TUI_PANE" "]" ""
sleep 1
capture "manage-payees"

stop_tui

# ── Settings tab ──────────────────────────────────────────────────────────────
start_tui
tmux send-keys -t "$TUI_PANE" "" Tab   # → Accounts
tmux send-keys -t "$TUI_PANE" "" Tab   # → Trends
tmux send-keys -t "$TUI_PANE" "" Tab   # → Manage
tmux send-keys -t "$TUI_PANE" "" Tab   # → Settings
sleep 1
capture "settings"

stop_tui

echo ""
echo "Captured $count screenshots → $OUTPUT_DIR"
