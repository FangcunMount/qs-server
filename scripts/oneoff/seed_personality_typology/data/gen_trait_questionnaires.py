#!/usr/bin/env python3
"""Generate questionnaire JSON for Big5 IPIP-50 and Enneagram 45."""
import json
import os

LIKERT5 = [
    {"code": "1", "content": "非常不符合", "score": 1.0},
    {"code": "2", "content": "比较不符合", "score": 2.0},
    {"code": "3", "content": "中立 / 说不清", "score": 3.0},
    {"code": "4", "content": "比较符合", "score": 4.0},
    {"code": "5", "content": "非常符合", "score": 5.0},
]

BIG5_FACTORS = [
    ("O", "开放性"),
    ("C", "尽责性"),
    ("E", "外向性"),
    ("A", "宜人性"),
    ("N", "神经质"),
]

BIG5_ITEMS = [
    ("BIG5_Q01", "E", False, "我是聚会或群体中的活跃者。"),
    ("BIG5_Q02", "A", True, "我很少关心别人的感受。"),
    ("BIG5_Q03", "C", False, "我总是提前做好准备。"),
    ("BIG5_Q04", "N", False, "我容易感到压力或紧张。"),
    ("BIG5_Q05", "O", False, "我的词汇量比较丰富。"),
    ("BIG5_Q06", "E", True, "我平时话不多。"),
    ("BIG5_Q07", "A", False, "我对别人很感兴趣。"),
    ("BIG5_Q08", "C", True, "我经常把自己的东西弄得到处都是。"),
    ("BIG5_Q09", "N", True, "大多数时候我比较放松。"),
    ("BIG5_Q10", "O", True, "我很难理解抽象的想法。"),
    ("BIG5_Q11", "E", False, "我在人群中感到自在。"),
    ("BIG5_Q12", "A", True, "我有时会冒犯别人。"),
    ("BIG5_Q13", "C", False, "我会注意细节。"),
    ("BIG5_Q14", "N", False, "我经常为事情担心。"),
    ("BIG5_Q15", "O", False, "我有丰富的想象力。"),
    ("BIG5_Q16", "E", True, "我喜欢待在背景中，不太主动表现自己。"),
    ("BIG5_Q17", "A", False, "我能体会他人的情绪。"),
    ("BIG5_Q18", "C", True, "我有时会把事情搞得一团糟。"),
    ("BIG5_Q19", "N", True, "我很少感到沮丧或低落。"),
    ("BIG5_Q20", "O", True, "我对抽象思考不太感兴趣。"),
    ("BIG5_Q21", "E", False, "我喜欢主动与别人交谈。"),
    ("BIG5_Q22", "A", True, "我不太关心别人的问题。"),
    ("BIG5_Q23", "C", False, "我会很快把家务或任务完成。"),
    ("BIG5_Q24", "N", False, "我容易被事情打扰或烦躁。"),
    ("BIG5_Q25", "O", False, "我有很多想法。"),
    ("BIG5_Q26", "E", True, "我没什么话想说。"),
    ("BIG5_Q27", "A", False, "我有一颗柔软、体贴的心。"),
    ("BIG5_Q28", "C", True, "我经常忘记把东西放回原处。"),
    ("BIG5_Q29", "N", False, "我的情绪变化比较大。"),
    ("BIG5_Q30", "O", True, "我的想象力不算好。"),
    ("BIG5_Q31", "E", False, "我常常和不同的人聊天。"),
    ("BIG5_Q32", "A", True, "我对别人不太感兴趣。"),
    ("BIG5_Q33", "C", False, "我喜欢有秩序的环境。"),
    ("BIG5_Q34", "N", False, "我经常有情绪波动。"),
    ("BIG5_Q35", "O", False, "我能很快理解新事物。"),
    ("BIG5_Q36", "E", True, "我不喜欢把注意力集中到自己身上。"),
    ("BIG5_Q37", "A", False, "我愿意花时间帮助别人。"),
    ("BIG5_Q38", "C", True, "我常常逃避或拖延自己的责任。"),
    ("BIG5_Q39", "N", False, "我容易有负面情绪。"),
    ("BIG5_Q40", "O", False, "我喜欢使用或理解比较复杂的表达。"),
    ("BIG5_Q41", "E", False, "我不介意成为别人关注的中心。"),
    ("BIG5_Q42", "A", True, "我能感受到别人的痛苦，但不一定会被影响。"),
    ("BIG5_Q43", "C", False, "我会按照计划做事。"),
    ("BIG5_Q44", "N", True, "我大多数时候情绪稳定。"),
    ("BIG5_Q45", "O", False, "我会花时间思考事情背后的意义。"),
    ("BIG5_Q46", "E", True, "我在陌生人面前比较安静。"),
    ("BIG5_Q47", "A", False, "我会尽量让别人感到舒服。"),
    ("BIG5_Q48", "C", True, "我做事有时比较随意，不太按流程来。"),
    ("BIG5_Q49", "N", False, "我容易感到焦虑。"),
    ("BIG5_Q50", "O", True, "我很少产生新的想法。"),
]

