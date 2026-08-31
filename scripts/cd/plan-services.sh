#!/usr/bin/env bash
set -Eeuo pipefail

EVENT_NAME="${EVENT_NAME:-${GITHUB_EVENT_NAME:-}}"
MANUAL_SERVICE="${MANUAL_SERVICE:-}"
DEPLOY_SHA="${DEPLOY_SHA:-${GITHUB_SHA:-}}"
LAST_DEPLOYED_SHA="${LAST_DEPLOYED_SHA:-}"

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

is_non_runtime_path() {
  case "$1" in
    docs/*|*.md)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

only_non_runtime_changes_since_last_deploy() {
  local changed_path changed_count=0

  if [ -z "$LAST_DEPLOYED_SHA" ]; then
    echo "No trusted production deployment baseline was found; using a conservative full deploy."
    return 1
  fi
  if ! git rev-parse --verify "${LAST_DEPLOYED_SHA}^{commit}" >/dev/null 2>&1; then
    echo "Production deployment baseline is not available locally: ${LAST_DEPLOYED_SHA}; using a conservative full deploy."
    return 1
  fi
  if ! git merge-base --is-ancestor "$LAST_DEPLOYED_SHA" "$DEPLOY_SHA"; then
    echo "Production deployment baseline is not an ancestor of the target: baseline=${LAST_DEPLOYED_SHA} target=${DEPLOY_SHA}; using a conservative full deploy."
    return 1
  fi

  while IFS= read -r changed_path; do
    [ -z "$changed_path" ] && continue
    changed_count=$((changed_count + 1))
    if ! is_non_runtime_path "$changed_path"; then
      echo "Runtime-affecting path changed since the last full production deploy: ${changed_path}"
      return 1
    fi
  done < <(git diff --name-only "$LAST_DEPLOYED_SHA" "$DEPLOY_SHA")

  if [ "$changed_count" -eq 0 ]; then
    echo "Target ${DEPLOY_SHA} is already recorded as the last full production deploy."
  else
    echo "Only documentation changed since the last full production deploy (${LAST_DEPLOYED_SHA}..${DEPLOY_SHA})."
  fi
  return 0
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

if only_non_runtime_changes_since_last_deploy; then
  echo "Skipping service deployment because production runtime bytes are unchanged."
  emit_outputs
  exit 0
fi

echo "Automatic deploy target is the current main HEAD; deploying all services: ${DEPLOY_SHA}"
add_service all
emit_outputs
