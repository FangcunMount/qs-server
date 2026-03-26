#!/usr/bin/env python3
"""
Compare REST docs in api/rest/* (OpenAPI 3.1) against generated Swagger files.
Focuses on path/method coverage to spot mismatches quickly.

Usage:
  python scripts/compare_api_docs.py            # compare apiserver + collection
  python scripts/compare_api_docs.py --service apiserver

Requires: PyYAML (`pip install pyyaml`)
"""
from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Dict, Iterable, Set, Tuple

import yaml

ROOT = Path(__file__).resolve().parent.parent
INTERNAL_SWAGGER_DIRS = {
    "apiserver": "apiserver",
    "collection": "collection-server",
}


def load_paths_from_openapi(file_path: Path) -> Set[Tuple[str, str]]:
    data = yaml.safe_load(file_path.read_text())
    paths: Dict[str, Dict] = data.get("paths", {}) or {}
    items: Set[Tuple[str, str]] = set()
    for path, methods in paths.items():
        for method in methods.keys():
            if method.lower() not in {"get", "post", "put", "delete", "patch", "options", "head"}:
                continue
            items.add((method.upper(), path))
    return items


def load_paths_from_swagger(file_path: Path) -> Set[Tuple[str, str]]:
    data = json.loads(file_path.read_text())
    paths: Dict[str, Dict] = data.get("paths", {}) or {}
    base_path = data.get("basePath", "") or ""
    base_path = base_path.rstrip("/")
    items: Set[Tuple[str, str]] = set()
    for path, methods in paths.items():
        for method in methods.keys():
            if method.lower() not in {"get", "post", "put", "delete", "patch", "options", "head"}:
                continue
            # Normalize by stripping basePath for comparison
            normalized = path
            if base_path and normalized.startswith(base_path):
                normalized = normalized[len(base_path) :]
            items.add((method.upper(), normalized))
    return items


def compare(service: str) -> bool:
    rest_path = ROOT / "api" / "rest" / f"{service}.yaml"
    # Prefer new internal swagger outputs; fall back to legacy api/<service>/swagger.json if present.
    internal_dir = INTERNAL_SWAGGER_DIRS.get(service, service)
    swagger_path = ROOT / "internal" / internal_dir / "docs" / "swagger.json"
    legacy_swagger_path = ROOT / "api" / service / "swagger.json"
    if not swagger_path.exists() and legacy_swagger_path.exists():
        swagger_path = legacy_swagger_path
    if not rest_path.exists():
        print(f"[{service}] missing REST doc: {rest_path}")
        return False
    if not swagger_path.exists():
        print(f"[{service}] missing swagger json: {swagger_path}")
        return False

    rest_paths = load_paths_from_openapi(rest_path)
    swagger_paths = load_paths_from_swagger(swagger_path)

    missing_in_rest = sorted(swagger_paths - rest_paths)
    missing_in_swagger = sorted(rest_paths - swagger_paths)

    print(f"\n=== {service} ===")
    print(f"REST paths: {len(rest_paths)}, swagger paths: {len(swagger_paths)}")
    if missing_in_rest:
        print("→ In swagger.json but NOT in api/rest:")
        for method, path in missing_in_rest:
            print(f"  {method} {path}")
    else:
        print("→ No swagger-only paths.")

    if missing_in_swagger:
        print("→ In api/rest but NOT in swagger.json:")
        for method, path in missing_in_swagger:
            print(f"  {method} {path}")
    else:
        print("→ No rest-only paths.")

    return not missing_in_rest and not missing_in_swagger


def main():
    parser = argparse.ArgumentParser(description="Compare REST docs vs swagger outputs")
    parser.add_argument(
        "--service",
        choices=["apiserver", "collection"],
        nargs="*",
        help="services to check (default: both)",
    )
    args = parser.parse_args()
    services: Iterable[str] = args.service or ["apiserver", "collection"]
    ok = True
    for svc in services:
        ok = compare(svc) and ok
    return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(main())
