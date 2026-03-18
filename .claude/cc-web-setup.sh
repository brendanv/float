#!/usr/bin/env bash
# Setup script for Claude Code on the web.
# Runs once per new session (as root on Ubuntu 24.04) before Claude Code launches.
# Paste this into the "Setup script" field in the cloud environment settings.
# PATH persistence is handled by the SessionStart hook in .claude/settings.json.
set -euo pipefail

export GITHUB_TOKEN="ghp_your_token_here"
PROJECT_DIR="/home/user/float"

# Install mise if not present
if ! command -v mise &>/dev/null; then
  curl -fsSL https://mise.run | sh
fi

export PATH="$HOME/.local/bin:$PATH"

# Trust the project and install all pinned tools (go, buf, golangci-lint, hledger)
mise trust --yes "$PROJECT_DIR"
cd "$PROJECT_DIR" && mise install --yes
