#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
01_generate_factor_design.py

Read a scales YAML and generate an Excel workbook for factor/rule design.

Workbook sheets:
- scales: scale-level audit summary
- factors: one row per factor (existing + proposed placeholders)
- rules: one row per (factor x interpret rule) in Mongo-like schema (min/max/risk/conclusion/suggestion)

Notes
- This script does NOT change YAML. It only produces the Excel workbook for editing.
- "scale.interpretation" (global interpretation) is considered deprecated; we don't rely on it here.
"""

import argparse
import json
import hashlib
from typing import Any, Dict, List, Tuple
import yaml
import pandas as pd

SCORED_TYPES = {"Radio", "Checkbox"}  # can expand later

def _stable_id(text: str, n: int = 10) -> str:
    h = hashlib.sha1(text.encode("utf-8")).hexdigest()
    return h[:n]

def _is_scored_question(q: Dict[str, Any]) -> bool:
    if q.get("type") not in SCORED_TYPES:
        return False
    opts = q.get("options") or []
    for o in opts:
        s = str(o.get("score", "")).strip()
        if s != "" and s != "0":
            return True
    ms = str(q.get("max_score", "")).strip()
    try:
        return float(ms) > 0
    except Exception:
        return False

def _question_codes(scale: Dict[str, Any]) -> List[str]:
    return [q.get("code") for q in (scale.get("questions") or []) if q.get("code")]

def _scored_question_codes(scale: Dict[str, Any]) -> List[str]:
    out = []
    for q in (scale.get("questions") or []):
        if _is_scored_question(q) and q.get("code"):
            out.append(q["code"])
    return out

def _covered_codes_by_factors(scale: Dict[str, Any]) -> set:
    covered = set()
    for f in (scale.get("factors") or []):
        for c in (f.get("source_codes") or []):
            covered.add(c)
    return covered

def _existing_factors_rows(scale: Dict[str, Any]) -> Tuple[List[Dict[str, Any]], List[Dict[str, Any]]]:
    factor_rows = []
    rule_rows = []
    for f in (scale.get("factors") or []):
        scale_code = scale.get("code", "")
        scale_name = scale.get("name", "")
        factor_code = f.get("code", "")
        factor_title = f.get("title", "")
        factor_type = f.get("type", "first_grade")
        is_total_score = str(f.get("is_total_score", "0"))
        source_codes = ",".join(f.get("source_codes") or [])
        calc_rule = f.get("calc_rule") or {}
        formula = calc_rule.get("formula", "")
        append_params = calc_rule.get("append_params", [])
        try:
            append_params_json = json.dumps(append_params, ensure_ascii=False)
        except Exception:
            append_params_json = "[]"
        max_score = str(f.get("max_score", ""))

        interpret_rule = f.get("interpret_rule") or {}
        is_show = str(interpret_rule.get("is_show", "0"))
        interpretation = interpret_rule.get("interpretation") or []

        factor_rows.append({
            "enabled": 1,
            "source": "existing",
            "scale_code": scale_code,
            "scale_name": scale_name,
            "factor_code": factor_code,
            "factor_title": factor_title,
            "factor_type": factor_type,
            "is_total_score": is_total_score,
            "formula": formula,
            "append_params_json": append_params_json,
            "source_codes": source_codes,
            "max_score": max_score,
            "is_show": is_show,
            "notes": "",
        })

        # Convert existing interpretation blocks to Mongo-like rows
        for idx, blk in enumerate(interpretation):
            rule_rows.append({
                "enabled": 1,
                "scale_code": scale_code,
                "scale_name": scale_name,
                "factor_code": factor_code,
                "factor_title": factor_title,
                "rule_index": idx,
                "min_score": str(blk.get("start", "")),
                "max_score": str(blk.get("end", "")),
                "risk_level": "",
                "conclusion": str(blk.get("content", "")).strip(),
                "suggestion": "",
            })

    return factor_rows, rule_rows

def _propose_vanderbilt(scale: Dict[str, Any]) -> Tuple[List[Dict[str, Any]], List[Dict[str, Any]]]:
    """
    First-draft factor plan for Vanderbilt based on question order.
    Uses symptom counts (经常/总是) + performance impairment counts (有点问题/有问题).
    """
    name = scale.get("name", "")
    qs = scale.get("questions") or []
    qcodes = [q.get("code") for q in qs]

    def qslice(a: int, b: int) -> List[str]:
        return [c for c in qcodes[a:b] if c]

    inatt = qslice(0, 9)
    hyper = qslice(9, 18)
    odd = qslice(18, 26)
    cd = qslice(26, 40)
    anxdep = qslice(40, 47)
    perf = qslice(47, 55)  # only exists in "+表现"

    scale_code = scale.get("code", "")
    scale_name = name
    append_cnt_sym = json.dumps({"cnt_option_contents": ["经常", "总是"]}, ensure_ascii=False)
    append_cnt_perf = json.dumps({"cnt_option_contents": ["有点问题", "有问题"]}, ensure_ascii=False)

    factors, rules = [], []

    def add_cnt_factor(title: str, codes: List[str], threshold: int, tag: str):
        fcode = "auto-" + _stable_id(scale_code + "|" + title)
        max_n = len(codes)
        factors.append({
            "enabled": 1,
            "source": "proposed",
            "scale_code": scale_code,
            "scale_name": scale_name,
            "factor_code": fcode,
            "factor_title": title,
            "factor_type": "first_grade",
            "is_total_score": "0",
            "formula": "cnt",
            "append_params_json": append_cnt_sym,
            "source_codes": ",".join(codes),
            "max_score": str(max_n),
            "is_show": "1",
            "notes": f"症状计数（经常/总是）；常用阈值：≥{threshold}",
        })
        rules.extend([
            {"enabled": 1, "scale_code": scale_code, "scale_name": scale_name, "factor_code": fcode, "factor_title": title, "rule_index": 0,
             "min_score": "0", "max_score": str(threshold - 1), "risk_level": "none",
             "conclusion": f"{tag}症状计数未达到常用筛查阈值（< {threshold}）。",
             "suggestion": "如家长/老师仍观察到明显困扰，建议结合功能受损情况与专业评估。"},
            {"enabled": 1, "scale_code": scale_code, "scale_name": scale_name, "factor_code": fcode, "factor_title": title, "rule_index": 1,
             "min_score": str(threshold), "max_score": str(max_n), "risk_level": "medium",
             "conclusion": f"{tag}症状计数达到常用筛查阈值（≥ {threshold}）。",
             "suggestion": "建议进一步专业评估（临床访谈/多信息源），并关注学校与家庭功能受损。"},
        ])

    if inatt:
        add_cnt_factor("注意缺陷症状计数", inatt, threshold=6, tag="注意缺陷")
    if hyper:
        add_cnt_factor("多动冲动症状计数", hyper, threshold=6, tag="多动/冲动")
    if odd:
        add_cnt_factor("对立违抗问题计数（ODD）", odd, threshold=4, tag="对立违抗")
    if cd:
        add_cnt_factor("品行问题计数（CD）", cd, threshold=3, tag="品行问题")
    if anxdep:
        add_cnt_factor("焦虑/抑郁问题计数", anxdep, threshold=3, tag="焦虑/抑郁")

    if "表现" in name and perf:
        fcode = "auto-" + _stable_id(scale_code + "|" + "功能受损/表现问题计数")
        factors.append({
            "enabled": 1,
            "source": "proposed",
            "scale_code": scale_code,
            "scale_name": scale_name,
            "factor_code": fcode,
            "factor_title": "功能受损/表现问题计数",
            "factor_type": "first_grade",
            "is_total_score": "0",
            "formula": "cnt",
            "append_params_json": append_cnt_perf,
            "source_codes": ",".join(perf),
            "max_score": str(len(perf)),
            "is_show": "1",
            "notes": "计数选项：有点问题/有问题（4/5分）；常见判定：≥1 存在功能受损",
        })
        rules.extend([
            {"enabled": 1, "scale_code": scale_code, "scale_name": scale_name, "factor_code": fcode, "factor_title": "功能受损/表现问题计数", "rule_index": 0,
             "min_score": "0", "max_score": "0", "risk_level": "none",
             "conclusion": "未提示明显功能受损（表现问题计数=0）。",
             "suggestion": "可结合家校观察随访；如仍有学习/社交困扰，建议进一步评估。"},
            {"enabled": 1, "scale_code": scale_code, "scale_name": scale_name, "factor_code": fcode, "factor_title": "功能受损/表现问题计数", "rule_index": 1,
             "min_score": "1", "max_score": str(len(perf)), "risk_level": "high",
             "conclusion": "提示存在功能受损（至少1项表现为“有点问题/有问题”）。",
             "suggestion": "建议结合症状计数与功能受损，尽快专业评估并制定干预计划。"},
        ])

    return factors, rules

def _propose_ygtss(scale: Dict[str, Any]) -> Tuple[List[Dict[str, Any]], List[Dict[str, Any]]]:
    qs = scale.get("questions") or []

    def find_code(title: str) -> str:
        for q in qs:
            if q.get("title") == title and q.get("type") == "Radio":
                return q.get("code", "")
        return ""

    motor_titles = ["运动性抽动发生的数量", "运动性抽动频率", "运动性抽动的强度", "运动性抽动的复杂性", "运动性抽动的干扰性"]
    vocal_titles = ["发声性抽动发生的数量", "发声性抽动频率", "发声性抽动的强度", "发声性抽动的复杂性", "发声性抽动的干扰性"]

    motor = [find_code(t) for t in motor_titles if find_code(t)]
    vocal = [find_code(t) for t in vocal_titles if find_code(t)]
    impairment = find_code("抽动症的功能损害")

    scale_code = scale.get("code", "")
    scale_name = scale.get("name", "")
    factors, rules = [], []

    def add_sum(title: str, codes: List[str], max_score: int, is_total: str, show: str):
        fcode = "auto-" + _stable_id(scale_code + "|" + title)
        factors.append({
            "enabled": 1, "source": "proposed",
            "scale_code": scale_code, "scale_name": scale_name,
            "factor_code": fcode, "factor_title": title,
            "factor_type": "first_grade",
            "is_total_score": is_total,
            "formula": "sum", "append_params_json": "[]",
            "source_codes": ",".join([c for c in codes if c]),
            "max_score": str(max_score),
            "is_show": show,
            "notes": "YGTSS：运动/发声严重度(0-25)；总抽动(0-50)；功能损害(0-50)；总体严重度(0-100)",
        })
        return fcode

    fc_motor = add_sum("运动性抽动严重度（0-25）", motor, 25, "0", "1")
    fc_vocal = add_sum("发声性抽动严重度（0-25）", vocal, 25, "0", "1")
    fc_total = add_sum("总抽动严重度（0-50）", motor + vocal, 50, "0", "1")
    fc_imp = add_sum("功能损害（0-50）", [impairment] if impairment else [], 50, "0", "1")
    fc_global = add_sum("总体严重度（0-100）", motor + vocal + ([impairment] if impairment else []), 100, "1", "1")

    # 给出可用但保守的分段（YGTSS没有统一硬阈值，避免伪权威）
    for fcode, title, mx in [(fc_total, "总抽动严重度（0-50）", 50), (fc_imp, "功能损害（0-50）", 50), (fc_global, "总体严重度（0-100）", 100)]:
        rules.extend([
            {"enabled": 1, "scale_code": scale_code, "scale_name": scale_name, "factor_code": fcode, "factor_title": title, "rule_index": 0,
             "min_score": "0", "max_score": str(int(mx * 0.24)), "risk_level": "none",
             "conclusion": "当前结果提示严重度/影响程度较轻。",
             "suggestion": "建议结合波动、触发因素、共病及临床评估综合判断。"},
            {"enabled": 1, "scale_code": scale_code, "scale_name": scale_name, "factor_code": fcode, "factor_title": title, "rule_index": 1,
             "min_score": str(int(mx * 0.25)), "max_score": str(int(mx * 0.49)), "risk_level": "medium",
             "conclusion": "当前结果提示严重度/影响程度为中等范围。",
             "suggestion": "建议制定随访频率并评估是否需要行为/药物干预；必要时转专科。"},
            {"enabled": 1, "scale_code": scale_code, "scale_name": scale_name, "factor_code": fcode, "factor_title": title, "rule_index": 2,
             "min_score": str(int(mx * 0.50)), "max_score": str(mx), "risk_level": "high",
             "conclusion": "当前结果提示严重度/影响程度较高。",
             "suggestion": "建议尽快专科评估，综合干预，并重点关注功能损害。"},
        ])

    return factors, rules

def _propose_gad7(scale: Dict[str, Any]) -> Tuple[List[Dict[str, Any]], List[Dict[str, Any]]]:
    qs = scale.get("questions") or []
    codes = [q.get("code") for q in qs if q.get("type") == "Radio" and q.get("code")]
    scale_code = scale.get("code", "")
    scale_name = scale.get("name", "")
    fcode = "auto-" + _stable_id(scale_code + "|GAD-7总分")
    factors = [{
        "enabled": 1, "source": "proposed",
        "scale_code": scale_code, "scale_name": scale_name,
        "factor_code": fcode, "factor_title": "GAD-7 总分",
        "factor_type": "first_grade", "is_total_score": "1",
        "formula": "sum", "append_params_json": "[]",
        "source_codes": ",".join(codes), "max_score": "21", "is_show": "1",
        "notes": "常用分级：0-4最小；5-9轻度；10-14中度；15-21重度",
    }]
    rules = [
        {"enabled": 1, "scale_code": scale_code, "scale_name": scale_name, "factor_code": fcode, "factor_title": "GAD-7 总分", "rule_index": 0,
         "min_score": "0", "max_score": "4", "risk_level": "none", "conclusion": "焦虑症状最小或无明显焦虑。", "suggestion": "如仍有困扰，可结合压力源与生活方式调整；必要时咨询专业人士。"},
        {"enabled": 1, "scale_code": scale_code, "scale_name": scale_name, "factor_code": fcode, "factor_title": "GAD-7 总分", "rule_index": 1,
         "min_score": "5", "max_score": "9", "risk_level": "mild", "conclusion": "焦虑症状轻度。", "suggestion": "建议自我管理并观察；如持续或影响功能，建议进一步评估。"},
        {"enabled": 1, "scale_code": scale_code, "scale_name": scale_name, "factor_code": fcode, "factor_title": "GAD-7 总分", "rule_index": 2,
         "min_score": "10", "max_score": "14", "risk_level": "medium", "conclusion": "焦虑症状中度。", "suggestion": "建议专业评估；可考虑结构化心理干预。"},
        {"enabled": 1, "scale_code": scale_code, "scale_name": scale_name, "factor_code": fcode, "factor_title": "GAD-7 总分", "rule_index": 3,
         "min_score": "15", "max_score": "21", "risk_level": "high", "conclusion": "焦虑症状重度。", "suggestion": "建议尽快专业评估与干预，并关注功能受损与风险。"},
    ]
    return factors, rules

def _propose_pss(scale: Dict[str, Any]) -> Tuple[List[Dict[str, Any]], List[Dict[str, Any]]]:
    qs = scale.get("questions") or []
    codes = [q.get("code") for q in qs if q.get("type") == "Radio" and q.get("code")]
    scale_code = scale.get("code", "")
    scale_name = scale.get("name", "")
    mx = len(codes) * 4 if codes else 40
    fcode = "auto-" + _stable_id(scale_code + "|PSS总分")
    factors = [{
        "enabled": 1, "source": "proposed",
        "scale_code": scale_code, "scale_name": scale_name,
        "factor_code": fcode, "factor_title": "压力知觉总分（PSS）",
        "factor_type": "first_grade", "is_total_score": "1",
        "formula": "sum", "append_params_json": "[]",
        "source_codes": ",".join(codes), "max_score": str(mx), "is_show": "1",
        "notes": "PSS-10 常含反向计分题；若你未在题目选项分值上处理，需要额外修正。",
    }]
    rules = [
        {"enabled": 1, "scale_code": scale_code, "scale_name": scale_name, "factor_code": fcode, "factor_title": "压力知觉总分（PSS）", "rule_index": 0,
         "min_score": "0", "max_score": str(int(mx * 0.33)), "risk_level": "none", "conclusion": "压力知觉水平较低。", "suggestion": "保持规律作息与适度运动；遇压力事件可使用放松训练。"},
        {"enabled": 1, "scale_code": scale_code, "scale_name": scale_name, "factor_code": fcode, "factor_title": "压力知觉总分（PSS）", "rule_index": 1,
         "min_score": str(int(mx * 0.34)), "max_score": str(int(mx * 0.66)), "risk_level": "medium", "conclusion": "压力知觉水平中等。", "suggestion": "建议评估主要压力源并做时间/任务管理；必要时寻求心理支持。"},
        {"enabled": 1, "scale_code": scale_code, "scale_name": scale_name, "factor_code": fcode, "factor_title": "压力知觉总分（PSS）", "rule_index": 2,
         "min_score": str(int(mx * 0.67)), "max_score": str(mx), "risk_level": "high", "conclusion": "压力知觉水平较高。", "suggestion": "建议专业评估；必要时心理干预，并关注睡眠与焦虑/抑郁共病。"},
    ]
    return factors, rules

def _propose_mchat_like(scale: Dict[str, Any]) -> Tuple[List[Dict[str, Any]], List[Dict[str, Any]]]:
    qs = scale.get("questions") or []
    codes = [q.get("code") for q in qs if q.get("type") == "Radio" and q.get("code")]
    scale_code = scale.get("code", "")
    scale_name = scale.get("name", "")
    fcode = "auto-" + _stable_id(scale_code + "|MCHAT总分")
    factors = [{
        "enabled": 1, "source": "proposed",
        "scale_code": scale_code, "scale_name": scale_name,
        "factor_code": fcode, "factor_title": "筛查总分",
        "factor_type": "first_grade", "is_total_score": "1",
        "formula": "sum", "append_params_json": "[]",
        "source_codes": ",".join(codes), "max_score": str(len(codes)), "is_show": "1",
        "notes": "常见分层：0-2低风险；3-7中风险；≥8高风险。请结合你题目计分键核对。",
    }]
    rules = [
        {"enabled": 1, "scale_code": scale_code, "scale_name": scale_name, "factor_code": fcode, "factor_title": "筛查总分", "rule_index": 0,
         "min_score": "0", "max_score": "2", "risk_level": "none", "conclusion": "低风险。", "suggestion": "如仍有发育担忧，建议随访并结合儿保/发育评估。"},
        {"enabled": 1, "scale_code": scale_code, "scale_name": scale_name, "factor_code": fcode, "factor_title": "筛查总分", "rule_index": 1,
         "min_score": "3", "max_score": "7", "risk_level": "medium", "conclusion": "中风险。", "suggestion": "建议二阶段随访或进一步评估；必要时转发育/儿童精神专科。"},
        {"enabled": 1, "scale_code": scale_code, "scale_name": scale_name, "factor_code": fcode, "factor_title": "筛查总分", "rule_index": 2,
         "min_score": "8", "max_score": str(len(codes)), "risk_level": "high", "conclusion": "高风险。", "suggestion": "建议尽快转介专业评估与早期干预咨询。"},
    ]
    return factors, rules

def _should_propose(scale: Dict[str, Any]) -> bool:
    return not (scale.get("factors") and len(scale.get("factors")) > 0)

def _proposal_for_scale(scale: Dict[str, Any]) -> Tuple[List[Dict[str, Any]], List[Dict[str, Any]]]:
    name = scale.get("name", "")
    if "范德比尔特" in name:
        return _propose_vanderbilt(scale)
    if "YGTSS" in name and "家长版" not in name:
        return _propose_ygtss(scale)
    if "GAD-7" in name:
        return _propose_gad7(scale)
    if "压力知觉量表" in name:
        return _propose_pss(scale)
    if "M-CMAT" in name or "M-CHAT" in name:
        return _propose_mchat_like(scale)
    return [], []

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("-i", "--input", required=True, help="Input scales YAML path")
    ap.add_argument("-o", "--output", required=True, help="Output Excel workbook path (.xlsx)")
    args = ap.parse_args()

    with open(args.input, "r", encoding="utf-8") as f:
        scales = yaml.safe_load(f)
    if not isinstance(scales, list):
        raise SystemExit("ERROR: YAML top-level must be a list of scales")

    scales_rows, factors_rows, rules_rows = [], [], []

    for s in scales:
        scode = s.get("code", "")
        sname = s.get("name", "")
        qcodes = _question_codes(s)
        scored = _scored_question_codes(s)
        covered = _covered_codes_by_factors(s)
        uncovered_scored = [c for c in scored if c not in covered]

        scales_rows.append({
            "scale_code": scode,
            "scale_name": sname,
            "category": s.get("category", ""),
            "questions_total": len(qcodes),
            "questions_scored": len(scored),
            "factors_count": len(s.get("factors") or []),
            "uncovered_scored_questions": len(uncovered_scored),
            "uncovered_scored_question_codes": ",".join(uncovered_scored),
            "notes": "",
        })

        fr, rr = _existing_factors_rows(s)
        factors_rows.extend(fr)
        rules_rows.extend(rr)

        if _should_propose(s):
            pfr, prr = _proposal_for_scale(s)
            factors_rows.extend(pfr)
            rules_rows.extend(prr)

    df_scales = pd.DataFrame(scales_rows)
    df_factors = pd.DataFrame(factors_rows)
    df_rules = pd.DataFrame(rules_rows)

    with pd.ExcelWriter(args.output, engine="openpyxl") as w:
        df_scales.to_excel(w, sheet_name="scales", index=False)
        df_factors.to_excel(w, sheet_name="factors", index=False)
        df_rules.to_excel(w, sheet_name="rules", index=False)

    print(f"OK: wrote workbook -> {args.output}")
    print("Edit 'factors' and 'rules', then run 02_apply_factor_design.py to update YAML.")

if __name__ == "__main__":
    main()
