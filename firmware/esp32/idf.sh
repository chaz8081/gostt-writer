#!/usr/bin/env bash
# Wrapper for idf.py that sources ESP-IDF's export.sh under real bash.
# Needed because Taskfile v3.46 uses gosh (built-in shell) which doesn't
# propagate PATH changes from sourced scripts.
#
# Usage: ./idf.sh build
#        ./idf.sh -p /dev/cu.wchusbserial10 flash monitor

set -euo pipefail

# Default to the repo-local submodule; allow override via IDF_PATH env var.
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
IDF_PATH="${IDF_PATH:-${REPO_ROOT}/third_party/esp-idf}"
export IDF_PATH

if [ ! -d "$IDF_PATH" ]; then
  echo "ESP-IDF not found at $IDF_PATH" >&2
  echo "Run: git submodule update --init third_party/esp-idf && third_party/esp-idf/install.sh esp32s3" >&2
  exit 1
fi

# Source export.sh to get idf.py and toolchain on PATH (suppress noise)
. "$IDF_PATH/export.sh" > /dev/null 2>&1

exec idf.py "$@"
