#!/usr/bin/env bash
set -Eeuo pipefail
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
python3 -B "$SCRIPT_DIR/test-production-inventory.py"
node --check "$SCRIPT_DIR/production-inventory.js"
node "$SCRIPT_DIR/test-production-inventory.js"
