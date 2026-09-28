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
unit=rm-m5-20260928-role-restore
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
  inspect-role-root)
    id -un
    ;;
  inspect-role-sudo)
    sudo -n -l || true
    ls -lh /tmp/rm-m5-20260928-stage/m5-authz-temporary-tool-linux /tmp/rm-m5-20260928-stage/m5-authz-temporary-restore-live.sh
    ;;
  prepare-role-window|resume-prepare-role-window)
    [[ -f /tmp/rm-m5-20260928-stage/m5-authz-temporary-tool-linux ]] || fail 'staged tool missing'
    [[ -f /tmp/rm-m5-20260928-stage/m5-authz-temporary-restore-live.sh ]] || fail 'staged restore missing'
    ! sudo -n systemctl list-timers --all --no-legend --no-pager | grep -Eq 'rm-m5-.*role-restore' || fail 'existing role recovery timer'
    sudo -n install -o root -g root -m 0700 /tmp/rm-m5-20260928-stage/m5-authz-temporary-tool-linux "$tool"
    sudo -n install -o root -g root -m 0700 /tmp/rm-m5-20260928-stage/m5-authz-temporary-restore-live.sh "$restore"
    expected=824defd5012780be40eccfcfc45ec2f7f2ce7708707a9d14c2af98012ce93bdb
    actual=$(sudo -n sha256sum "$tool" | awk '{print $1}')
    [[ "$actual" == "$expected" ]] || fail 'installed tool hash mismatch'
    cat >/tmp/rm-m5-20260928-role-restore.service <<'UNIT'
[Unit]
Description=One-time QS M5-06 Qingdao test role restoration
[Service]
Type=oneshot
User=root
ExecStart=/bin/sh /root/rm-m5-role-restore.sh --execute
TimeoutStartSec=11min
UNIT
    cat >/tmp/rm-m5-20260928-role-restore.timer <<'UNIT'
[Unit]
Description=One-time QS M5-06 Qingdao test role restoration timer
[Timer]
OnActiveSec=90s
AccuracySec=1s
Unit=rm-m5-20260928-role-restore.service
UNIT
    sudo -n install -o root -g root -m 0644 /tmp/rm-m5-20260928-role-restore.service "/run/systemd/system/${unit}.service"
    sudo -n install -o root -g root -m 0644 /tmp/rm-m5-20260928-role-restore.timer "/run/systemd/system/${unit}.timer"
    sudo -n systemctl daemon-reload
    sudo -n docker cp "$tool" "$container:$inside"
    sudo -n docker exec -u 0 "$container" chown 2000:2000 "$inside"
    sudo -n docker exec -u 0 "$container" chmod 0700 "$inside"
    assert_original
    run_tool plan-revoke | jq '{time,mode,request:{subject:.request.subject,org_id:.request.org_id,expected_policy_version:.request.expected_policy_version,roles:.request.roles}}'
    echo 'PASS role recovery files installed, original roles inspected, revoke planned; no mutation'
    ;;
  revoke-role-window)
    expected=824defd5012780be40eccfcfc45ec2f7f2ce7708707a9d14c2af98012ce93bdb
    actual=$(sudo -n sha256sum "$tool" | awk '{print $1}')
    [[ "$actual" == "$expected" ]] || fail 'installed tool hash mismatch'
    ! sudo -n systemctl list-timers --all --no-legend --no-pager | grep -Eq 'rm-m5-.*role-restore' || fail 'existing role recovery timer'
    assert_original
    run_tool plan-revoke >/dev/null
    sudo -n systemctl start "${unit}.timer"
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
  inspect-role-http)
    assert_original
    sudo -n journalctl -u "${unit}.service" --since '2026-09-28 21:47:00' --no-pager -o cat | grep -E 'original role facts restored|Started|Finished|Failed' || true
    sudo -n docker exec nginx tail -n 20000 /data/log/nginx/access.log | awk '
      /"GET \/api\/v1\/evaluations\/assessment-progress/ {
        if (match($0, /\[[^]]+\]/)) {
          stamp=substr($0,RSTART+1,RLENGTH-2)
          split($0,quotes,"\"")
          split(quotes[3],after," ")
          if (stamp ~ /28\/Sep\/2026:21:4[7-9]:/) print stamp,after[2]
        }
      }
    ' | tail -n 50
    ;;
  cleanup-role-window)
    assert_original
    sudo -n systemctl stop "${unit}.timer"
    sudo -n systemctl is-active "${unit}.service" | grep -Fxq inactive || fail 'recovery service still active'
    sudo -n docker exec -u 0 "$container" rm -f "$inside"
    sudo -n mkdir -p /dev/shm/rm-m5-20260928-cleanup
    sudo -n rsync -a --remove-source-files "$tool" "$restore" "/run/systemd/system/${unit}.service" "/run/systemd/system/${unit}.timer" /dev/shm/rm-m5-20260928-cleanup/
    sudo -n systemctl daemon-reload
    rm -f /tmp/rm-m5-20260928-stage/m5-authz-temporary-tool-linux /tmp/rm-m5-20260928-stage/m5-authz-temporary-restore-live.sh
    echo 'PASS original scoped roles restored; temporary recovery files removed'
    ;;
  *) fail 'unsupported role-window phase' ;;
esac
