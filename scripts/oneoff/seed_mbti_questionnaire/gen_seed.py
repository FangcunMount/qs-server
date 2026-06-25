#!/usr/bin/env python3
"""Generate mbti_oejts.json and mbti_questionnaire.json from OEJTS 1.2 data."""
import json
import os

questions_raw = [
    (1, "JP", "你如何记录任务？", "制定清单", "依靠记忆"),
    (2, "TF", "你如何看待新信息？", "愿意相信", "持怀疑态度"),
    (3, "EI", "你对独处有什么感受？", "独处时感到无聊", "需要独处时间"),
    (4, "SN", "你如何看待现状？", "接受事物的本来面目", "对现状不满足"),
    (5, "JP", "你如何整理空间？", "保持房间整洁", "随手放置东西"),
    (6, "TF", "你如何看待逻辑思维？", '认为"像机器人"是侮辱', "追求机械般的思维"),
    (7, "EI", "你的能量状态如何？", "精力充沛", "平静温和"),
    (8, "SN", "你喜欢什么类型的考试？", "喜欢选择题", "喜欢论述题"),
    (9, "JP", "你如何形容自己的生活方式？", "有条理", "随性混乱"),
    (10, "TF", "你如何应对批评？", "容易受伤", "脸皮厚"),
    (11, "EI", "你在什么环境下工作最好？", "在团队中表现最佳", "独自工作表现最佳"),
    (12, "SN", "你的时间焦点在哪里？", "关注过去", "关注未来"),
    (13, "JP", "你何时制定计划？", "提前规划", "最后一刻才计划"),
    (14, "TF", "你希望从他人那里得到什么？", "渴望他人的爱", "渴望他人的尊重"),
    (15, "EI", "聚会让你感觉如何？", "因聚会而兴奋", "因聚会而疲惫"),
    (16, "SN", "在群体中，你倾向于...", "融入群体", "与众不同"),
    (17, "JP", "你如何做决定？", "做出承诺", "保留选择"),
    (18, "TF", "你想擅长什么？", "想擅长帮助他人", "想擅长修理事物"),
    (19, "EI", "在交谈中，你更倾向于...", "话比较多", "更善于倾听"),
    (20, "SN", "讲故事时，你会...", "描述发生了什么", "描述这意味着什么"),
    (21, "JP", "你何时完成任务？", "立即完成工作", "拖延"),
    (22, "TF", "你如何做决定？", "跟随内心", "跟随理性"),
    (23, "EI", "周末你更喜欢做什么？", "喜欢外出活动", "喜欢待在家里"),
    (24, "SN", "学习新事物时，你想要什么？", "想要细节", "想要大局观"),
    (25, "JP", "你如何应对新情况？", "提前准备", "即兴发挥"),
    (26, "TF", "道德的基础是什么？", "道德基于同情", "道德基于正义"),
    (27, "EI", "你的自然音量如何？", "自然而然大声说话", "很难大声喊叫"),
    (28, "SN", "你如何获取知识？", "经验主义", "理论主义"),
    (29, "JP", "什么更能驱动你？", "努力工作", "尽情玩乐"),
    (30, "TF", "你如何看待情感？", "重视情感", "对情感感到不自在"),
    (31, "EI", "你对成为焦点有什么感受？", "喜欢表演", "避免公开演讲"),
    (32, "SN", "什么问题最让你感兴趣？", '想知道"谁/什么/何时"', '想知道"为什么"'),
]

signs = {
    1: 1, 2: -1, 3: -1, 4: 1, 5: 1, 6: 1, 7: -1, 8: 1, 9: -1, 10: 1,
    11: -1, 12: 1, 13: 1, 14: -1, 15: -1, 16: 1, 17: -1, 18: -1, 19: 1, 20: 1,
    21: 1, 22: 1, 23: -1, 24: -1, 25: -1, 26: -1, 27: 1, 28: -1, 29: 1, 30: -1,
    31: 1, 32: 1,
}
constants = {"EI": 30, "SN": 12, "TF": 30, "JP": 18}
threshold = 24
poles = {"EI": ("I", "E"), "SN": ("S", "N"), "TF": ("F", "T"), "JP": ("J", "P")}
dim_names = {
    "EI": "外向 / 内向",
    "SN": "感觉 / 直觉",
    "TF": "情感 / 思考",
    "JP": "判断 / 知觉",
}