ENNEAGRAM_FACTORS = [
    ("E1", "1号 改革者 / 完美主义者"),
    ("E2", "2号 助人者"),
    ("E3", "3号 成就者"),
    ("E4", "4号 个人主义者"),
    ("E5", "5号 探究者"),
    ("E6", "6号 忠诚者"),
    ("E7", "7号 热情者"),
    ("E8", "8号 挑战者"),
    ("E9", "9号 和平者"),
]

ENNEAGRAM_ITEMS = [
    ("ENNEA_Q01", "E1", "我很在意事情是否符合规则、标准或原则。"),
    ("ENNEA_Q02", "E1", "当别人做事不够严谨时，我会忍不住想纠正。"),
    ("ENNEA_Q03", "E1", "我常常对自己要求很高，希望把事情做到正确。"),
    ("ENNEA_Q04", "E1", "如果事情有明显瑕疵，我会很难真正放松。"),
    ("ENNEA_Q05", "E1", "我希望自己是正直、自律、值得信赖的人。"),
    ("ENNEA_Q06", "E2", "我很容易注意到别人需要什么帮助。"),
    ("ENNEA_Q07", "E2", "被别人需要会让我感到有价值。"),
    ("ENNEA_Q08", "E2", "我常常优先照顾别人的感受，而不是自己的需求。"),
    ("ENNEA_Q09", "E2", "如果我的付出没有被看见，我会感到失落。"),
    ("ENNEA_Q10", "E2", "我希望自己是温暖、体贴、能支持他人的人。"),
    ("ENNEA_Q11", "E3", "我很在意自己是否有成果、效率和竞争力。"),
    ("ENNEA_Q12", "E3", "我希望别人看到我是优秀、有能力、有价值的。"),
    ("ENNEA_Q13", "E3", "为了达成目标，我能迅速调整自己的状态和形象。"),
    ("ENNEA_Q14", "E3", "失败或停滞会让我非常不舒服。"),
    ("ENNEA_Q15", "E3", "我习惯用成就来证明自己。"),
    ("ENNEA_Q16", "E4", "我常常觉得自己和大多数人不太一样。"),
    ("ENNEA_Q17", "E4", "我很在意真实表达自己的感受和独特性。"),
    ("ENNEA_Q18", "E4", "我容易被某种遗憾、缺失或复杂情绪吸引。"),
    ("ENNEA_Q19", "E4", "我希望别人理解我内在细腻而独特的一面。"),
    ("ENNEA_Q20", "E4", "平淡无奇或没有个人风格的生活会让我感到空洞。"),
    ("ENNEA_Q21", "E5", "我喜欢先观察、理解，再决定是否参与。"),
    ("ENNEA_Q22", "E5", "我需要大量独处时间来整理信息和恢复能量。"),
    ("ENNEA_Q23", "E5", "比起情绪表达，我更习惯用分析和知识处理问题。"),
    ("ENNEA_Q24", "E5", "当外界要求太多时，我会本能地退回自己的空间。"),
    ("ENNEA_Q25", "E5", "掌握知识和理解原理会让我感到安全。"),
    ("ENNEA_Q26", "E6", "我会提前考虑风险、漏洞和可能出错的地方。"),
    ("ENNEA_Q27", "E6", "我很重视可靠的关系、承诺和安全感。"),
    ("ENNEA_Q28", "E6", "面对不确定性时，我会反复确认信息或寻求支持。"),
    ("ENNEA_Q29", "E6", "我既希望信任权威，又会忍不住怀疑权威。"),
    ("ENNEA_Q30", "E6", "我希望自己是谨慎、负责、值得依靠的人。"),
    ("ENNEA_Q31", "E7", "我喜欢保持选择开放，不希望被单一计划限制。"),
    ("ENNEA_Q32", "E7", "我会主动寻找有趣、新鲜、令人兴奋的体验。"),
    ("ENNEA_Q33", "E7", "当事情变得沉重或压抑时，我倾向于转向更积极的可能性。"),
    ("ENNEA_Q34", "E7", "我常常同时对很多计划和想法感兴趣。"),
    ("ENNEA_Q35", "E7", "我希望生活充满自由、快乐和丰富体验。"),
    ("ENNEA_Q36", "E8", "我不喜欢被别人控制或摆布。"),
    ("ENNEA_Q37", "E8", "面对冲突时，我倾向于直接表达立场。"),
    ("ENNEA_Q38", "E8", "我会保护自己在意的人，也愿意替弱者出头。"),
    ("ENNEA_Q39", "E8", "我欣赏力量、果断和掌控局面的能力。"),
    ("ENNEA_Q40", "E8", "表现脆弱对我来说并不容易。"),
    ("ENNEA_Q41", "E9", "我希望关系和环境保持平和，不喜欢冲突升级。"),
    ("ENNEA_Q42", "E9", "我常常能理解不同人的立场。"),
    ("ENNEA_Q43", "E9", "为了避免麻烦，我有时会压下自己的真实想法。"),
    ("ENNEA_Q44", "E9", "我更喜欢稳定、舒适、节奏平缓的状态。"),
    ("ENNEA_Q45", "E9", "当别人强烈表达意见时，我容易顺着对方来维持和气。"),
]


