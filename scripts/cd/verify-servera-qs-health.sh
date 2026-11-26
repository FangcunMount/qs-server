#!/usr/bin/env bash
set -Eeuo pipefail

export PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin${PATH:+:$PATH}"

case "${EXPECTED_COLLECTION_REPLICAS:-}" in
  ''|0|*[!0-9]*)
    echo "EXPECTED_COLLECTION_REPLICAS must be a positive integer" >&2
    exit 1
    ;;
esac

if ! sudo docker info >/dev/null 2>&1; then
  echo "Docker daemon is unavailable" >&2
  exit 1
fi

# Take one running-container snapshot so the count and readiness checks use
# the same replica identities. Compose release checks and diagnostic
# containers can share the project prefix, so require the service and name.
inventory=$(sudo docker ps --no-trunc --format '{{.ID}}|{{.Names}}|{{.Label "com.docker.compose.project"}}|{{.Label "com.docker.compose.service"}}|{{.Label "com.docker.compose.oneoff"}}')
selection=$(printf '%s\n' "$inventory" | awk -F'|' '
  $5 ~ /^[Tt][Rr][Uu][Ee]$/ { next }
  $2 == "qs-apiserver" { print "api", "legacy", $1; next }
  $3 == "qs-collection" && $4 == "server" { print "collection", "legacy", $1; next }
  $3 !~ /^qs-m5-[a-f0-9]+(-r[0-9]+)?-(apiserver|collection)$/ { next }
  {
    generation = $3
    sub(/-(apiserver|collection)$/, "", generation)
    if ($3 == generation "-apiserver" && $4 == "apiserver-1" && $2 == $3 "-1")
      print "api", generation, $1
    if ($3 == generation "-collection" && $4 ~ /^collection-[1-9][0-9]*$/ &&
        $2 == generation "-" $4 && !seen[$3 FS $4]++)
      print "collection", generation, $1
  }
')

api_count=$(printf '%s\n' "$selection" | awk '$1=="api"{n++} END{print n+0}')
if [ "$api_count" -ne 1 ]; then
  echo "Expected one running QS API instance, found $api_count" >&2
  exit 1
fi
APISERVER_CONTAINER=$(printf '%s\n' "$selection" | awk '$1=="api"{print $3}')
export APISERVER_CONTAINER
generation=$(printf '%s\n' "$selection" | awk '$1=="api"{print $2}')
collection_ids=$(printf '%s\n' "$selection" | awk -v generation="$generation" '
  $1 == "collection" { if ($2 != generation) mismatch = 1; print $3 }
  END { if (mismatch) exit 1 }
') || { echo "QS API and Collection instances use different layouts" >&2; exit 1; }
collection_count=$(printf '%s\n' "$collection_ids" | awk 'NF{n++} END{print n+0}')
if [ "$collection_count" -ne "$EXPECTED_COLLECTION_REPLICAS" ]; then
  echo "Expected $EXPECTED_COLLECTION_REPLICAS running Collection replicas, found $collection_count" >&2
  exit 1
fi
printf 'QS running layout %s: API %s, Collection %s\n' "$generation" "$api_count" "$collection_count"

failed=0

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8081/healthz 2>/dev/null || echo "000")
if [ "$HTTP_CODE" = "200" ]; then
  echo "✅ API Server HTTP (8081/healthz): OK"
else
  echo "❌ API Server HTTP (8081/healthz): Failed ($HTTP_CODE)"
  failed=1
fi

ready_count=0
for container_id in $collection_ids; do
  if sudo docker exec "$container_id" wget -qO- \
    http://127.0.0.1:8080/serve-readyz >/dev/null 2>&1; then
    ready_count=$((ready_count + 1))
  else
    echo "❌ Collection replica ${container_id} serving readiness failed"
  fi
done
if [ "$ready_count" -eq "$EXPECTED_COLLECTION_REPLICAS" ]; then
  echo "✅ Collection replicas serving-ready (${ready_count}/${EXPECTED_COLLECTION_REPLICAS})"
else
  echo "❌ Collection replicas serving-ready (${ready_count}/${EXPECTED_COLLECTION_REPLICAS})"
  failed=1
fi

nginx_version=$(sudo docker exec nginx nginx -v 2>&1 | sed -n 's#^nginx version: nginx/##p')
if [ -z "$nginx_version" ] ||
  [ "$(printf '%s\n' 1.27.3 "$nginx_version" | sort -V | head -n1)" != "1.27.3" ]; then
  echo "❌ Nginx version ${nginx_version:-unknown} is below required 1.27.3"
  failed=1
else
  echo "✅ Nginx version gate: ${nginx_version} >= 1.27.3"
fi

nginx_effective=$(sudo docker exec nginx nginx -T 2>&1 || true)
upstream_count=$(grep -Ec '^[[:space:]]*upstream[[:space:]]+collect-api[[:space:]]*\{' <<<"$nginx_effective" || true)
collect_upstream=$(awk '
    /^[[:space:]]*upstream[[:space:]]+collect-api[[:space:]]*\{/ { inside = 1 }
    inside {
      print
      if ($0 ~ /^[[:space:]]*}[[:space:]]*$/) inside = 0
    }
  ' <<<"$nginx_effective")
if [ "$upstream_count" -eq 1 ] &&
  printf '%s\n' "$collect_upstream" | grep -Fq 'resolver 127.0.0.11 valid=10s ipv6=off;' &&
  printf '%s\n' "$collect_upstream" | grep -Fq 'resolver_timeout 5s;' &&
  printf '%s\n' "$collect_upstream" | grep -Fq 'server qs-collection-server:8080 resolve;' &&
  ! printf '%s\n' "$collect_upstream" |
    grep -Eq '(^|[[:space:]])ip_hash[[:space:]]*;|weight[[:space:]]*=|(^|[[:space:]])backup([[:space:];]|$)'; then
  echo "✅ Effective collect-api upstream uses Docker DNS"
else
  echo "❌ Effective collect-api upstream contract failed"
  failed=1
fi

collection_ips=""
for container_id in $collection_ids; do
  collection_ip=$(sudo docker inspect "$container_id" \
    --format '{{with index .NetworkSettings.Networks "infra-network"}}{{.IPAddress}}{{end}}')
  collection_ips="${collection_ips}${collection_ip}\n"
done
collection_ips=$(printf '%b' "$collection_ips" | sed '/^$/d' | sort -u)
resolved_ips=$(sudo docker exec nginx getent ahostsv4 qs-collection-server 2>/dev/null |
  awk '$1 ~ /^[0-9]+([.][0-9]+){3}$/ {print $1}' | sort -u || true)
if [ "$resolved_ips" = "$collection_ips" ] &&
  [ "$(printf '%s\n' "$resolved_ips" | sed '/^$/d' | wc -l | tr -d '[:space:]')" -eq "$EXPECTED_COLLECTION_REPLICAS" ]; then
  echo "✅ Nginx Docker DNS resolves every collection replica"
else
  echo "❌ Nginx Docker DNS mismatch"
  echo "Expected:"
  printf '%s\n' "$collection_ips"
  echo "Resolved:"
  printf '%s\n' "$resolved_ips"
  failed=1
fi

HTTP_CODE=$(curl -sS --resolve collect.fangcunmount.cn:443:127.0.0.1 \
  -o /dev/null -w "%{http_code}" https://collect.fangcunmount.cn/health 2>/dev/null || true)
if [ "$HTTP_CODE" = "200" ]; then
  echo "✅ Collection Nginx route: OK"
else
  echo "❌ Collection Nginx route: Failed (${HTTP_CODE:-000})"
  failed=1
fi

if [ "$failed" -ne 0 ]; then
  exit 1
fi
echo "=========================================="
