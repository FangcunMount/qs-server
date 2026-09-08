#!/usr/bin/env python3
"""Reject executable/configuration remnants of the retired authentication method."""
from pathlib import Path
import re
import subprocess

ROOT = Path(__file__).resolve().parents[1]
PATTERN = re.compile(r"\b(?:ServiceToken(?:Issuer)?|IssueServiceToken(?:Request|Response)?|NewServiceToken|NewVerifiedServiceClaims|TokenTypeService|TOKEN_TYPE_SERVICE|ServiceAuthHelper|ServiceAuthConfig|IAMServiceAuthOptions)\b|auth/serviceauth|\bservice-auth\s*:")
errors = []
paths = set(subprocess.check_output(["git", "ls-files", "--cached", "--others", "--exclude-standard"], cwd=ROOT, text=True).splitlines())
for relative in sorted(paths):
    path = ROOT / relative
    if not path.is_file() or path.suffix not in {".go", ".proto", ".yaml", ".yml"}:
        continue
    if path.name.endswith("_test.go"):
        continue  # Negative contract tests intentionally name retired wire types.
    for number, line in enumerate(path.read_text().splitlines(), 1):
        if 'reserved "TOKEN_TYPE_SERVICE";' in line or path.name == "authn.pb.go":
            continue  # Generated descriptors retain protobuf reservation metadata.
        if PATTERN.search(line):
            errors.append(f"{relative}:{number}: retired authentication reference")
        if path.suffix == ".go" and '"github.com/FangcunMount/iam/v3/' in line:
            errors.append(f"{relative}:{number}: obsolete module import")
if errors:
    raise SystemExit("\n".join(errors))
print("ServiceToken retirement contract passed.")
