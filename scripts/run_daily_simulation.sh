#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONFIG_FILE="${SEEDDATA_CONFIG:-$ROOT_DIR/configs/seeddata.yaml}"
STEPS="${SEEDDATA_STEPS:-daily_simulation,statistics_backfill}"
GO_BIN="${SEEDDATA_GO:-go}"
LOG_FILE="${SEEDDATA_LOG_FILE:-$ROOT_DIR/logs/seeddata-daily-simulation.log}"

mkdir -p "$(dirname "$LOG_FILE")"

cd "$ROOT_DIR"

{
  echo "[$(date '+%Y-%m-%d %H:%M:%S %z')] start seeddata steps=$STEPS config=$CONFIG_FILE"
  "$GO_BIN" run ./cmd/tools/seeddata --config "$CONFIG_FILE" --steps "$STEPS"
  echo "[$(date '+%Y-%m-%d %H:%M:%S %z')] done seeddata steps=$STEPS"
} >>"$LOG_FILE" 2>&1
