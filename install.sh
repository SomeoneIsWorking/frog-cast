#!/usr/bin/env bash
# Build frog-cast and install it into /Applications.
set -euo pipefail

APP_NAME="frog-cast.app"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BUILT_APP="$SCRIPT_DIR/build/bin/$APP_NAME"
DEST="/Applications/$APP_NAME"

cd "$SCRIPT_DIR"

echo "==> Building (wails build)"
wails build

if [ ! -d "$BUILT_APP" ]; then
  echo "ERROR: expected build output not found at $BUILT_APP" >&2
  exit 1
fi

# Quit only the installed app, via a normal macOS Quit, not a name match that
# could also hit a dev build or anything else called frog-cast.
INSTALLED_BIN="$DEST/Contents/MacOS/frog-cast"
WAS_RUNNING=0
if pgrep -fx "$INSTALLED_BIN" >/dev/null; then
  WAS_RUNNING=1
  echo "==> Quitting the installed app"
  # osascript can report an error when the app exits before replying; the
  # loop below is what decides whether it actually quit.
  osascript -e "tell application \"$DEST\" to quit" >/dev/null 2>&1 || true
  for _ in $(seq 1 20); do
    pgrep -fx "$INSTALLED_BIN" >/dev/null || break
    sleep 0.5
  done
  if pgrep -fx "$INSTALLED_BIN" >/dev/null; then
    echo "ERROR: the installed app didn't quit; quit it manually and rerun" >&2
    exit 1
  fi
fi

echo "==> Installing to $DEST"
rm -rf "$DEST"
cp -R "$BUILT_APP" "$DEST"

echo "==> Done. Installed $APP_NAME to /Applications"

if [ "$WAS_RUNNING" = 1 ]; then
  echo "==> Reopening it"
  open "$DEST"
fi
