#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MODULE_DIR="$ROOT_DIR/tools/seeddata-runner"

echo "seeddata moved to tools/seeddata-runner; forwarding to module-local script" >&2
cd "$MODULE_DIR"
exec ./scripts/run_seeddata_daemon.sh
