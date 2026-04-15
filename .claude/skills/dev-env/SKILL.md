---
name: dev-env
description: Start the full float development environment (floatd + Vite web dev server) in a tmux session. Automatically seeds fake data if the data directory is empty. TRIGGER when the user wants to start the dev environment, boot float for development, run floatd and web-dev together, or set up a working dev stack.
---

# dev-env skill

Boots the complete float development stack in a tmux session: `floatd` (with `air` hot-reload) and the Vite web dev server. Automatically seeds fake data if the data directory is missing or incomplete.

## What gets started

| Service | URL | Description |
|---------|-----|-------------|
| floatd | http://localhost:8080 | gRPC API + embedded production web UI |
| Vite dev server | http://localhost:5173 | Hot-reloading web UI (proxies API to floatd) |

For web UI development, use **http://localhost:5173**. For API/gRPC testing, use **http://localhost:8080**.

## Step 1 — Resolve the data directory

```bash
DATA_DIR="${FLOAT_DATA_DIR:-$VAULT_DATA_DIR}"
echo "Data dir: $DATA_DIR"
```

`$VAULT_DATA_DIR` is set by `mise.toml` to `<repo-root>/data`.

## Step 2 — Check if data needs seeding

```bash
if [ ! -f "$DATA_DIR/main.journal" ] || [ ! -f "$DATA_DIR/config.toml" ]; then
  echo "Data directory is empty or incomplete — needs seeding"
else
  echo "Data directory OK, skipping seed"
fi
```

If the data directory is missing or incomplete, follow the **gen-fake-data** skill before continuing. Use the resolved `$DATA_DIR` as the output directory. Default to 12 months of history for a usable dataset.

## Step 3 — Kill any stale dev session

```bash
tmux kill-session -t float-dev 2>/dev/null || true
```

## Step 4 — Create a new tmux session

```bash
tmux new-session -d -s float-dev -x 220 -y 50
```

## Step 5 — Start floatd in the first pane

```bash
tmux send-keys -t float-dev:0.0 \
  "cd /home/user/float && FLOAT_DATA_DIR=\"$DATA_DIR\" mise run floatd" Enter
```

`mise run floatd` uses `air` for hot-reload — it will recompile `cmd/floatd/` on any `.go` file change.

## Step 6 — Wait for floatd to be ready

Poll until port 8080 accepts connections (up to 15 seconds):

```bash
for i in $(seq 1 15); do
  if curl -sf --max-time 1 http://localhost:8080 -o /dev/null 2>/dev/null; then
    echo "floatd ready"
    break
  fi
  sleep 1
done
```

If floatd is not ready after 15 seconds, capture its output to diagnose:

```bash
tmux capture-pane -t float-dev:0.0 -p
```

Common causes of failure: port 8080 already in use, missing `config.toml`, malformed journal file. Fix the issue before continuing.

## Step 7 — Open a second pane and start the Vite dev server

```bash
tmux split-window -t float-dev:0 -v
tmux send-keys -t float-dev:0.1 \
  "cd /home/user/float && mise run web-dev" Enter
```

`mise run web-dev` first regenerates JS protobuf code (`mise run web-gen`), then starts Vite on port 5173. On first run, `web-gen` installs bun dependencies — this may take ~30 seconds.

## Step 8 — Wait for Vite to be ready

```bash
for i in $(seq 1 30); do
  if curl -sf --max-time 1 http://localhost:5173 -o /dev/null 2>/dev/null; then
    echo "Vite ready"
    break
  fi
  sleep 1
done
```

If Vite is not ready after 30 seconds, capture its output:

```bash
tmux capture-pane -t float-dev:0.1 -p
```

## Step 9 — Report to the user

Tell the user:
- The data directory path (and whether fake data was generated)
- **Web UI (dev):** http://localhost:5173 — use this for development; changes to `web/src/` hot-reload instantly
- **API / Web UI (prod build):** http://localhost:8080 — gRPC API and the last production-built web UI
- **View logs:** `tmux attach -t float-dev` (detach with `Ctrl-b d`)
- **Stop everything:** `tmux kill-session -t float-dev`

## Troubleshooting

**floatd won't start — port in use:**
```bash
lsof -ti :8080 | xargs kill -9
```

**Vite won't start — port in use:**
```bash
lsof -ti :5173 | xargs kill -9
```

**`mise` command not found in tmux:** The PATH hook in `.claude/settings.json` only applies to Claude's shell. For tmux panes, source mise manually:
```bash
tmux send-keys -t float-dev:0.0 \
  'export PATH="$HOME/.local/share/mise/shims:$HOME/.local/bin:$PATH"' Enter
```

**Inspect live logs without attaching:**
```bash
tmux capture-pane -t float-dev:0.0 -p   # floatd
tmux capture-pane -t float-dev:0.1 -p   # web-dev
```

## Stopping the dev environment

```bash
tmux kill-session -t float-dev
```
