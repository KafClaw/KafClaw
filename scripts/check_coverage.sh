#!/usr/bin/env bash
set -euo pipefail

# Check the complete Go profile; do not exclude packages or source files.
python3 - "${1:-coverage.out}" "${2:-85}" <<'PY'
import sys
from decimal import Decimal

profile, minimum = sys.argv[1], Decimal(sys.argv[2])
blocks = {}
with open(profile, encoding="utf-8") as source:
    header = next(source).strip()
    if header not in {"mode: set", "mode: count", "mode: atomic"}:
        raise SystemExit(f"Invalid coverage profile: {header}")
    for line in source:
        location, statements, hits = line.split()
        statements, hits = int(statements), int(hits)
        # Integration tests can exercise a package from several test binaries.
        previous = blocks.get(location, (statements, 0))
        if previous[0] != statements:
            raise SystemExit(f"Inconsistent coverage block: {location}")
        blocks[location] = (statements, max(previous[1], hits))

total = sum(statements for statements, _ in blocks.values())
covered = sum(statements for statements, hits in blocks.values() if hits > 0)
if total == 0:
    raise SystemExit("Coverage profile contains no statements")
percent = Decimal(covered) * 100 / Decimal(total)
print(f"Go coverage: {percent:.2f}% ({covered}/{total} statements; minimum {minimum}%)")
if percent < minimum:
    raise SystemExit("Coverage gate failed")
PY
