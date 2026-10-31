#!/usr/bin/env bash
# Historical NSQ/Relay proof; the current host is verified by native RocketMQ M3-C.
set -euo pipefail
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
exec bash "$script_dir/run-retired-outbox-contracts.sh" mysql-outcome-script
