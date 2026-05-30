---
name: tui-preview
description: Generate a preview image of the float TUI (terminal UI) and upload it to the paste service. TRIGGER when the user asks to see the TUI, preview TUI changes visually, capture a screenshot of the terminal interface, or share what the float TUI looks like. Requires tmux, freeze (via mise), a running floatd with data in $VAULT_DATA_DIR, and paste service credentials ($PASTE_URL, $PASTE_USER_ID, $PASTE_API_KEY).
---

# tui-preview skill

Use this skill to generate preview images of the float TUI. It starts `floatd` and the `float` TUI in a tmux session, navigates to the requested view, captures the pane with `freeze`, and uploads the result via the paste service.

## Prerequisites

- **tmux** — must be installed via the system package manager (`apt install tmux` or `brew install tmux`). Verify with `tmux -V`.
- **freeze** — managed by mise. Run `mise install` from the repo root to install.
- **floatd data** — `FLOAT_DATA_DIR` (or `VAULT_DATA_DIR`) must point to a directory containing `config.toml` and `main.journal`.

## Capturing all screenshots at once

Use the dedicated script to capture every TUI view in one pass:

```bash
cd /home/user/float && bash scripts/capture-tui-screenshots.sh /tmp/tui-screenshots
```

The script accepts an optional output directory (defaults to a `mktemp -d` temp dir). It builds the binaries once, starts floatd, then cycles through all views — restarting the TUI between groups for reliability. All PNGs land in the output directory.

## All defined screenshots

| File | Description |
|------|-------------|
| `home.png` | Home tab — default view |
| `home-split.png` | Home tab — split view showing all postings |
| `home-filter.png` | Home tab — filtered to expenses |
| `accounts.png` | Accounts tab — account tree |
| `accounts-register.png` | Accounts tab — single-account register |
| `trends.png` | Trends tab — net worth chart |
| `assertions.png` | Assertions tab — balance assertion freshness |
| `assertions-editor.png` | Assertions tab — record assertion modal |
| `manage-rules.png` | Manage tab → Rules sub-tab |
| `manage-imports.png` | Manage tab → Imports sub-tab |
| `manage-tags.png` | Manage tab → Tags sub-tab |
| `manage-snapshots.png` | Manage tab → Snapshots sub-tab |
| `manage-prices.png` | Manage tab → Prices sub-tab |
| `manage-payees.png` | Manage tab → Payees sub-tab |
| `settings.png` | Settings tab — theme selector |

## Capturing a single view manually

For one-off previews use these steps directly.

### Step 1 — Start a tmux session

```bash
tmux new-session -d -s float-preview -x 220 -y 50
```

### Step 2 — Start floatd in the first pane

```bash
tmux send-keys -t float-preview:0.0 \
  "FLOAT_DATA_DIR=${FLOAT_DATA_DIR:-$VAULT_DATA_DIR} mise run floatd" Enter
```

Wait ~3 seconds for floatd to fully start and print its listening address.

### Step 3 — Open a second pane and start the TUI

```bash
tmux split-window -t float-preview:0 -v
tmux send-keys -t float-preview:0.1 \
  "FLOAT_ADDR=${FLOAT_ADDR:-localhost:8080} mise run float" Enter
```

Wait ~2 seconds for the TUI to connect and render initial data.

### Step 4 — Navigate to the target view

Send keystrokes with `tmux send-keys -t float-preview:0.1`. Main tabs cycle with `Tab`/`Shift+Tab`:

| Tab (index) | View |
|-------------|------|
| 0 | Home (default) |
| 1 | Accounts |
| 2 | Trends |
| 3 | Portfolio |
| 4 | Monthly |
| 5 | Assertions |
| 6 | Manage |
| 7 | Settings |

Manage sub-tabs cycle with `[` / `]`:

| Sub-tab | View |
|---------|------|
| 0 | Rules |
| 1 | Imports |
| 2 | Tags |
| 3 | Snapshots |
| 4 | Prices |
| 5 | Payees |

Other useful keys:

| Key | Action |
|-----|--------|
| `s` | Toggle split view (show all postings) |
| `/` → query → `Enter` | Apply transaction filter |
| `v` | Cycle preset filters on Home |
| `h` / `l` | Move focus left / right |
| `j` / `k` | Scroll accounts or transactions |
| `[` / `]` | Previous / next period (chart & accounts) |
| `Enter` | Drill into account register |
| `Esc` | Go back |

```bash
# Example: navigate to Manage → Imports
tmux send-keys -t float-preview:0.1 "" Tab   # → Accounts
tmux send-keys -t float-preview:0.1 "" Tab   # → Trends
tmux send-keys -t float-preview:0.1 "" Tab   # → Portfolio
tmux send-keys -t float-preview:0.1 "" Tab   # → Monthly
tmux send-keys -t float-preview:0.1 "" Tab   # → Assertions
tmux send-keys -t float-preview:0.1 "" Tab   # → Manage
tmux send-keys -t float-preview:0.1 "]" ""   # → Imports
```

After each navigation action, wait ~1 second for the TUI to re-render before capturing.

### Step 5 — Capture the pane with freeze

```bash
tmux capture-pane -t float-preview:0.1 -ep | \
  freeze --language ansi --theme charm --padding 20 -o /tmp/tui-preview.png
```

### Step 6 — Upload the image with the paste skill

```bash
RESPONSE=$(curl -s -X POST "$PASTE_URL/api/upload" \
  -H "X-PASTE-USERID: $PASTE_USER_ID" \
  -H "X-PASTE-API-KEY: $PASTE_API_KEY" \
  -H "Origin: $PASTE_URL" \
  -F "file=@/tmp/tui-preview.png" \
  -F "visibility=logged_in" \
  -F "expiration=1day")

SLUG=$(echo "$RESPONSE" | jq -r '.slug')
echo "Preview uploaded: ${PASTE_URL}/p/${SLUG}"
```

### Step 7 — Cleanup

```bash
tmux kill-session -t float-preview
rm -f /tmp/tui-preview.png
```
