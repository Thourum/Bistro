#!/bin/bash
set -euo pipefail

# Both builds use the same seed. No app build or package step is hidden here.
HARNESS_DIR="$(cd "$(dirname "$0")" && pwd)"
SEED_DIR="${BISTRO_BENCH_SEED:-/tmp/bistro-bench-seed}"
if [[ $# -eq 0 || "$1" == "--help" ]]; then
  echo "Usage: $0 --app <Bistro.app> --label <build> --out <empty-dir> [--runs N] [--dmg <file>]"
  echo "BISTRO_BENCH_SEED selects a reusable seed directory (default /tmp/bistro-bench-seed)."
  exit 0
fi

OUT_DIR=""
ARGS=("$@")
for ((i=0; i<${#ARGS[@]}; i++)); do
  if [[ "${ARGS[$i]}" == "--out" ]]; then
    OUT_DIR="${ARGS[$((i+1))]:-}"
  fi
done
if [[ -z "$OUT_DIR" ]]; then echo "--out is required" >&2; exit 2; fi
if [[ ! -f "$SEED_DIR/seed-manifest.json" ]]; then
  node "$HARNESS_DIR/seed.mjs" --out "$SEED_DIR"
fi
node "$HARNESS_DIR/run-electron.mjs" --seed "$SEED_DIR" "$@"
node "$HARNESS_DIR/stats.mjs" "$OUT_DIR"
