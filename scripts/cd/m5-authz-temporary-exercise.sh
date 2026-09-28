#!/usr/bin/env bash
# One-off, fixed-subject M5-06 authorization window. Remove after evidence is captured.
set -Eeuo pipefail

fail() { echo "$*" >&2; exit 1; }
: "${EXPECTED_API_SHA:?}"
: "${ROLE_WINDOW_MODE:?}"
[[ "$EXPECTED_API_SHA" =~ ^[0-9a-f]{40}$ ]] || fail 'invalid API SHA'
container=qs-apiserver
image=$(sudo -n docker inspect "$container" --format '{{.Config.Image}}')
health=$(sudo -n docker inspect "$container" --format '{{.State.Health.Status}}')
[[ "$image" == *":${EXPECTED_API_SHA}" && "$health" == healthy ]] || fail 'API image or health changed'

tool=/root/rm-m5-authz-tool-linux
restore=/root/rm-m5-role-restore.sh
inside=/tmp/rm-m5-authz-tool
endpoint=iam-apiserver:9090
ca=/etc/qs-server/ssl/grpc/ca/ca-chain.crt
cert=/etc/qs-server/ssl/grpc/server/qs-apiserver.crt
key=/etc/qs-server/ssl/grpc/server/qs-apiserver.key
run_tool() {
  sudo -n docker exec "$container" "$inside" "$endpoint" "$ca" "$cert" "$key" "$1"
}
assert_original() {
  local snap
  snap=$(run_tool inspect)
  jq -e '(.roles | sort) == (["qs:assessment_operator", "qs:result_reviewer"] | sort) and (.assignment_scopes | length) == 2' <<<"$snap" >/dev/null || fail 'original scoped roles absent'
  echo "$snap" | jq '{time,policy_version,roles,assignment_scopes}'
}

case "$ROLE_WINDOW_MODE" in
  inspect-role-sudo)
    sudo -n -l || true
    ls -lh /tmp/rm-m5-20260928-stage/m5-authz-temporary-tool-linux /tmp/rm-m5-20260928-stage/m5-authz-temporary-restore-live.sh
    ;;
  prepare-role-window)
    [[ -f /tmp/rm-m5-20260928-stage/m5-authz-temporary-tool-linux ]] || fail 'staged tool missing'
    [[ -f /tmp/rm-m5-20260928-stage/m5-authz-temporary-restore-live.sh ]] || fail 'staged restore missing'
    ! sudo -n systemctl list-timers --all --no-legend --no-pager | grep -Eq 'rm-m5-.*role-restore' || fail 'existing role recovery timer'
    sudo -n install -o root -g root -m 0700 /tmp/rm-m5-20260928-stage/m5-authz-temporary-tool-linux "$tool"
    sudo -n install -o root -g root -m 0700 /tmp/rm-m5-20260928-stage/m5-authz-temporary-restore-live.sh "$restore"
    expected=$(sudo -n awk -F= '/^expected=/{print $2}' "$restore")
    actual=$(sudo -n sha256sum "$tool" | awk '{print $1}')
    [[ "$actual" == "$expected" ]] || fail 'installed tool hash mismatch'
    sudo -n docker cp "$tool" "$container:$inside"
    sudo -n docker exec -u 0 "$container" chown 2000:2000 "$inside"
    sudo -n docker exec -u 0 "$container" chmod 0700 "$inside"
    assert_original
    run_tool plan-revoke | jq '{time,mode,request:{subject:.request.subject,org_id:.request.org_id,expected_policy_version:.request.expected_policy_version,roles:.request.roles}}'
    echo 'PASS role recovery files installed, original roles inspected, revoke planned; no mutation'
    ;;
  revoke-role-window)
    expected=$(sudo -n awk -F= '/^expected=/{print $2}' "$restore")
    actual=$(sudo -n sha256sum "$tool" | awk '{print $1}')
    [[ "$actual" == "$expected" ]] || fail 'installed tool hash mismatch'
    ! sudo -n systemctl list-timers --all --no-legend --no-pager | grep -Eq 'rm-m5-.*role-restore' || fail 'existing role recovery timer'
    assert_original
    run_tool plan-revoke >/dev/null
    unit="rm-m5-${ROLE_WINDOW_RUN_ID}-role-restore"
    sudo -n systemd-run --unit="$unit" --on-active=90s --collect /bin/sh "$restore" --execute
    sudo -n systemctl is-active "${unit}.timer" | grep -Fxq active || fail 'role recovery timer not active; stop without revoking'
    echo "RECOVERY_TIMER_ACTIVE unit=$unit time=$(date --iso-8601=seconds)"
    run_tool revoke || {
      echo 'REVOKE_OUTCOME_UNKNOWN; inspect role snapshot; recovery timer remains active' >&2
      run_tool inspect || true
      exit 1
    }
    echo "ROLE_REVOKE_RETURNED time=$(date --iso-8601=seconds)"
    ;;
  inspect-role-window)
    run_tool inspect | jq '{time,policy_version,roles,assignment_scopes}'
    sudo -n systemctl list-timers --all --no-legend --no-pager | grep -E 'rm-m5-.*role-restore' || true
    ;;
  cleanup-role-window)
    assert_original
    ! sudo -n systemctl list-timers --all --no-legend --no-pager | grep -Eq 'rm-m5-.*role-restore' || fail 'recovery timer still active'
    sudo -n rm -f "$tool" "$restore"
    sudo -n docker exec -u 0 "$container" rm -f "$inside"
    rm -f /tmp/rm-m5-20260928-stage/m5-authz-temporary-tool-linux /tmp/rm-m5-20260928-stage/m5-authz-temporary-restore-live.sh
    echo 'PASS original scoped roles restored; temporary recovery files removed'
    ;;
  *) fail 'unsupported role-window phase' ;;
esac
