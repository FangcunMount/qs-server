#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)
WORKFLOW="$REPO_ROOT/.github/workflows/db-ops.yml"

require_literal() {
  local value=$1
  if ! grep -Fq -- "$value" "$WORKFLOW"; then
    echo "AI evaluation size audit workflow contract is missing: $value" >&2
    exit 1
  fi
}

require_literal "- audit-ai-evaluation-size"
require_literal "github.event.inputs.operation == 'audit-ai-evaluation-size'"
require_literal "./scripts/oneoff/audit_ai_explanation_prompt_evaluation_size"
require_literal "CGO_ENABLED=0 GOOS=linux GOARCH=amd64"
require_literal "--network infra-network"
require_literal "--read-only"
require_literal "--cap-drop ALL"
require_literal "--security-opt no-new-privileges:true"
require_literal "--max-runs=0"
require_literal 'MONGO_PASSWORD: ${{ secrets.MONGODB_PASSWORD }}'

if grep -n -E -- 'echo .*MONGO_(URI|USERNAME|PASSWORD)|set -x' "$WORKFLOW"; then
  echo "AI evaluation size audit workflow must not log Mongo credentials" >&2
  exit 1
fi

echo "AI evaluation size audit workflow contract passed"
