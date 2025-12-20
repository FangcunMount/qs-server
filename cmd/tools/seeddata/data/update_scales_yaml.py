#!/usr/bin/env python3
# -*- coding: utf-8 -*-

"""
Update medical scales YAML with metadata fields:
- category
- stages
- applicableAges
- reporters
- tags (主题标签 only)
"""

import argparse
from typing import Dict, Any
import yaml

# 主类/阶段/人群/填报人 取值严格按你给的枚举
# tags 只保留“主题标签”（中文字符串）
MAPPING: Dict[str, Dict[str, Any]] = {
  "SNAP-IV量表（18项）": {
    "category": "adhd",
    "stages": ["screening", "follow_up"],
    "applicableAges": ["school_child", "adolescent"],
    "reporters": ["parent", "teacher"],
    "tags": [],
  },
  "SNAP-IV量表（26项）": {
    "category": "adhd",
    "stages": ["screening", "follow_up"],
    "applicableAges": ["school_child", "adolescent"],
    "reporters": ["parent", "teacher"],
    "tags": ["共病/对立外化"],
  },
  "Conners父母症状问卷（PSQ）": {
    "category": "adhd",
    "stages": ["deep_assessment", "follow_up"],
    "applicableAges": ["school_child", "adolescent"],
    "reporters": ["parent"],
    "tags": ["共病/对立外化"],
  },
  "Conners教师用量表": {
    "category": "adhd",
    "stages": ["deep_assessment", "follow_up"],
    "applicableAges": ["school_child", "adolescent"],
    "reporters": ["teacher"],
    "tags": ["共病/对立外化"],
  },
  "范德比尔特ADHD评定量表（父母版）": {
    "category": "adhd",
    "stages": ["deep_assessment", "follow_up", "outcome"],
    "applicableAges": ["school_child", "adolescent"],
    "reporters": ["parent"],
    "tags": ["功能结局/生活质量", "共病/对立外化", "焦虑", "抑郁"],
  },
  "范德比尔特ADHD评定量表（父母版）+表现": {
    "category": "adhd",
    "stages": ["deep_assessment", "follow_up", "outcome"],
    "applicableAges": ["school_child", "adolescent"],
    "reporters": ["parent"],
    "tags": ["功能结局/生活质量", "共病/对立外化", "焦虑", "抑郁"],
  },
  "注意缺陷多动及攻击评定量表（IOWA）": {
    "category": "adhd",
    "stages": ["screening", "follow_up"],
    "applicableAges": ["school_child", "adolescent"],
    "reporters": ["parent", "teacher"],
    "tags": ["攻击/品行", "共病/对立外化"],
  },

  "耶鲁抽动症整体严重程度量表（YGTSS）": {
    "category": "tic",
    "stages": ["deep_assessment", "follow_up"],
    "applicableAges": ["school_child", "adolescent"],
    "reporters": ["clinical"],
    "tags": ["功能结局/生活质量"],
  },
  "耶鲁抽动症整体严重程度量表（家长版）": {
    "category": "tic",
    "stages": ["follow_up"],
    "applicableAges": ["school_child", "adolescent"],
    "reporters": ["parent"],
    "tags": ["功能结局/生活质量"],
  },

  "SPM": {
    "category": "sensory",
    "stages": ["deep_assessment", "follow_up"],
    "applicableAges": ["preschool", "school_child", "adolescent"],
    "reporters": ["parent", "teacher"],
    "tags": ["社会能力/同伴"],
  },
  "儿童感觉统合能力发展评定量表": {
    "category": "sensory",
    "stages": ["deep_assessment", "follow_up"],
    "applicableAges": ["preschool", "school_child"],
    "reporters": ["parent"],
    "tags": [],
  },

  "Brief-2": {
    "category": "executive",
    "stages": ["deep_assessment", "follow_up"],
    "applicableAges": ["school_child", "adolescent"],
    "reporters": ["parent", "teacher", "self"],
    "tags": ["情绪调节"],
  },
  "执行功能行为评定量表（BRIEF）第二版": {
    "category": "executive",
    "stages": ["deep_assessment", "follow_up"],
    "applicableAges": ["school_child", "adolescent"],
    "reporters": ["parent", "teacher", "self"],
    "tags": ["情绪调节"],
  },
  "儿童执行功能的行为评定量表--他评问卷": {
    "category": "executive",
    "stages": ["deep_assessment", "follow_up"],
    "applicableAges": ["school_child", "adolescent"],
    "reporters": ["parent", "teacher"],
    "tags": ["情绪调节"],
  },

  "儿童困难问卷（QCD）": {
    "category": "qol",
    "stages": ["follow_up", "outcome"],
    "applicableAges": ["school_child", "adolescent"],
    "reporters": ["parent"],
    "tags": ["功能结局/生活质量"],
  },
  "Weiss功能缺陷量表父母版": {
    "category": "qol",
    "stages": ["follow_up", "outcome"],
    "applicableAges": ["school_child", "adolescent"],
    "reporters": ["parent"],
    "tags": ["功能结局/生活质量", "社会能力/同伴"],
  },
  "Weiss功能缺陷量表父母版-社会活动": {
    "category": "qol",
    "stages": ["follow_up", "outcome"],
    "applicableAges": ["school_child", "adolescent"],
    "reporters": ["parent"],
    "tags": ["功能结局/生活质量", "社会能力/同伴"],
  },

  "孤独症谱系障碍筛查性评估（M-CMAT，16-30个月）": {
    "category": "neurodev",
    "stages": ["screening"],
    "applicableAges": ["infant"],
    "reporters": ["parent"],
    "tags": ["ASD筛查/评定"],
  },

  "焦虑自评量表（SAS）": {
    "category": "mental",
    "stages": ["screening", "follow_up"],
    "applicableAges": ["adolescent", "adult"],
    "reporters": ["self"],
    "tags": ["焦虑"],
  },
  "广泛性焦虑障碍量表（GAD-7）": {
    "category": "mental",
    "stages": ["screening", "follow_up"],
    "applicableAges": ["adolescent", "adult"],
    "reporters": ["self"],
    "tags": ["焦虑"],
  },
  "儿童焦虑性情绪障碍筛查量表（SCARED）": {
    "category": "mental",
    "stages": ["screening", "follow_up"],
    "applicableAges": ["school_child", "adolescent"],
    "reporters": ["parent", "self"],
    "tags": ["焦虑"],
  },
  "抑郁自评量表（SDS）": {
    "category": "mental",
    "stages": ["screening", "follow_up"],
    "applicableAges": ["adolescent", "adult"],
    "reporters": ["self"],
    "tags": ["抑郁"],
  },
  "症状自评量表SCL-90": {
    "category": "mental",
    "stages": ["screening", "follow_up"],
    "applicableAges": ["adult"],
    "reporters": ["self"],
    "tags": ["广谱心理症状"],
  },
  "压力知觉量表": {
    "category": "mental",
    "stages": ["screening", "follow_up"],
    "applicableAges": ["adolescent", "adult"],
    "reporters": ["self"],
    "tags": ["压力/应激"],
  },
  "家庭顺应行为量表父母版（FASA）": {
    "category": "mental",
    "stages": ["deep_assessment", "follow_up"],
    "applicableAges": ["school_child", "adolescent"],
    "reporters": ["parent"],
    "tags": ["家庭系统/顺应"],
  },
  "Achenbach儿童行为量表": {
    "category": "mental",
    "stages": ["deep_assessment", "follow_up"],
    "applicableAges": ["preschool", "school_child", "adolescent"],
    "reporters": ["parent"],
    "tags": ["广谱行为情绪", "社会能力/同伴"],
  },
  "Achenbach儿童行为表（CBCL）——一般项目": {
    "category": "mental",
    "stages": ["deep_assessment", "follow_up"],
    "applicableAges": ["preschool", "school_child", "adolescent"],
    "reporters": ["parent"],
    "tags": ["广谱行为情绪"],
  },
  "Achenbach儿童行为表（CBCL）——社会能力": {
    "category": "mental",
    "stages": ["deep_assessment", "follow_up", "outcome"],
    "applicableAges": ["preschool", "school_child", "adolescent"],
    "reporters": ["parent"],
    "tags": ["社会能力/同伴", "功能结局/生活质量"],
  },
  "Achenbach儿童行为表（CBCL）——行为问题": {
    "category": "mental",
    "stages": ["deep_assessment", "follow_up"],
    "applicableAges": ["preschool", "school_child", "adolescent"],
    "reporters": ["parent"],
    "tags": ["广谱行为情绪"],
  },
}

