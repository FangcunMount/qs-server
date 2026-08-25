#!/usr/bin/env bash
set -Eeuo pipefail

OPERATION="${QS_AUTHZ_CONSUMER_OPERATION:-}"
HOST_ROLE="${QS_AUTHZ_CONSUMER_HOST_ROLE:-}"
RELEASE_SHA="${QS_AUTHZ_RELEASE_SHA:-}"
STATE_ROOT="${QS_AUTHZ_CONSUMER_STATE_ROOT:-/opt/backups/qs-server/authz-cutover/runtime}"
STATE_FILE="${STATE_ROOT}/${RELEASE_SHA}.${HOST_ROLE}-containers"

if [ "${#RELEASE_SHA}" -ne 40 ] || [ -n "${RELEASE_SHA//[0-9a-f]/}" ]; then
  echo "QS_AUTHZ_RELEASE_SHA must be an exact lowercase 40-character Git SHA" >&2
  exit 1
fi
case "$OPERATION" in
  stop|start|status) ;;
  *) echo "unsupported qs-server AuthZ consumer operation" >&2; exit 1 ;;
esac
case "$HOST_ROLE" in
  app|worker) ;;
  *) echo "unsupported qs-server AuthZ consumer host role" >&2; exit 1 ;;
esac

if sudo -n true 2>/dev/null; then
  run_privileged() { sudo "$@"; }
else
  : "${SUDO_PASSWORD:?SUDO_PASSWORD is required when passwordless sudo is unavailable}"
  run_privileged() { printf '%s\n' "$SUDO_PASSWORD" | sudo -S "$@"; }
fi

running_ids() {
  case "$HOST_ROLE" in
    app)
      {
        run_privileged docker ps -q --filter 'name=^/qs-apiserver$'
        run_privileged docker ps -q \
          --filter 'label=com.docker.compose.project=qs-collection' \
          --filter 'label=com.docker.compose.service=server'
        run_privileged docker ps -q \
          --filter 'label=com.docker.compose.project=qs-collection' \
          --filter 'label=com.docker.compose.service=qs-collection-server'
        run_privileged docker ps -q --filter 'name=^/qs-collection-server$'
      } | awk 'NF && !seen[$0]++'
      ;;
    worker)
      {
        run_privileged docker ps -q \
          --filter 'label=com.docker.compose.project=qs-worker' \
          --filter 'label=com.docker.compose.service=runtime'
        run_privileged docker ps -q --filter 'name=^/qs-worker$'
        run_privileged docker ps --format '{{.ID}} {{.Names}}' |
          awk '$2 ~ /^qs-deploy-worker-[0-9]+-qs-worker-[0-9]+$/ { print $1 }'
      } | awk 'NF && !seen[$0]++'
      ;;
  esac
}

case "$OPERATION" in
  stop)
    run_privileged install -d -m 0750 "$STATE_ROOT"
    temporary_state="$(mktemp)"
    trap 'rm -f "$temporary_state"' EXIT
    running_ids >"$temporary_state"
    if [ ! -s "$temporary_state" ]; then
      echo "no running ${HOST_ROLE} authorization consumers were found" >&2
      exit 1
    fi
    run_privileged install -m 0640 "$temporary_state" "$STATE_FILE"
    while IFS= read -r container_id; do
      [ -n "$container_id" ] || continue
      run_privileged docker stop "$container_id"
    done <"$temporary_state"
    if [ -n "$(running_ids)" ]; then
      echo "${HOST_ROLE} authorization consumers are still running" >&2
      exit 1
    fi
    echo "qs-server AuthZ consumers stopped: role=${HOST_ROLE} state=${STATE_FILE}"
    ;;
  start)
    if ! run_privileged test -f "$STATE_FILE"; then
      echo "cutover runtime state is missing: ${STATE_FILE}" >&2
      exit 1
    fi
    while IFS= read -r container_id; do
      [ -n "$container_id" ] || continue
      run_privileged docker start "$container_id"
    done < <(run_privileged cat "$STATE_FILE")
    echo "qs-server AuthZ consumers restored: role=${HOST_ROLE} state=${STATE_FILE}"
    ;;
  status)
    count="$(running_ids | awk 'NF { count++ } END { print count + 0 }')"
    echo "qs-server AuthZ consumer status: role=${HOST_ROLE} running=${count} release_sha=${RELEASE_SHA}"
    ;;
esac
