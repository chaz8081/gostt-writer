#!/usr/bin/env bash
# Wrapper for idf.py that sources ESP-IDF's export.sh under real bash.
# Needed because Taskfile v3.46 uses gosh (built-in shell) which doesn't
# propagate PATH changes from sourced scripts.
#
# Usage: ./idf.sh build
#        ./idf.sh -p /dev/cu.wchusbserial10 flash monitor

set -euo pipefail

IDF_PATH="${IDF_PATH:-${HOME}/github/espressif/esp-idf}"

if [ ! -d "$IDF_PATH" ]; then
  echo "ESP-IDF not found at $IDF_PATH" >&2
  echo "Set IDF_PATH or install: https://docs.espressif.com/projects/esp-idf/en/stable/esp32s3/get-started/" >&2
  exit 1
fi

# Source export.sh to get idf.py and toolchain on PATH (suppress noise)
. "$IDF_PATH/export.sh" > /dev/null 2>&1

exec idf.py "$@"
