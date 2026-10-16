#!/bin/sh
# NextUI starts tools by running this script.
PAK_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$PAK_DIR" || exit 1

export LD_LIBRARY_PATH="$PAK_DIR/lib:$LD_LIBRARY_PATH"

LOGS_PATH="${LOGS_PATH:-$USERDATA_PATH/logs}"
mkdir -p "$LOGS_PATH"

# Restores NextUI's brightness and volume once the app has opened the display.
syncsettings.elf &

# The app writes its own log; this file only catches crashes before logging starts.
exec ./boombrick > "$LOGS_PATH/Boombrick.crash.txt" 2>&1
