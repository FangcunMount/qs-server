#!/bin/sh
# Draft watchdog for the approved M5-06 window. Do not deploy or start before review.
set -eu

if [ "${1:-}" != "--execute" ]; then
  echo "requires --execute after the scoped production review" >&2
  exit 64
fi

tool=/root/rm-m5-authz-tool-linux
expected=824defd5012780be40eccfcfc45ec2f7f2ce7708707a9d14c2af98012ce93bdb
container=qs-apiserver
installed=/tmp/rm-m5-authz-tool
endpoint=iam-apiserver:9090
ca=/etc/qs-server/ssl/grpc/ca/ca-chain.crt
cert=/etc/qs-server/ssl/grpc/server/qs-apiserver.crt
key=/etc/qs-server/ssl/grpc/server/qs-apiserver.key
deadline=$(($(date +%s) + 600))
seen_revoked=0

actual=$(timeout 5s sha256sum "$tool" | awk '{print $1}') || exit 78
if [ "$actual" != "$expected" ]; then
  echo "role recovery tool hash mismatch" >&2
  exit 78
fi

while [ "$(date +%s)" -lt "$deadline" ]; do
  if [ "$(timeout 5s docker inspect "$container" --format '{{.State.Running}}' 2>/dev/null || true)" = true ]; then
    if timeout 10s docker cp "$tool" "$container:$installed" >/dev/null 2>&1 &&
       timeout 5s docker exec -u 0 "$container" chown 2000:2000 "$installed" >/dev/null 2>&1 &&
       timeout 5s docker exec -u 0 "$container" chmod 0700 "$installed" >/dev/null 2>&1; then
      snapshot=$(timeout 15s docker exec "$container" "$installed" "$endpoint" "$ca" "$cert" "$key" inspect 2>/dev/null || true)
      if [ -n "$snapshot" ]; then
        if printf '%s\n' "$snapshot" | jq -e '(.roles | sort) == (["qs:assessment_operator", "qs:result_reviewer"] | sort) and (.assignment_scopes | length) == 2' >/dev/null 2>&1; then
          if [ "$seen_revoked" -eq 1 ]; then
            echo "original role facts restored and verified"
            exit 0
          fi
          # A timer may fire before the planned revoke. Keep watching.
        elif printf '%s\n' "$snapshot" | jq -e '.roles == ["qs:result_reviewer"] and (.assignment_scopes | length) == 1' >/dev/null 2>&1; then
          seen_revoked=1
          # The tool reads the current policy version and refuses changed scopes.
          timeout 15s docker exec "$container" "$installed" "$endpoint" "$ca" "$cert" "$key" restore >/dev/null 2>&1 || true
        else
          echo "unexpected role facts; refuse automatic replacement" >&2
          exit 70
        fi
      fi
    fi
  fi
  sleep 5
done

echo "role recovery not verified within ten minutes; manual intervention required" >&2
exit 1
