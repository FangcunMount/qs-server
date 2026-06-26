package typology

import (
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
)

// ScoreMBTIReference exposes the legacy MBTI scorer baseline for equivalence and characterization tests.
func ScoreMBTIReference(model *modeltypology.MBTILegacyModel, answerSheet *evaluationinput.AnswerSheet) (MBTIResultDetail, error) {
	return ScoreMBTI(model, answerSheet)
}

// ScoreSBTIReference exposes the legacy SBTI scorer baseline for equivalence and characterization tests.
func ScoreSBTIReference(model *modeltypology.SBTILegacyModel, answerSheet *evaluationinput.AnswerSheet) (SBTIResultDetail, error) {
	return ScoreSBTI(model, answerSheet)
}
