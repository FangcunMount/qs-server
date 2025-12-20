#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
02_apply_factor_design.py

Apply the factor/rule design workbook to a scales YAML.

- Removes deprecated field: scale.interpretation (global interpretation).
- Rebuilds scale.factors from workbook's 'factors' and 'rules' sheets.
- Keeps other fields untouched.
"""

import argparse
import json
import hashlib
from typing import Any, Dict, List
import yaml
import pandas as pd

def _stable_id(text: str, n: int = 10) -> str:
    h = hashlib.sha1(text.encode("utf-8")).hexdigest()
    return h[:n]

def _parse_json_or_default(s: Any, default):
    if s is None:
        return default
    if isinstance(s, (dict, list)):
        return s
    s = str(s).strip()
    if s == "":
        return default
    try:
        return json.loads(s)
    except Exception:
        return default

def _split_codes(s: Any) -> List[str]:
    if s is None:
        return []
    if isinstance(s, list):
        return [str(x).strip() for x in s if str(x).strip()]
    text = str(s).strip()
    if text == "":
        return []
    return [x.strip() for x in text.split(",") if x.strip()]

def _rules_for_factor(df_rules: pd.DataFrame, scale_code: str, factor_code: str) -> List[Dict[str, Any]]:
    sub = df_rules[(df_rules["enabled"] == 1) & (df_rules["scale_code"] == scale_code) & (df_rules["factor_code"] == factor_code)]
    rows = []
    if sub.empty:
        return rows
    sub = sub.sort_values(by=["min_score", "max_score"], kind="stable")
    for _, r in sub.iterrows():
        min_score = str(r.get("min_score", "")).strip()
        max_score = str(r.get("max_score", "")).strip()
        risk = str(r.get("risk_level", "")).strip()
        conclusion = str(r.get("conclusion", "")).strip()
        suggestion = str(r.get("suggestion", "")).strip()

        parts = []
        if risk:
            parts.append(f"风险等级：{risk}")
        if conclusion:
            parts.append(f"结论：{conclusion}")
        if suggestion:
            parts.append(f"建议：{suggestion}")
        content = "\n".join(parts).strip() or conclusion or ""

        rows.append({"start": min_score, "end": max_score, "content": content})
    return rows

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("-i", "--input", required=True, help="Input scales YAML path")
    ap.add_argument("-x", "--xlsx", required=True, help="Factor design workbook path (.xlsx)")
    ap.add_argument("-o", "--output", required=True, help="Output scales YAML path")
    args = ap.parse_args()

    with open(args.input, "r", encoding="utf-8") as f:
        scales = yaml.safe_load(f)
    if not isinstance(scales, list):
        raise SystemExit("ERROR: YAML top-level must be a list of scales")

    df_factors = pd.read_excel(args.xlsx, sheet_name="factors")
    df_rules = pd.read_excel(args.xlsx, sheet_name="rules")

    df_factors["enabled"] = df_factors["enabled"].fillna(0).astype(int)
    df_rules["enabled"] = df_rules["enabled"].fillna(0).astype(int)

    by_scale: Dict[str, List[pd.Series]] = {}
    for _, r in df_factors[df_factors["enabled"] == 1].iterrows():
        sc = str(r.get("scale_code", "")).strip()
        if sc:
            by_scale.setdefault(sc, []).append(r)

    updated = 0
    for s in scales:
        # 删除全局 interpretation（你要求删除概念）
        if "interpretation" in s:
            s.pop("interpretation", None)

        scode = str(s.get("code", "")).strip()
        if not scode or scode not in by_scale:
            continue

        new_factors = []
        for r in by_scale[scode]:
            fcode = str(r.get("factor_code", "")).strip()
            title = str(r.get("factor_title", "")).strip()
            if not fcode:
                fcode = "auto-" + _stable_id(scode + "|" + title)

            ftype = str(r.get("factor_type", "first_grade")).strip() or "first_grade"
            is_total = str(r.get("is_total_score", "0")).strip() or "0"
            formula = str(r.get("formula", "sum")).strip() or "sum"
            max_score = str(r.get("max_score", "")).strip()
            is_show = str(r.get("is_show", "0")).strip() or "0"

            append_params = _parse_json_or_default(r.get("append_params_json", "[]"), [])
            calc_rule = {"formula": formula, "append_params": append_params}
            source_codes = _split_codes(r.get("source_codes", ""))

            blocks = _rules_for_factor(df_rules, scode, fcode)
            interpret_rule = {"is_show": is_show, "interpretation": blocks}

            new_factors.append({
                "code": fcode,
                "type": ftype,
                "title": title,
                "is_total_score": is_total,
                "source_codes": source_codes,
                "calc_rule": calc_rule,
                "max_score": max_score,
                "interpret_rule": interpret_rule,
            })

        s["factors"] = new_factors
        updated += 1

    with open(args.output, "w", encoding="utf-8") as f:
        yaml.safe_dump(scales, f, sort_keys=False, allow_unicode=True, width=120)

    print(f"OK: updated {updated} scales and wrote -> {args.output}")
    print("Scales not present in workbook were left unchanged (except global interpretation removed).")

if __name__ == "__main__":
    main()
