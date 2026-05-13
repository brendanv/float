#!/usr/bin/env python3
"""Parse go test -json output and print only failed tests with their output."""

import json
import sys
from collections import defaultdict

events = []
non_json = []
for line in sys.stdin:
    line = line.rstrip()
    if not line:
        continue
    try:
        events.append(json.loads(line))
    except Exception:
        non_json.append(line)

build_errors = [l for l in non_json if not l.startswith("go: ")]
if build_errors:
    print("Build errors:")
    print("\n".join(build_errors))
    print()

outputs = defaultdict(list)
results = {}
meta = {}

for e in events:
    test = e.get("Test")
    if not test:
        continue
    pkg = e.get("Package", "")
    key = f"{pkg}.{test}"
    action = e.get("Action", "")
    if action == "output":
        outputs[key].append(e.get("Output", ""))
    elif action in ("pass", "fail"):
        results[key] = action
        meta[key] = {"pkg": pkg, "test": test, "elapsed": e.get("Elapsed", 0)}

failures = [(k, meta[k]) for k, v in results.items() if v == "fail" and k in meta]
passed = sum(1 for v in results.values() if v == "pass")

if not failures:
    print(f"All tests passed ({passed} passed).")
    sys.exit(0)

print(f"{len(failures)} FAILED, {passed} passed:\n")
for key, m in failures:
    print(f'FAIL: {m["test"]} ({m["pkg"]}) [{m["elapsed"]:.2f}s]')
    out = "".join(outputs.get(key, []))
    for line in out.splitlines():
        stripped = line.strip()
        if stripped and not stripped.startswith("=== RUN") and not stripped.startswith("--- FAIL"):
            print(f"  {line}")
    print()

sys.exit(1)
