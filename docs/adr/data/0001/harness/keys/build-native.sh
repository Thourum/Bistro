#!/bin/sh
set -eu
# Run with sh build-native.sh; the executable is local evidence tooling, not an app.
HERE=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
mkdir -p "$HERE/bin"
xcrun clang -fobjc-arc -Wall -Wextra -Werror -mmacosx-version-min=13.0 \
  -framework AppKit -framework ApplicationServices -framework Carbon \
  "$HERE/bistro-keys.m" -o "$HERE/bin/bistro-keys"
printf '%s\n' "$HERE/bin/bistro-keys"
