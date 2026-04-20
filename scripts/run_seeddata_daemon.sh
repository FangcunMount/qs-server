#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MODULE_DIR="${SEEDDATA_ROOT:-$ROOT_DIR/../seeddata-runner}"

echo "seeddata moved to sibling repo: $MODULE_DIR; forwarding to module-local script" >&2
cd "$MODULE_DIR"
exec ./scripts/run_seeddata_daemon.sh
