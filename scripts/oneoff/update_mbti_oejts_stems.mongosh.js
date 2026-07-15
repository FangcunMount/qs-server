// Run with:
// mongosh "$MONGO_URI/$MONGO_DB" scripts/oneoff/update_mbti_oejts_stems.mongosh.js
//
// This script updates both the editable head and the published snapshot for
// MBTI_OEJTS@2.0.1. It deliberately changes only questions[].title.

const questionnaireCode = "MBTI_OEJTS";
const questionnaireVersion = "2.0.1";
const stems = [
  ["MBTI_Q01", "记录待办事项时，你更倾向于：1=制定清单；5=依靠记忆。"],
  ["MBTI_Q02", "面对未经验证的新信息时，你更倾向于：1=愿意相信；5=持怀疑态度。"],
  ["MBTI_Q03", "谈到独处时，你更接近：1=独处时感到无聊；5=需要独处时间。"],
  ["MBTI_Q04", "面对当前状况时，你更接近：1=接受事物的本来面目；5=对现状不满足。"],
  ["MBTI_Q05", "整理个人空间时，你更接近：1=保持房间整洁；5=随手放置东西。"],
  ["MBTI_Q06", "谈到逻辑思维时，你更接近：1=认为\"像机器人\"是侮辱；5=追求机械般的思维。"],
  ["MBTI_Q07", "在日常状态中，你更接近：1=精力充沛；5=平静温和。"],
  ["MBTI_Q08", "参加考试时，你更偏好：1=喜欢选择题；5=喜欢论述题。"],
  ["MBTI_Q09", "你的日常生活方式更接近：1=有条理；5=随性混乱。"],
  ["MBTI_Q10", "收到批评时，你更接近：1=容易受伤；5=脸皮厚。"],
  ["MBTI_Q11", "工作时，你更接近：1=在团队中表现最佳；5=独自工作表现最佳。"],
  ["MBTI_Q12", "你的时间焦点更接近：1=关注过去；5=关注未来。"],
  ["MBTI_Q13", "面对一项任务时，你更接近：1=提前规划；5=最后一刻才计划。"],
  ["MBTI_Q14", "在人际关系中，你更接近：1=渴望他人的爱；5=渴望他人的尊重。"],
  ["MBTI_Q15", "参加聚会后，你更接近：1=因聚会而兴奋；5=因聚会而疲惫。"],
  ["MBTI_Q16", "身处群体中时，你更接近：1=融入群体；5=与众不同。"],
  ["MBTI_Q17", "面对需要决定的事时，你更接近：1=做出承诺；5=保留选择。"],
  ["MBTI_Q18", "如果培养一项能力，你更接近：1=想擅长帮助他人；5=想擅长修理事物。"],
  ["MBTI_Q19", "与人交谈时，你更接近：1=话比较多；5=更善于倾听。"],
  ["MBTI_Q20", "讲述一件发生过的事时，你更重视：1=描述发生了什么；5=描述这意味着什么。"],
  ["MBTI_Q21", "处理一项明确任务时，你更接近：1=立即完成工作；5=拖延。"],
  ["MBTI_Q22", "作出重要决定时，你更接近：1=跟随内心；5=跟随理性。"],
  ["MBTI_Q23", "周末安排时间时，你更接近：1=喜欢外出活动；5=喜欢待在家里。"],
  ["MBTI_Q24", "学习一项新事物时，你更接近：1=想要细节；5=想要大局观。"],
  ["MBTI_Q25", "面对新情况时，你更接近：1=提前准备；5=即兴发挥。"],
  ["MBTI_Q26", "判断一件事是否合乎道德时，你更认同：1=道德基于同情；5=道德基于正义。"],
  ["MBTI_Q27", "在自然交谈中，你更接近：1=自然而然大声说话；5=很难大声喊叫。"],
  ["MBTI_Q28", "获取知识时，你更接近：1=经验主义；5=理论主义。"],
  ["MBTI_Q29", "安排时间时，你更接近：1=努力工作；5=尽情玩乐。"],
  ["MBTI_Q30", "谈及情感时，你更接近：1=重视情感；5=对情感感到不自在。"],
  ["MBTI_Q31", "成为众人焦点时，你更接近：1=喜欢表演；5=避免公开演讲。"],
  ["MBTI_Q32", "面对一个陌生问题时，你更接近：1=想知道\"谁/什么/何时\"；5=想知道\"为什么\"。"],
];

const recordFilter = {
  code: questionnaireCode,
  version: questionnaireVersion,
  deleted_at: null,
  $or: [
    { record_role: "head" },
    { record_role: "published_snapshot" },
    { record_role: { $exists: false } },
    { record_role: "" },
  ],
};

const documents = db.questionnaires.find(recordFilter, {
  _id: 1,
  record_role: 1,
  questions: 1,
}).toArray();
if (documents.length === 0) {
  throw new Error(`no active questionnaire records found for ${questionnaireCode}@${questionnaireVersion}`);
}

const expectedCodes = new Set(stems.map(([code]) => code));
for (const document of documents) {
  const actualCodes = new Set((document.questions || []).map((question) => question.code));
  const missing = [...expectedCodes].filter((code) => !actualCodes.has(code));
  if (missing.length > 0) {
    throw new Error(`questionnaire ${document._id} (${document.record_role || "legacy head"}) is missing: ${missing.join(", ")}`);
  }
}

const now = new Date();
const result = db.questionnaires.bulkWrite(
  stems.map(([code, title]) => ({
    updateMany: {
      filter: { ...recordFilter, "questions.code": code },
      update: {
        $set: {
          "questions.$[question].title": title,
          updated_at: now,
        },
      },
      arrayFilters: [{ "question.code": code }],
    },
  })),
  { ordered: true },
);

const expectedMatches = documents.length * stems.length;
if (result.matchedCount !== expectedMatches) {
  throw new Error(`matched ${result.matchedCount} updates, expected ${expectedMatches}; no rollback was attempted`);
}

printjson({
  questionnaire: `${questionnaireCode}@${questionnaireVersion}`,
  recordsUpdated: documents.length,
  questionTitlesUpdated: result.modifiedCount,
});
