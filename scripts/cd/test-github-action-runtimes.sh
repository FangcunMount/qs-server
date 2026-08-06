#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)
WORKFLOW_ROOT="$REPO_ROOT/.github/workflows"
shopt -s nullglob
WORKFLOW_FILES=("$WORKFLOW_ROOT"/*.yml "$WORKFLOW_ROOT"/*.yaml)
if [ "${#WORKFLOW_FILES[@]}" -eq 0 ]; then
  echo "no GitHub Actions workflow files found under $WORKFLOW_ROOT" >&2
  exit 1
fi

assert_current_action() {
  local action_ref=$1
  if ! grep -Fq -- "uses: ${action_ref}" "${WORKFLOW_FILES[@]}"; then
    echo "required GitHub Action runtime contract is missing: ${action_ref}" >&2
    exit 1
  fi
}

assert_retired_action_absent() {
  local pattern=$1
  if grep -n -E -- "$pattern" "${WORKFLOW_FILES[@]}"; then
    echo "retired or unsupported GitHub Action reference is still present" >&2
    exit 1
  fi
}

assert_current_action 'actions/upload-artifact@v7'
assert_current_action 'docker/login-action@v4'
assert_current_action 'docker/setup-buildx-action@v4'

assert_retired_action_absent 'uses:[[:space:]]+actions/upload-artifact@v([1-6])([^0-9]|$)'
assert_retired_action_absent 'uses:[[:space:]]+docker/login-action@v([1-3])([^0-9]|$)'
assert_retired_action_absent 'uses:[[:space:]]+docker/setup-buildx-action@v([1-3])([^0-9]|$)'
assert_retired_action_absent 'uses:[[:space:]]+codecov/codecov-action@'

echo "GitHub Action runtime contract passed"
