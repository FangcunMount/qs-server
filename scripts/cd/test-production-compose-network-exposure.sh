#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
compose_file="${repo_root}/build/docker/docker-compose.prod.yml"

require_mapping() {
  local mapping="$1"
  if ! grep -Fq -- "- \"${mapping}\"" "${compose_file}"; then
    echo "missing loopback-only production port mapping: ${mapping}" >&2
    exit 1
  fi
}

reject_mapping() {
  local mapping="$1"
  if grep -Fq -- "- \"${mapping}\"" "${compose_file}"; then
    echo "production port mapping must not listen on every interface: ${mapping}" >&2
    exit 1
  fi
}

require_mapping "127.0.0.1:8081:8080"
require_mapping "127.0.0.1:9445:8443"
require_mapping "127.0.0.1:19090:9090"

reject_mapping "8081:8080"
reject_mapping "0.0.0.0:8081:8080"
reject_mapping "8081:8080/tcp"
reject_mapping "9445:8443"
reject_mapping "0.0.0.0:9445:8443"
reject_mapping "9445:8443/tcp"

echo "[OK] production apiserver host ports are loopback-only"
