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
    echo "retired Node 20 GitHub Action major version is still referenced" >&2
    exit 1
  fi
}

assert_action_input() {
  local action_ref=$1
  local input=$2
  local workflow_file=$3
  if ! awk -v action="uses: ${action_ref}" -v input="$input" '
    index($0, action) { in_action = 1; next }
    in_action && /^[[:space:]]+- name:/ { exit }
    in_action && index($0, input) { found = 1 }
    END { exit found ? 0 : 1 }
  ' "$workflow_file"; then
    echo "required GitHub Action input is missing for ${action_ref}: ${input}" >&2
    exit 1
  fi
}

assert_current_action 'actions/upload-artifact@v7'
assert_current_action 'docker/login-action@v4'
assert_current_action 'docker/setup-buildx-action@v4'
assert_current_action 'codecov/codecov-action@v7'

assert_retired_action_absent 'uses:[[:space:]]+actions/upload-artifact@v([1-6])([^0-9]|$)'
assert_retired_action_absent 'uses:[[:space:]]+docker/login-action@v([1-3])([^0-9]|$)'
assert_retired_action_absent 'uses:[[:space:]]+docker/setup-buildx-action@v([1-3])([^0-9]|$)'
assert_retired_action_absent 'uses:[[:space:]]+codecov/codecov-action@v([1-6])([^0-9]|$)'

CI_WORKFLOW="$WORKFLOW_ROOT/ci.yml"
assert_action_input 'codecov/codecov-action@v7' 'use_oidc: true' "$CI_WORKFLOW"
assert_action_input 'codecov/codecov-action@v7' 'fail_ci_if_error: true' "$CI_WORKFLOW"
if ! grep -Fq -- 'id-token: write' "$CI_WORKFLOW"; then
  echo "Codecov OIDC requires id-token: write in ci.yml" >&2
  exit 1
fi

echo "GitHub Action runtime contract passed"