profiles = {
    "INTJ": ("建筑师", "独立战略家，善于长远规划与系统思考。", "理性、独立、有远见", "逻辑严密、执行力强、标准高", "可能显得冷漠或过于挑剔", "把宏大目标拆成小步，定期与信任的人交流进展。"),
    "INTP": ("逻辑学家", "好奇的思想家，热衷探索原理与可能性。", "分析力强、开放、独创", "善于发现问题本质、学习快", "容易拖延、不善表达情感", "为想法设定截止日期，选一个项目先做完。"),
    "ENTJ": ("指挥官", "天生的组织者，推动团队向目标前进。", "果断、自信、高效", "领导力强、善于统筹资源", "可能显得强势、忽略他人感受", "决策前多问一句「别人需要什么」。"),
    "ENTP": ("辩论家", "点子多多，喜欢挑战现状和头脑风暴。", "机智、灵活、爱争辩", "创新、适应快、口才好", "容易分心、不喜例行公事", "把创意落到一件可交付的小事上。"),
    "INFJ": ("提倡者", "理想主义者，关注意义与他人成长。", "洞察力强、有原则、温和", "善于倾听、能激励他人", "易过度承担、难说「不」", "保护自己的精力，把理想写成可执行清单。"),
    "INFP": ("调停者", "内心丰富，追求价值一致与真诚连接。", "敏感、理想化、有创意", "共情强、忠于信念", "易逃避冲突、易自我怀疑", "小步行动，让价值观体现在日常选择里。"),
    "ENFJ": ("主人公", "善于凝聚人心，关心群体和谐与成长。", "热情、负责、善沟通", "鼓舞他人、组织力强", "易过度取悦、忽略自己需求", "每周留时间只给自己，不必时刻当「桥梁」。"),
    "ENFP": ("竞选者", "热情开朗，对新体验和人充满好奇。", "外向、想象力丰富、乐观", "感染力强、善于联结人", "易三分钟热度、难坚持细节", "用清单管住一件长期事，其余随意。"),
    "ISTJ": ("物流师", "务实可靠，重视责任、秩序与事实。", "严谨、守信、有条理", "稳定、细致、能扛事", "可能固执、不喜突变", "重大变化时给自己适应期，再行动。"),
    "ISFJ": ("守卫者", "默默付出，守护身边人的安稳与幸福。", "体贴、忠诚、细心", "可靠、记性好、愿帮忙", "易压抑需求、怕让人失望", "练习直接说出自己的需要。"),
    "ESTJ": ("总经理", "讲究效率与规则，善于把事情办妥。", "务实、直接、有组织", "执行力强、标准清晰", "可能显得刻板、不够柔软", "倾听不同意见后再拍板。"),
    "ESFJ": ("执政官", "重视关系与氛围，乐于照顾他人感受。", "热心、合群、体贴", "善于协调、让人安心", "易过度在意评价、难拒绝", "把「对别人好」和「对自己好」放在同等位置。"),
    "ISTP": ("鉴赏家", "冷静动手派，擅长拆解问题并现场解决。", "冷静、灵活、独立", "动手能力强、临危不乱", "可能显得疏离、不喜空谈", "重要决定写下来，避免纯凭当下感觉。"),
    "ISFP": ("探险家", "温和艺术家气质，活在当下的感受与美感里。", "温和、敏感、随和", "审美好、包容、真实", "易回避压力、不喜规划", "给喜欢的事留固定时间块。"),
    "ESTP": ("企业家", "行动派，喜欢现场应变与直接体验。", "大胆、现实、精力充沛", "反应快、敢冒险、善谈判", "易冲动、厌倦理论", "大决定前停 24 小时再确认。"),
    "ESFP": ("表演者", "活力四射，享受当下、人群与感官体验。", "外向、风趣、热情", "让人开心、适应力强", "易逃避枯燥、难做长期计划", "把必须完成的事和玩乐分开排期。"),
}

