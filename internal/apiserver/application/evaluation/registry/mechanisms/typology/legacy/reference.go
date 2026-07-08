package legacy

import (
	evalinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/input"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/typology"
)

// ScoreMBTIReference exposes the legacy MBTI scorer baseline for equivalence and characterization tests.
func ScoreMBTIReference(model *modeltypology.MBTILegacyModel, answerSheet *evalinput.AnswerSheet) (MBTIResultDetail, error) {
	return ScoreMBTI(model, answerSheet)
}

// ScoreSBTIReference exposes the legacy SBTI scorer baseline for equivalence and characterization tests.
func ScoreSBTIReference(model *modeltypology.SBTILegacyModel, answerSheet *evalinput.AnswerSheet) (SBTIResultDetail, error) {
	return ScoreSBTI(model, answerSheet)
}
