# tui-preview skill

Use this skill to generate preview images of the float TUI. It starts `floatd` and the `float` TUI in a tmux session, navigates to the requested view, captures the pane with `freeze`, and uploads the result via the paste service.

## Prerequisites

- **tmux** — must be installed via the system package manager (`apt install tmux` or `brew install tmux`). Verify with `tmux -V`.
- **freeze** — managed by mise. Run `mise install` from the repo root to install.
- **floatd** — requires a `config.toml` and `main.journal` in `$VAULT_DATA_DIR`.

## Step 1 — Start a tmux session

Use a wide, tall terminal so the TUI renders at full fidelity:

```bash
tmux new-session -d -s float-preview -x 220 -y 50
```

## Step 2 — Start floatd in the first pane

```bash
tmux send-keys -t float-preview:0.0 \
  "FLOAT_DATA_DIR=${FLOAT_DATA_DIR:-$VAULT_DATA_DIR} mise run floatd" Enter
```

Wait ~3 seconds for floatd to fully start and print its listening address.

## Step 3 — Open a second pane and start the TUI

```bash
tmux split-window -t float-preview:0 -v
tmux send-keys -t float-preview:0.1 \
  "FLOAT_ADDR=${FLOAT_ADDR:-localhost:8080} mise run float" Enter
```

Wait ~2 seconds for the TUI to connect and render initial data.

## Step 4 — Navigate to the target view

Send keystrokes to the TUI pane with `tmux send-keys -t float-preview:0.1`. The available views and their key bindings are:

| View | Keys |
|------|------|
| Home tab (default) | — already on Home at startup |
| Manager tab | `Tab` |
| Previous month | `[` |
| Next month | `]` |
| Activate transaction filter | `/` then type query (e.g. `expenses`), then `Enter` |
| Toggle split view (show all postings) | `s` |
| Move focus left / right | `h` / `l` |
| Scroll accounts or transactions | `j` (down) / `k` (up) |

### Examples

```bash
# Navigate to expenses in January 2026
tmux send-keys -t float-preview:0.1 "[[[[[[[[[[" ""   # press [ multiple times to reach Jan
tmux send-keys -t float-preview:0.1 "/" ""
tmux send-keys -t float-preview:0.1 "expenses" ""
tmux send-keys -t float-preview:0.1 "" Enter

# Enable split view for the transactions panel
tmux send-keys -t float-preview:0.1 "s" ""

# Switch to Manager tab
tmux send-keys -t float-preview:0.1 "" Tab
```

After each navigation action, wait ~1 second for the TUI to re-render before capturing.

## Step 5 — Capture the pane with freeze

```bash
tmux capture-pane -t float-preview:0.1 -ep | \
  freeze --language ansi -o /tmp/tui-preview.png
```

Optionally add freeze style flags for a cleaner output:

```bash
tmux capture-pane -t float-preview:0.1 -ep | \
  freeze --language ansi \
    --theme dracula \
    --padding 20 \
    -o /tmp/tui-preview.png
```

## Step 6 — Upload the image with the paste skill

Use the [paste skill](.claude/skills/paste.md) to upload `/tmp/tui-preview.png` with:
- `visibility`: `logged_in`
- `expiration`: `1week`

```bash
RESPONSE=$(curl -s -X POST "$PASTE_URL/api/upload" \
  -H "X-PASTE-USERID: $PASTE_USER_ID" \
  -H "X-PASTE-API-KEY: $PASTE_API_KEY" \
  -F "file=@/tmp/tui-preview.png" \
  -F "visibility=logged_in" \
  -F "expiration=1week")

SLUG=$(echo "$RESPONSE" | jq -r '.slug')
FULL_URL="${PASTE_URL}/p/${SLUG}"
echo "Preview uploaded: $FULL_URL"
```

Output the full URL so it can be navigated to directly.

## Step 7 — Cleanup

```bash
tmux kill-session -t float-preview
rm -f /tmp/tui-preview.png
```

## Complete example

```bash
# 1. Start session
tmux new-session -d -s float-preview -x 220 -y 50

# 2. Start floatd
tmux send-keys -t float-preview:0.0 \
  "FLOAT_DATA_DIR=$VAULT_DATA_DIR mise run floatd" Enter
sleep 3

# 3. Start TUI
tmux split-window -t float-preview:0 -v
tmux send-keys -t float-preview:0.1 "mise run float" Enter
sleep 2

# 4. (optional) navigate to a view — e.g. filter to expenses
tmux send-keys -t float-preview:0.1 "/" ""
tmux send-keys -t float-preview:0.1 "expenses" ""
tmux send-keys -t float-preview:0.1 "" Enter
sleep 1

# 5. Capture
tmux capture-pane -t float-preview:0.1 -ep | \
  freeze --language ansi -o /tmp/tui-preview.png

# 6. Upload
RESPONSE=$(curl -s -X POST "$PASTE_URL/api/upload" \
  -H "X-PASTE-USERID: $PASTE_USER_ID" \
  -H "X-PASTE-API-KEY: $PASTE_API_KEY" \
  -F "file=@/tmp/tui-preview.png" \
  -F "visibility=logged_in" \
  -F "expiration=1week")
SLUG=$(echo "$RESPONSE" | jq -r '.slug')
echo "Preview: ${PASTE_URL}/p/${SLUG}"

# 7. Cleanup
tmux kill-session -t float-preview
rm -f /tmp/tui-preview.png
```
