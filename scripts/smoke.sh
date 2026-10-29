#!/usr/bin/env bash
# Launch a packaged Bistro with --smoke. The app exits 0 once migrations ran and
# the window finished its first load; non-zero on error or after 60 s.
# Usage: scripts/smoke.sh <path-to-binary-or-.app>
set -euo pipefail
if [[ $# != 1 ]]; then
  echo "Usage: $0 <path-to-binary-or-.app>" >&2
  exit 1
fi
target="$1"
if [[ -d "$target" && "$target" == *.app ]]; then
  bundle="$(cd "$target" && pwd)"
  target="$bundle/Contents/MacOS/$(defaults read "$bundle/Contents/Info" CFBundleExecutable)"
fi
runner=()
if [[ "$(uname -s)" == Linux ]]; then runner=(xvfb-run -a); fi
timeout_command=timeout
if ! command -v "$timeout_command" >/dev/null 2>&1; then timeout_command=gtimeout; fi
# --smoke uses its own temporary data dir and never syncs (Task 14).
# macOS ships Bash 3.2, where nounset rejects an empty array expansion.
"$timeout_command" 90 ${runner[@]+"${runner[@]}"} "$target" --smoke
echo "smoke ok"
