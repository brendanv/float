#!/usr/bin/env bash
# Capture all float web UI screenshots and save them to a directory.
# Usage:
#   ./capture-screenshots.sh [OUTPUT_DIR]
#   ./capture-screenshots.sh --output /tmp/my-screenshots
#
# If OUTPUT_DIR is omitted a temporary directory is created automatically.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

OUTPUT_DIR=""
while [[ $# -gt 0 ]]; do
  case $1 in
    -o|--output)
      OUTPUT_DIR="$2"
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

mkdir -p "$OUTPUT_DIR"

cd "$SCRIPT_DIR"

# Remove stale PNGs so a partial previous run doesn't pollute the output.
mkdir -p test-results
find test-results -maxdepth 1 -name "*.png" -delete

echo "Capturing screenshots… (output: $OUTPUT_DIR)"
echo ""
playwright_exit=0
bun run playwright test \
  tests/screenshots.spec.js \
  tests/screenshots-mobile.spec.js \
  tests/datepicker-screenshots.spec.js \
  tests/portfolio-mobile.spec.js \
  --reporter=list || playwright_exit=$?

count=0
for f in test-results/*.png; do
  if [[ -f "$f" ]]; then
    cp "$f" "$OUTPUT_DIR/"
    ((count++)) || true
  fi
done

echo ""
echo "Captured $count screenshots → $OUTPUT_DIR"

if [[ $playwright_exit -ne 0 ]]; then
  echo "Warning: some tests failed — screenshots from passing tests were still saved."
  exit $playwright_exit
fi
