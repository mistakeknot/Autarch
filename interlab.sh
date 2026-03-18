#!/usr/bin/env bash
set -euo pipefail
# apps/Autarch/interlab.sh — wraps Autarch Go benchmarks for interlab consumption.
# Primary metric: scoring_batch_ns (BenchmarkScoreBatch50)
# Secondary: diff_specs_ns, diff_format_ns

MONOREPO="$(cd "$(dirname "$0")/../.." && pwd)"
HARNESS="${INTERLAB_HARNESS:-$MONOREPO/interverse/interlab/scripts/go-bench-harness.sh}"
DIR="$(cd "$(dirname "$0")" && pwd)"

echo "--- pollard scoring ---" >&2
bash "$HARNESS" --pkg ./internal/pollard/scoring/ --bench 'BenchmarkScoreBatch50$' --metric scoring_batch_ns --dir "$DIR"

echo "--- gurgeh diff ---" >&2
bash "$HARNESS" --pkg ./internal/gurgeh/diff/ --bench 'BenchmarkDiffSpecs50$' --metric diff_specs_ns --dir "$DIR"

echo "--- gurgeh format ---" >&2
bash "$HARNESS" --pkg ./internal/gurgeh/diff/ --bench 'BenchmarkFormatDiff$' --metric diff_format_ns --dir "$DIR"
