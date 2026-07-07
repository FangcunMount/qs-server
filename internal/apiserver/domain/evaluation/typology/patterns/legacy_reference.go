package patterns

import (
	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
)

// ScoreMBTIReference 暴露旧版 MBTI scorer 基线 用于 等价性 和 表征 tests。
func ScoreMBTIReference(model *modeltypology.MBTILegacyModel, answerSheet *evaluationinput.AnswerSheet) (MBTIResultDetail, error) {
	return ScoreMBTI(model, answerSheet)
}

// ScoreSBTIReference 暴露旧版 SBTI scorer 基线 用于 等价性 和 表征 tests。
func ScoreSBTIReference(model *modeltypology.SBTILegacyModel, answerSheet *evaluationinput.AnswerSheet) (SBTIResultDetail, error) {
	return ScoreSBTI(model, answerSheet)
}
