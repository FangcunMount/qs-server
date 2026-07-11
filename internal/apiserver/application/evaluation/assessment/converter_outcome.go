package assessment

import (
	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
)

func assessmentRowToOutcomeResult(row evaluationreadmodel.AssessmentRow) (*AssessmentOutcomeResult, error) {
	base, err := assessmentRowToResult(row)
	if err != nil || base == nil {
		return nil, err
	}
	return &AssessmentOutcomeResult{
		ID:                   base.ID,
		OrgID:                base.OrgID,
		TesteeID:             base.TesteeID,
		QuestionnaireCode:    base.QuestionnaireCode,
		QuestionnaireVersion: base.QuestionnaireVersion,
		AnswerSheetID:        base.AnswerSheetID,
		Model:                modelIdentityFromAssessmentRow(row),
		PrimaryScore:         primaryScoreFromAssessmentRow(row),
		Level:                levelFromAssessmentRow(row),
		OriginType:           base.OriginType,
		OriginID:             base.OriginID,
		Status:               base.Status,
		SubmittedAt:          base.SubmittedAt,
		InterpretedAt:        base.InterpretedAt,
		FailedAt:             base.FailedAt,
		FailureReason:        base.FailureReason,
	}, nil
}

func assessmentRowsToOutcomeResults(rows []evaluationreadmodel.AssessmentRow) ([]*AssessmentOutcomeResult, error) {
	results := make([]*AssessmentOutcomeResult, 0, len(rows))
	for _, row := range rows {
		result, err := assessmentRowToOutcomeResult(row)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func modelIdentityFromAssessmentRow(row evaluationreadmodel.AssessmentRow) ModelIdentityResult {
	kind := derefString(row.EvaluationModelKind)
	subKind := derefString(row.EvaluationModelSubKind)
	algorithm := derefString(row.EvaluationModelAlgorithm)
	if algorithm == "" && kind != "" {
		if mappedKind, mappedSubKind, mappedAlgorithm, ok := modelcatalog.LegacyKindMapping(modelcatalog.Kind(kind)); ok {
			kind = string(mappedKind)
			if subKind == "" {
				subKind = string(mappedSubKind)
			}
			algorithm = string(mappedAlgorithm)
		}
	}
	return EnrichModelIdentityResult(ModelIdentityResult{
		Kind:      kind,
		SubKind:   subKind,
		Algorithm: algorithm,
		Code:      derefString(row.EvaluationModelCode),
		Version:   derefString(row.EvaluationModelVersion),
		Title:     derefString(row.EvaluationModelTitle),
	}, "")
}

func primaryScoreFromAssessmentRow(row evaluationreadmodel.AssessmentRow) *ScoreValueResult {
	if row.PrimaryScoreKind != nil && row.PrimaryScoreValue != nil {
		return &ScoreValueResult{
			Kind:  *row.PrimaryScoreKind,
			Value: *row.PrimaryScoreValue,
			Label: derefString(row.PrimaryScoreLabel),
			Max:   row.PrimaryScoreMax,
		}
	}
	if row.TotalScore != nil {
		return &ScoreValueResult{
			Kind:  string(domainoutcome.ScoreKindRawTotal),
			Value: *row.TotalScore,
		}
	}
	return nil
}

func levelFromAssessmentRow(row evaluationreadmodel.AssessmentRow) *ResultLevelResult {
	if row.LevelCode != nil {
		return &ResultLevelResult{
			Code:     *row.LevelCode,
			Label:    derefString(row.LevelLabel),
			Severity: derefString(row.Severity),
		}
	}
	if row.RiskLevel != nil && *row.RiskLevel != "" {
		return legacyRiskLevelResult(*row.RiskLevel)
	}
	return nil
}

// legacyRiskLevelResult keeps the legacy risk-level read-model projection in
// Evaluation. It is a score-fact compatibility mapping, not a report rule.
func legacyRiskLevelResult(code string) *ResultLevelResult {
	if !domainassessment.IsRiskLevelCode(code) {
		return nil
	}
	severity := "none"
	switch domainassessment.RiskLevel(code) {
	case domainassessment.RiskLevelSevere, domainassessment.RiskLevelHigh:
		severity = "high"
	case domainassessment.RiskLevelMedium:
		severity = "medium"
	case domainassessment.RiskLevelLow:
		severity = "low"
	}
	return &ResultLevelResult{Code: code, Label: code, Severity: severity}
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// RowToOutcomeResult exports 读模型 行 conversion 用于 transport layers。
func RowToOutcomeResult(row evaluationreadmodel.AssessmentRow) (*AssessmentOutcomeResult, error) {
	return assessmentRowToOutcomeResult(row)
}

// RowsToOutcomeResults exports batch 读模型 行 conversion 用于 transport layers。
func RowsToOutcomeResults(rows []evaluationreadmodel.AssessmentRow) ([]*AssessmentOutcomeResult, error) {
	return assessmentRowsToOutcomeResults(rows)
}
