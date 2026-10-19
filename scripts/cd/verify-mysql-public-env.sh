#!/usr/bin/env bash
set -euo pipefail
: "${MYSQL_HOST:?Organization MYSQL_PUBLIC_HOST is required}"
: "${MYSQL_PORT:?Organization MYSQL_PUBLIC_PORT is required}"
: "${MYSQL_TLS_CA_REF:?Organization MYSQL_TLS_CA_REF is required}"
[ "${MYSQL_TLS_MODE:-}" = VERIFY_IDENTITY ] || { echo 'Production requires MySQL VERIFY_IDENTITY' >&2; exit 1; }
[[ "$MYSQL_TLS_CA_REF" =~ ^[0-9a-f]{40}$ ]] || { echo 'CA source must be an immutable infra SHA' >&2; exit 1; }
python3 - <<'CHECK'
import ipaddress,os,socket
host=os.environ['MYSQL_HOST'];port=int(os.environ['MYSQL_PORT'])
assert 0<port<65536 and host not in {'mysql-rds-proxy','localhost','mysql'}
addresses=[ipaddress.ip_address(x[4][0]) for x in socket.getaddrinfo(host,port,type=socket.SOCK_STREAM)]
assert addresses and all(a.is_global for a in addresses), 'Production MySQL must resolve to public addresses'
print('Organization public MySQL / TLS policy verified')
CHECK
