#!/bin/sh
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../../../../.." && pwd)
cd "$ROOT/docs/adr/data/0001/gate2/compat"
# Isolated analysis tools: never modify the Electron workspace dependencies.
pnpm install --ignore-workspace --frozen-lockfile
node compat.mjs
