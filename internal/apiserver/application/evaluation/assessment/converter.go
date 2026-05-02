package assessment

import (
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

// ============= 领域模型到 DTO 的转换器 =============

// toAssessmentResult 将领域模型转换为 AssessmentResult
func toAssessmentResult(a *assessment.Assessment) (*AssessmentResult, error) {
	if a == nil {
		return nil, nil
	}

	orgID, err := safeconv.Int64ToUint64(a.OrgID())
	if err != nil {
		return nil, evalerrors.DatabaseMessage("测评机构ID超出安全范围")
	}

	result := &AssessmentResult{
		ID:                   a.ID().Uint64(),
		OrgID:                orgID,
		TesteeID:             a.TesteeID().Uint64(),
		QuestionnaireCode:    a.QuestionnaireRef().Code().String(),
		QuestionnaireVersion: a.QuestionnaireRef().Version(),
		AnswerSheetID:        a.AnswerSheetRef().ID().Uint64(),
		OriginType:           a.Origin().Type().String(),
		Status:               a.Status().String(),
	}

	// 量表引用（可选）
	if scaleRef := a.MedicalScaleRef(); scaleRef != nil {
		scaleID := scaleRef.ID().Uint64()
		scaleCode := scaleRef.Code().String()
		scaleName := scaleRef.Name()
		result.MedicalScaleID = &scaleID
		result.MedicalScaleCode = &scaleCode
		result.MedicalScaleName = &scaleName
	}

	// 来源ID（可选）
	if originID := a.Origin().ID(); originID != nil {
		result.OriginID = originID
	}

	// 总分（可选）
	if totalScore := a.TotalScore(); totalScore != nil {
		result.TotalScore = totalScore
	}

	// 风险等级（可选）
	if riskLevel := a.RiskLevel(); riskLevel != nil {
		rl := string(*riskLevel)
		result.RiskLevel = &rl
	}

	// 时间戳
	if submittedAt := a.SubmittedAt(); submittedAt != nil {
		result.SubmittedAt = submittedAt
	}
	if interpretedAt := a.InterpretedAt(); interpretedAt != nil {
		result.InterpretedAt = interpretedAt
	}
	if failedAt := a.FailedAt(); failedAt != nil {
		result.FailedAt = failedAt
	}
	if failureReason := a.FailureReason(); failureReason != nil {
		result.FailureReason = failureReason
	}

	return result, nil
}

func assessmentRowToResult(row evaluationreadmodel.AssessmentRow) (*AssessmentResult, error) {
	orgID, err := safeconv.Int64ToUint64(row.OrgID)
	if err != nil {
		return nil, evalerrors.DatabaseMessage("测评机构ID超出安全范围")
	}
	return &AssessmentResult{
		ID:                   row.ID,
		OrgID:                orgID,
		TesteeID:             row.TesteeID,
		QuestionnaireCode:    row.QuestionnaireCode,
		QuestionnaireVersion: row.QuestionnaireVersion,
		AnswerSheetID:        row.AnswerSheetID,
		MedicalScaleID:       row.MedicalScaleID,
		MedicalScaleCode:     row.MedicalScaleCode,
		MedicalScaleName:     row.MedicalScaleName,
		OriginType:           row.OriginType,
		OriginID:             row.OriginID,
		Status:               row.Status,
		TotalScore:           row.TotalScore,
		RiskLevel:            row.RiskLevel,
		SubmittedAt:          row.SubmittedAt,
		InterpretedAt:        row.InterpretedAt,
		FailedAt:             row.FailedAt,
		FailureReason:        row.FailureReason,
	}, nil
}

func assessmentRowsToResults(rows []evaluationreadmodel.AssessmentRow) ([]*AssessmentResult, error) {
	results := make([]*AssessmentResult, 0, len(rows))
	for _, row := range rows {
		result, err := assessmentRowToResult(row)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}
