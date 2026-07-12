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
import re
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
    items: Set[Tuple[str, str]] = set()
    for path, methods in paths.items():
        for method in methods.keys():
            if method.lower() not in {"get", "post", "put", "delete", "patch", "options", "head"}:
                continue
            items.add((method.upper(), path))
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

    quality_errors = validate_openapi(rest_path)
    if quality_errors:
        print("→ OpenAPI quality errors:")
        for error in quality_errors:
            print(f"  {error}")
    else:
        print("→ OpenAPI operationId, path parameter, description, security and error-response checks passed.")

    return not missing_in_rest and not missing_in_swagger and not quality_errors


def validate_openapi(file_path: Path) -> list[str]:
    """Validate generated-contract invariants that are not covered by path diff."""
    data = yaml.safe_load(file_path.read_text()) or {}
    errors: list[str] = []
    if not data.get("security"):
        errors.append("missing root security requirement")

    operation_ids: dict[str, tuple[str, str]] = {}
    paths = data.get("paths", {}) or {}
    for path, methods in paths.items():
        expected_path_params = set(re.findall(r"\{([^}]+)\}", path))
        for method, operation in methods.items():
            if method.lower() not in {"get", "post", "put", "delete", "patch", "options", "head"}:
                continue
            operation = operation or {}
            operation_id = operation.get("operationId")
            label = f"{method.upper()} {path}"
            if not operation_id:
                errors.append(f"{label}: missing operationId")
            elif operation_id in operation_ids:
                errors.append(f"{label}: duplicate operationId {operation_id!r} (also {operation_ids[operation_id][0]} {operation_ids[operation_id][1]})")
            else:
                operation_ids[operation_id] = (method.upper(), path)

            actual_path_params = {
                parameter.get("name")
                for parameter in operation.get("parameters", []) or []
                if parameter.get("in") == "path"
            }
            if expected_path_params != actual_path_params:
                errors.append(f"{label}: path parameters {sorted(actual_path_params)} != template {sorted(expected_path_params)}")
            for parameter in operation.get("parameters", []) or []:
                if parameter.get("in") == "path" and parameter.get("required") is not True:
                    errors.append(f"{label}: path parameter {parameter.get('name')!r} must be required")

            if not operation.get("description"):
                errors.append(f"{label}: missing description")
            responses = {str(code): value for code, value in (operation.get("responses") or {}).items()}
            if "500" not in responses:
                errors.append(f"{label}: missing standard 500 error response")
            if operation.get("security", data.get("security")):
                for status in ("401", "403"):
                    if status not in responses:
                        errors.append(f"{label}: missing {status} security error response")
    return errors


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
