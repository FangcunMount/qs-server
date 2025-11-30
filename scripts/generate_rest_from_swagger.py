#!/usr/bin/env python3
"""
根据 swagger.json（v2）生成 api/rest/<service>.yaml 的 OAS 3.1 摘要，减少手工维护。

功能要点：
1) 从 swagger v2 读取 info/tags/paths/definitions/securityDefinitions。
2) 自动转换为 OpenAPI 3.1 结构，处理 $ref (#/definitions -> #/components/schemas)。
3) 剥离 basePath 前缀，保持 api/rest 与 compare 脚本一致的路径形态。
4) requestBody：将 in: body/formData 合并为 application/json；其他参数直接保留。
5) servers：可通过 --server 指定多个；若未指定则使用 basePath 或 "/"。

用法示例：
  python scripts/generate_rest_from_swagger.py \\
    --swagger api/apiserver/swagger.json \\
    --output  api/rest/apiserver.yaml \\
    --server http://localhost:8081 \\
    --server https://api.example.com
"""
from __future__ import annotations

import argparse
import json
from copy import deepcopy
from pathlib import Path
from typing import Any, Dict, List, Tuple

import yaml


def rewrite_refs(obj: Any) -> Any:
    """递归替换 $ref 引用到 components/schemas。"""
    if isinstance(obj, dict):
        new = {}
        for k, v in obj.items():
            if k == "$ref" and isinstance(v, str) and v.startswith("#/definitions/"):
                new[k] = v.replace("#/definitions/", "#/components/schemas/")
            else:
                new[k] = rewrite_refs(v)
        return new
    if isinstance(obj, list):
        return [rewrite_refs(i) for i in obj]
    return obj


def to_request_body(params: List[Dict[str, Any]]) -> Tuple[Dict[str, Any] | None, List[Dict[str, Any]]]:
    """将 body/formData 参数转为 requestBody，其余参数原样返回。"""
    body_param = None
    form_params: List[Dict[str, Any]] = []
    normal_params: List[Dict[str, Any]] = []
    for p in params:
        if p.get("in") == "body":
            body_param = p
        elif p.get("in") == "formData":
            form_params.append(p)
        else:
            normal_params.append(p)

    if body_param:
        schema = rewrite_refs(body_param.get("schema", {}))
        return (
            {
                "required": body_param.get("required", False),
                "content": {"application/json": {"schema": schema}},
            },
            normal_params,
        )

    if form_params:
        properties = {}
        required = []
        for p in form_params:
            name = p.get("name", "")
            properties[name] = {k: v for k, v in p.items() if k not in {"in", "name", "required"}}
            if p.get("required"):
                required.append(name)
        schema: Dict[str, Any] = {"type": "object", "properties": rewrite_refs(properties)}
        if required:
            schema["required"] = required
        return (
            {
                "required": any(p.get("required") for p in form_params),
                "content": {"application/json": {"schema": schema}},
            },
            normal_params,
        )

    return None, normal_params


def convert(swagger: Dict[str, Any], servers: List[str]) -> Dict[str, Any]:
    base_path = swagger.get("basePath", "") or ""
    base_path = base_path.rstrip("/") or ""

    oas: Dict[str, Any] = {
        "openapi": "3.1.0",
        "info": swagger.get("info", {}),
        "tags": swagger.get("tags", []),
        "paths": {},
        "components": {"schemas": rewrite_refs(swagger.get("definitions", {}))},
    }

    # servers
    if servers:
        oas["servers"] = [{"url": s.rstrip("/") + (base_path or "")} for s in servers]
    else:
        oas["servers"] = [{"url": base_path or "/"}]

    # securitySchemes
    sec_defs = swagger.get("securityDefinitions") or {}
    if sec_defs:
        oas["components"]["securitySchemes"] = sec_defs

    # paths
    for raw_path, methods in swagger.get("paths", {}).items():
        norm_path = raw_path
        if base_path and norm_path.startswith(base_path):
            norm_path = norm_path[len(base_path) :] or "/"
        if not norm_path.startswith("/"):
            norm_path = "/" + norm_path

        oas.setdefault("paths", {}).setdefault(norm_path, {})

        for method, spec in methods.items():
            if method.lower() not in {"get", "post", "put", "delete", "patch", "options", "head"}:
                continue

            op = {
                k: deepcopy(spec.get(k))
                for k in ["tags", "summary", "description", "operationId", "deprecated"]
                if spec.get(k) is not None
            }

            params = deepcopy(spec.get("parameters", []))
            request_body, remain_params = to_request_body(params)
            if remain_params:
                op["parameters"] = rewrite_refs(remain_params)
            if request_body:
                op["requestBody"] = request_body

            responses = {}
            for code, resp in (spec.get("responses") or {}).items():
                resp_copy = deepcopy(resp)
                schema = resp_copy.pop("schema", None)
                if schema:
                    content_type = "application/json"
                    resp_copy.setdefault("content", {})[content_type] = {
                        "schema": rewrite_refs(schema),
                    }
                responses[code] = rewrite_refs(resp_copy)
            if responses:
                op["responses"] = responses

            oas["paths"][norm_path][method.lower()] = op

    return oas


def main():
    parser = argparse.ArgumentParser(description="Generate OAS 3.1 rest docs from swagger.json")
    parser.add_argument("--swagger", required=True, help="Path to swagger.json (v2)")
    parser.add_argument("--output", required=True, help="Output yaml path under api/rest")
    parser.add_argument(
        "--server",
        action="append",
        default=[],
        help="Server URL (can be specified multiple times). If omitted, uses basePath or '/'.",
    )
    args = parser.parse_args()

    swagger_path = Path(args.swagger)
    swagger_data = json.loads(swagger_path.read_text())
    oas = convert(swagger_data, args.server)

    output_path = Path(args.output)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(yaml.safe_dump(oas, sort_keys=False, allow_unicode=True))
    print(f"Generated: {output_path}")


if __name__ == "__main__":
    main()
