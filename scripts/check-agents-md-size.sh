#!/usr/bin/env bash
# check-agents-md-size.sh — Detect AGENTS.md bloat and suggest extraction
#
# Checks if AGENTS.md exceeds a line threshold and identifies sections
# that are candidates for extraction to docs/reference/ topic files.
#
# Usage:
#   ./scripts/check-agents-md-size.sh           # Check and report
#   ./scripts/check-agents-md-size.sh --strict   # Exit 1 if over threshold
#
# Thresholds (adjustable):
#   WARN at 450 lines, FAIL at 550 lines (--strict mode only)

set -euo pipefail

AGENTS_MD="${AGENTS_MD:-AGENTS.md}"
WARN_THRESHOLD="${WARN_THRESHOLD:-450}"
FAIL_THRESHOLD="${FAIL_THRESHOLD:-550}"
STRICT=false

[[ "${1:-}" == "--strict" ]] && STRICT=true

if [[ ! -f "$AGENTS_MD" ]]; then
    echo "AGENTS.md not found at: $AGENTS_MD" >&2
    exit 1
fi

LINE_COUNT=$(wc -l < "$AGENTS_MD" | tr -d ' ')

# Count intermem entries (auto-memory annotations that tend to accumulate)
INTERMEM_COUNT=$(grep -c '<!-- intermem:' "$AGENTS_MD" 2>/dev/null || true)
INTERMEM_COUNT=${INTERMEM_COUNT:-0}

# Count H2 sections
SECTION_COUNT=$(grep -c '^## ' "$AGENTS_MD" 2>/dev/null || echo 0)

# Find sections over 40 lines (candidates for extraction)
echo "=== AGENTS.md Health Check ==="
echo "Lines: $LINE_COUNT (warn: $WARN_THRESHOLD, fail: $FAIL_THRESHOLD)"
echo "Sections: $SECTION_COUNT"
echo "Intermem annotations: $INTERMEM_COUNT"
echo ""

if (( INTERMEM_COUNT > 5 )); then
    echo "WARNING: $INTERMEM_COUNT intermem annotations found."
    echo "  These are auto-memory entries that should be extracted to docs/reference/ topic files."
    echo "  Run: grep -n 'intermem:' AGENTS.md"
    echo ""
fi

# Measure each H2 section length
echo "=== Section Sizes ==="
prev_line=0
prev_heading=""
while IFS= read -r line; do
    line_num="${line%%:*}"
    heading="${line#*:}"
    if (( prev_line > 0 )); then
        section_lines=$(( line_num - prev_line ))
        marker=""
        if (( section_lines > 60 )); then
            marker=" ← EXTRACT CANDIDATE"
        elif (( section_lines > 40 )); then
            marker=" ← growing"
        fi
        printf "  %3d lines  %s%s\n" "$section_lines" "$prev_heading" "$marker"
    fi
    prev_line=$line_num
    prev_heading="$heading"
done < <(grep -n '^## ' "$AGENTS_MD"; echo "$LINE_COUNT:EOF")

echo ""

if (( LINE_COUNT > FAIL_THRESHOLD )); then
    echo "FAIL: AGENTS.md is $LINE_COUNT lines (threshold: $FAIL_THRESHOLD)."
    echo "  Extract large sections to docs/reference/ and link from the Documentation Map."
    if $STRICT; then exit 1; fi
elif (( LINE_COUNT > WARN_THRESHOLD )); then
    echo "WARN: AGENTS.md is $LINE_COUNT lines (threshold: $WARN_THRESHOLD)."
    echo "  Consider extracting sections marked 'EXTRACT CANDIDATE' above."
else
    echo "OK: AGENTS.md is $LINE_COUNT lines — within healthy range."
fi
