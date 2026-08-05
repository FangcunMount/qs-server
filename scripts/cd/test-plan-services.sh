#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
TEST_ROOT=$(mktemp -d)
trap 'rm -rf "$TEST_ROOT"' EXIT

REPO="$TEST_ROOT/repo"
OUTPUT="$TEST_ROOT/github-output"
mkdir -p "$REPO"
git -C "$REPO" init -q
git -C "$REPO" config user.name qs-server-ci
git -C "$REPO" config user.email qs-server-ci@example.invalid
git -C "$REPO" commit --allow-empty -qm base
BASE_SHA=$(git -C "$REPO" rev-parse HEAD)
git -C "$REPO" commit --allow-empty -qm head
HEAD_SHA=$(git -C "$REPO" rev-parse HEAD)
git -C "$REPO" update-ref refs/remotes/origin/main "$HEAD_SHA"

assert_output() {
  local expected="$1"
  if ! grep -Fxq "$expected" "$OUTPUT"; then
    echo "missing plan output: $expected" >&2
    cat "$OUTPUT" >&2
    exit 1
  fi
}

: >"$OUTPUT"
(
  cd "$REPO"
  EVENT_NAME=workflow_run DEPLOY_SHA="$HEAD_SHA" GITHUB_OUTPUT="$OUTPUT" \
    "$SCRIPT_DIR/plan-services.sh"
)
assert_output 'services=["apiserver","collection","worker"]'
assert_output 'has_services=true'

: >"$OUTPUT"
(
  cd "$REPO"
  EVENT_NAME=workflow_run DEPLOY_SHA="$BASE_SHA" GITHUB_OUTPUT="$OUTPUT" \
    "$SCRIPT_DIR/plan-services.sh"
)
assert_output 'services=[]'
assert_output 'has_services=false'

: >"$OUTPUT"
(
  cd "$REPO"
  EVENT_NAME=workflow_dispatch MANUAL_SERVICE=collection DEPLOY_SHA="$BASE_SHA" GITHUB_OUTPUT="$OUTPUT" \
    "$SCRIPT_DIR/plan-services.sh"
)
assert_output 'services=["collection"]'
assert_output 'collection=true'
assert_output 'apiserver=false'
assert_output 'worker=false'