def trait_question(code, stem, factor, reverse=False):
    q = {
        "code": code,
        "stem": stem,
        "type": "Radio",
        "required": True,
        "factor": factor,
        "options": LIKERT5,
    }
    if reverse:
        q["reverse"] = True
    return q


def write_json(path, data):
    with open(path, "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False, indent=2)
        f.write("\n")


def build_big5():
    return {
        "code": "BIG5_IPIP_50",
        "version": "1.0.0",
        "title": "大五人格测评（IPIP-50）",
        "description": "基于 IPIP public-domain Big Five 五因素结构的大五人格测评，仅用于人格探索与自我了解。",
        "img_url": "",
        "type": "Survey",
        "factors": [{"code": code, "name": name} for code, name in BIG5_FACTORS],
        "questions": [trait_question(code, title, factor, reverse) for code, factor, reverse, title in BIG5_ITEMS],
    }


def build_enneagram():
    return {
        "code": "ENNEAGRAM_45",
        "version": "1.0.0",
        "title": "九型人格测评",
        "description": "基于九型人格九类动机结构的自我探索测评，仅供娱乐和个人成长参考，不作为心理诊断或职业测评依据。",
        "img_url": "",
        "type": "Survey",
        "factors": [{"code": code, "name": name} for code, name in ENNEAGRAM_FACTORS],
        "questions": [trait_question(code, title, factor) for code, factor, title in ENNEAGRAM_ITEMS],
    }


def main():
    here = os.path.dirname(os.path.abspath(__file__))
    files = {
        "big5_ipip_50_questionnaire.json": build_big5(),
        "enneagram_45_questionnaire.json": build_enneagram(),
    }
    for name, data in files.items():
        path = os.path.join(here, name)
        write_json(path, data)
        print(f"wrote {path} ({len(data['questions'])} questions)")


if __name__ == "__main__":
    main()
