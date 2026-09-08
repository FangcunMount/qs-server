#!/usr/bin/env bash
# Verify against a local IAM v4 checkout before the v4.0.0 tag is published.
set -euo pipefail
qs_root=$(cd "$(dirname "$0")/.." && pwd)
iam_root=$(cd "${1:-$qs_root/../iam}" && pwd)
verification_dir=$(mktemp -d)
trap 'rm -rf "$verification_dir"' EXIT
export GOWORK="$verification_dir/go.work"
(cd "$verification_dir" && go work init "$iam_root" "$qs_root")
go work edit "-replace=github.com/FangcunMount/iam/v4@v4.0.0=$iam_root"
cd "$qs_root"
python3 scripts/check_service_token_retirement.py
go test ./...
go test -tags=integration -run '^$' ./...
python3 scripts/check_docs_facts.py