q_seed = {
    "code": "MBTI_OEJTS",
    "version": "1.0.0",
    "title": "MBTI 人格类型测评（OEJTS 32题）",
    "description": "基于 Open Extended Jungian Type Scales 1.2 的 32 题人格类型测评，结果兼容 MBTI 四字母类型。仅供娱乐与自我探索，非官方 MBTI 评估。",
    "img_url": "",
    "type": "Survey",
    "questions": [],
}
labels = ["非常符合左侧", "比较符合左侧", "中立", "比较符合右侧", "非常符合右侧"]
for qid, _dim, title, left, right in questions_raw:
    code = f"MBTI_Q{qid:02d}"
    stem = f"{title}\n（更偏向：{left} ← 1 · 2 · 3 · 4 · 5 → {right}）"
    opts = [{"code": str(v), "content": labels[v - 1], "score": float(v)} for v in range(1, 6)]
    q_seed["questions"].append({"code": code, "stem": stem, "type": "Radio", "required": True, "options": opts})

mappings = [
    {"question_code": f"MBTI_Q{qid:02d}", "dimension": dim, "sign": signs[qid]}
    for qid, dim, *_ in questions_raw
]

type_profiles = []
for code in [
    "INTJ", "INTP", "ENTJ", "ENTP", "INFJ", "INFP", "ENFJ", "ENFP",
    "ISTJ", "ISFJ", "ESTJ", "ESFJ", "ISTP", "ISFP", "ESTP", "ESFP",
]:
    name, one, summary, strengths, weaknesses, sugg = profiles[code]
    type_profiles.append({
        "type_code": code,
        "type_name": name,
        "one_liner": one,
        "summary": summary,
        "traits": [t.strip() for t in summary.split("、")[:3]],
        "strengths": [strengths],
        "weaknesses": [weaknesses],
        "suggestions": [sugg],
        "image_url": "",
    })

model = {
    "code": "MBTI_OEJTS",
    "version": "1.0.0",
    "title": "MBTI 人格类型测评（OEJTS）",
    "questionnaire_code": "MBTI_OEJTS",
    "questionnaire_version": "1.0.0",
    "status": "published",
    "source": {
        "questions_repo": "https://github.com/openjung/core",
        "source_site": "https://openpsychometrics.org/tests/OEJTS/",
        "license": "CC BY-NC-SA 4.0 (OEJTS items); MIT (openjung/core translations)",
        "attribution": "题目基于 Open Extended Jungian Type Scales 1.2 (Eric Jorgenson / Open Psychometrics)；中文题干来自 openjung/core。16 型画像为原创简版。",
        "non_commercial": True,
    },
    "dimension_order": ["EI", "SN", "TF", "JP"],
    "dimensions": {
        k: {
            "code": k,
            "name": dim_names[k],
            "left_pole": poles[k][0],
            "right_pole": poles[k][1],
            "constant": constants[k],
            "threshold": threshold,
        }
        for k in ["EI", "SN", "TF", "JP"]
    },
    "question_mappings": mappings,
    "type_profiles": type_profiles,
}

root = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", ".."))
model_path = os.path.join(root, "internal", "apiserver", "infra", "evaluationinput", "seed", "mbti_oejts.json")
q_path = os.path.join(os.path.dirname(__file__), "mbti_questionnaire.json")
for path, data in [(model_path, model), (q_path, q_seed)]:
    with open(path, "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False, indent=2)
        f.write("\n")
print("wrote", model_path)
print("wrote", q_path)
