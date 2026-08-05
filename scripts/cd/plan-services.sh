#!/usr/bin/env bash
set -Eeuo pipefail

EVENT_NAME="${EVENT_NAME:-${GITHUB_EVENT_NAME:-}}"
MANUAL_SERVICE="${MANUAL_SERVICE:-}"
DEPLOY_SHA="${DEPLOY_SHA:-${GITHUB_SHA:-}}"

want_apiserver=0
want_collection=0
want_worker=0

add_service() {
  case "$1" in
    apiserver) want_apiserver=1 ;;
    collection) want_collection=1 ;;
    worker) want_worker=1 ;;
    all)
      want_apiserver=1
      want_collection=1
      want_worker=1
      ;;
    *)
      echo "Unsupported service in deploy plan: $1" >&2
      exit 1
      ;;
  esac
}

services_json() {
  local sep=""
  printf '['
  if [ "$want_apiserver" -eq 1 ]; then
    printf '%s"apiserver"' "$sep"
    sep=","
  fi
  if [ "$want_collection" -eq 1 ]; then
    printf '%s"collection"' "$sep"
    sep=","
  fi
  if [ "$want_worker" -eq 1 ]; then
    printf '%s"worker"' "$sep"
  fi
  printf ']'
}

bool_text() {
  if [ "$1" -eq 1 ]; then
    printf 'true'
  else
    printf 'false'
  fi
}

emit_outputs() {
  local services has_services
  services="$(services_json)"
  has_services=false
  if [ "$services" != "[]" ]; then
    has_services=true
  fi

  echo "Deploy services: ${services}"

  if [ -n "${GITHUB_OUTPUT:-}" ]; then
    {
      printf 'services=%s\n' "$services"
      printf 'has_services=%s\n' "$has_services"
      printf 'apiserver=%s\n' "$(bool_text "$want_apiserver")"
      printf 'collection=%s\n' "$(bool_text "$want_collection")"
      printf 'worker=%s\n' "$(bool_text "$want_worker")"
    } >>"$GITHUB_OUTPUT"
  fi
}

current_main_sha() {
  local ref
  for ref in refs/remotes/origin/main refs/heads/main; do
    if git rev-parse --verify "${ref}^{commit}" >/dev/null 2>&1; then
      git rev-parse "${ref}^{commit}"
      return 0
    fi
  done
}

if [ "$EVENT_NAME" = "workflow_dispatch" ]; then
  add_service "${MANUAL_SERVICE:-all}"
  emit_outputs
  exit 0
fi

if [ -z "$DEPLOY_SHA" ]; then
  echo "DEPLOY_SHA is required for workflow_run deploy planning" >&2
  exit 1
fi

main_sha="$(current_main_sha || true)"
if [ -z "$main_sha" ]; then
  echo "Cannot resolve the current main SHA; refusing automatic deployment." >&2
  exit 1
fi

if [ "$DEPLOY_SHA" != "$main_sha" ]; then
  echo "Skipping stale automatic deploy target: target=${DEPLOY_SHA} current_main=${main_sha}"
  emit_outputs
  exit 0
fi

echo "Automatic deploy target is the current main HEAD; deploying all services: ${DEPLOY_SHA}"
add_service all
emit_outputs
