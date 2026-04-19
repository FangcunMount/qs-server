#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONFIG_FILE="${SEEDDATA_CONFIG:-$ROOT_DIR/configs/seeddata.yaml}"
GO_BIN="${SEEDDATA_GO:-go}"
LOG_FILE="${SEEDDATA_LOG_FILE:-$ROOT_DIR/logs/seeddata-daemon.log}"

mkdir -p "$(dirname "$LOG_FILE")"
cd "$ROOT_DIR"
{
  echo "[$(date '+%Y-%m-%d %H:%M:%S %z')] start seeddata daemon config=$CONFIG_FILE"
  exec "$GO_BIN" run ./cmd/seeddata --config "$CONFIG_FILE"
} >>"$LOG_FILE" 2>&1