META_KEYS = ["category", "stages", "applicableAges", "reporters", "tags"]

def update_one(item: Dict[str, Any]) -> Dict[str, Any]:
    name = item.get("name", "")
    meta = MAPPING.get(name)
    if not meta:
        return item

    # 为了尽量保持结构稳定：只覆盖 5 个字段，其他字段原样保留
    item["category"] = meta["category"]
    item["stages"] = meta["stages"]
    item["applicableAges"] = meta["applicableAges"]
    item["reporters"] = meta["reporters"]
    item["tags"] = meta["tags"]
    return item

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("-i", "--input", required=True, help="Input YAML path")
    ap.add_argument("-o", "--output", required=True, help="Output YAML path")
    args = ap.parse_args()

    with open(args.input, "r", encoding="utf-8") as f:
        data = yaml.safe_load(f)

    if not isinstance(data, list):
        raise SystemExit("ERROR: top-level YAML is not a list. Please check file format.")

    missing = []
    out = []
    for it in data:
        name = (it or {}).get("name")
        if name not in MAPPING:
            missing.append(name)
        out.append(update_one(it))

    with open(args.output, "w", encoding="utf-8") as f:
        yaml.safe_dump(out, f, sort_keys=False, allow_unicode=True, width=120)

    if missing:
        print("WARNING: no mapping for these names (left unchanged):")
        for n in missing:
            print(" -", n)
    else:
        print("OK: all scales updated.")

if __name__ == "__main__":
    main()
