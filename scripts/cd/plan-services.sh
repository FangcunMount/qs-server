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

previous_successful_deploy_sha() {
  if [ -z "${GITHUB_TOKEN:-}" ] || [ -z "${GITHUB_REPOSITORY:-}" ] || ! command -v gh >/dev/null 2>&1; then
    return 0
  fi

  GH_TOKEN="$GITHUB_TOKEN" gh api --method GET \
    "repos/${GITHUB_REPOSITORY}/actions/workflows/cd.yml/runs" \
    -f branch=main \
    -f event=workflow_run \
    -f status=success \
    -f per_page=10 \
    --jq '.workflow_runs[].head_sha' 2>/dev/null \
    | awk -v head="$DEPLOY_SHA" '$0 != head { print; exit }'
}

resolve_base_sha() {
  local previous_sha
  previous_sha="$(previous_successful_deploy_sha || true)"
  if [ -n "$previous_sha" ] && git cat-file -e "${previous_sha}^{commit}" 2>/dev/null; then
    printf '%s\n' "$previous_sha"
    return 0
  fi

  if [ -n "$DEPLOY_SHA" ] && git rev-parse --verify "${DEPLOY_SHA}^" >/dev/null 2>&1; then
    git rev-parse "${DEPLOY_SHA}^"
  fi
}

classify_path() {
  local path="$1"

  case "$path" in
    ""|*.md|Makefile|docs/*|MONGODB_*.md|.github/*|scripts/cd/*|scripts/perf/*|scripts/oneoff/*|scripts/cert/*)
      return 0
      ;;
    *_test.go)
      return 0
      ;;
    go.mod|go.sum|go.work|go.work.sum|configs/events.yaml|build/docker/docker-compose.prod.yml|internal/pkg/*|pkg/*)
      add_service all
      return 0
      ;;
    cmd/qs-apiserver/*|internal/apiserver/*|configs/apiserver.*|build/docker/Dockerfile.qs-apiserver|api/rest/apiserver.yaml)
      add_service apiserver
      return 0
      ;;
    cmd/collection-server/*|internal/collection-server/*|configs/collection-server.*|build/docker/Dockerfile.collection-server|api/rest/collection.yaml)
      add_service collection
      return 0
      ;;
    cmd/qs-worker/*|internal/worker/*|configs/worker.*|build/docker/Dockerfile.qs-worker)
      add_service worker
      return 0
      ;;
    web/swagger-ui/*)
      add_service apiserver
      add_service collection
      return 0
      ;;
    *.go|*.proto|cmd/*|internal/*|pkg/*|configs/*|build/docker/*)
      add_service all
      return 0
      ;;
  esac
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

base_sha="$(resolve_base_sha || true)"
if [ -z "$base_sha" ]; then
  echo "No comparable base deploy found; deploying all services."
  add_service all
  emit_outputs
  exit 0
fi

echo "Planning deploy range: ${base_sha}..${DEPLOY_SHA}"
changed_files="$(git diff --name-only "$base_sha" "$DEPLOY_SHA")"
if [ -z "$changed_files" ]; then
  echo "No file changes detected in deploy range."
  emit_outputs
  exit 0
fi

echo "Changed files:"
printf '%s\n' "$changed_files" | sed 's/^/  - /'

while IFS= read -r changed_file; do
  classify_path "$changed_file"
done <<<"$changed_files"

if [ "$(services_json)" = "[]" ]; then
  echo "No deployable service changes detected; production deploy will be skipped."
fi

emit_outputs
