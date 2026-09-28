#!/usr/bin/env bash
# Builds the Go extension and records frames from the game with Godot's movie
# writer, for eyeballing changes without a human at the keyboard.
# Usage: scripts/capture.sh <frames> [game args...]
set -euo pipefail
cd "$(dirname "$0")/.."
frames=${1:-30}; shift || true
GODOT=${GODOT:-$HOME/gd/bin/Godot.app/Contents/MacOS/Godot}
CGO_ENABLED=1 go build -buildmode=c-shared -o graphics/darwin_arm64.dylib .
cp graphics/darwin_arm64.dylib graphics/darwin_universal.dylib
out=${CAPTURE_DIR:-/tmp/gts-capture}
rm -rf "$out" && mkdir -p "$out"
"$GODOT" --path graphics --resolution ${RES:-1280x720} --write-movie "$out/f.png" \
  --fixed-fps ${FPS:-30} --quit-after "$frames" -- "$@" 2>&1 | grep -vE '^\s*$|movie|render time|Encoding time|^-+$' || true
ls "$out" | tail -1
