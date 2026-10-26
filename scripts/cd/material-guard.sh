#!/usr/bin/env bash
# Normal deploy-user client. Only the installed digest-selected tool runs as root.
M4_MATERIAL_TOOL_SHA256=36f3046ed453dfbf8beae684269c94d1a6db0abaa23de51e09d41e462c6a3d5c

m4_material_preflight() {
  local role="$1" revision="$2" host tool report
  case "$role" in
    iam|qs-api|ai) host=serverA ;;
    worker) host=serverD ;;
    *) echo 'Invalid material role; refusing deployment' >&2; return 1 ;;
  esac
  if ! [[ "$revision" =~ ^m4-[a-z0-9][a-z0-9-]{0,59}$ ]]; then
    echo 'Invalid material revision; refusing deployment' >&2
    return 1
  fi
  tool="/opt/fangcun-m4/material-tools/${M4_MATERIAL_TOOL_SHA256}.py"
  if ! python3 -I - "$tool" "$M4_MATERIAL_TOOL_SHA256" <<'PYTOOL'
import hashlib,os,stat,sys
from pathlib import Path
def require(value):
    if not value:raise ValueError
try:
    p=Path(sys.argv[1])
    require(p.resolve()==p)
    for parent in p.parents:
        i=parent.lstat()
        require(stat.S_ISDIR(i.st_mode) and i.st_uid==0 and i.st_mode&0o022==0)
    i=p.lstat()
    require(stat.S_ISREG(i.st_mode) and i.st_uid==0 and stat.S_IMODE(i.st_mode)==0o555 and 0<i.st_size<=65536)
    require(hashlib.sha256(p.read_bytes()).hexdigest()==sys.argv[2])
except Exception:
    raise SystemExit('Reviewed material tool unavailable; refusing deployment') from None
PYTOOL
  then return 1; fi
  report=$(mktemp "$DEPLOY_TMP/.m4-material-${role}.XXXXXX") || return 1
  # The parent already holds the original deployment lock. This entry is read-only.
  if ! $SUDO /usr/bin/python3 -I "$tool" --verify-installed --host "$host" \
    --expect-role "$role" --expect-revision "$revision" > "$report"; then return 1; fi
  python3 -I - "$report" "$role" "$revision" "$host" <<'PYRESULT'
import json,re,sys
from pathlib import Path
def require(value):
    if not value:raise ValueError
try:
    raw=Path(sys.argv[1]).read_bytes()
    require(0<len(raw)<=32768)
    x=json.loads(raw)
    require(x['status']=='verified' and x['read_only'] is True and x['activated'] is False)
    require((x['role'],x['revision'],x['host'])==tuple(sys.argv[2:5]))
    require(x['material_root']=='/var/lib/fangcun-m4-materials')
    require(re.fullmatch('[a-f0-9]{64}',x['receipt_sha256']))
except Exception:
    raise SystemExit('Installed materials not verified; refusing deployment') from None
PYRESULT
}
