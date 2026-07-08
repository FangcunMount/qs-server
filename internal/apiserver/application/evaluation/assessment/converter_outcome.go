package assessment

import (
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
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

func reportRowToOutcomeResult(row evaluationreadmodel.ReportRow) *ReportOutcomeResult {
	base := reportRowToResult(row)
	if base == nil {
		return nil
	}
	return &ReportOutcomeResult{
		AssessmentID: base.AssessmentID,
		Model:        modelIdentityFromReportRow(row),
		PrimaryScore: primaryScoreFromReportRow(row),
		Level:        levelFromReportRow(row),
		Conclusion:   base.Conclusion,
		Dimensions:   base.Dimensions,
		Suggestions:  base.Suggestions,
		ModelExtra:   base.ModelExtra,
		CreatedAt:    base.CreatedAt,
	}
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
	if kind == "" && row.MedicalScaleCode != nil {
		kind = string(modelcatalog.KindScale)
		algorithm = string(modelcatalog.AlgorithmScaleDefault)
	}
	return EnrichModelIdentityResult(ModelIdentityResult{
		Kind:      kind,
		SubKind:   subKind,
		Algorithm: algorithm,
		Code:      firstNonEmpty(derefString(row.EvaluationModelCode), derefString(row.MedicalScaleCode)),
		Version:   derefString(row.EvaluationModelVersion),
		Title:     firstNonEmpty(derefString(row.EvaluationModelTitle), derefString(row.MedicalScaleName)),
	}, "")
}

func modelIdentityFromReportRow(row evaluationreadmodel.ReportRow) ModelIdentityResult {
	if row.Model.Kind != "" || row.Model.Code != "" {
		return EnrichModelIdentityResult(ModelIdentityResult{
			Kind:            row.Model.Kind,
			SubKind:         row.Model.SubKind,
			Algorithm:       row.Model.Algorithm,
			Code:            row.Model.Code,
			Version:         row.Model.Version,
			Title:           row.Model.Title,
			ProductChannel:  row.Model.ProductChannel,
			AlgorithmFamily: row.Model.AlgorithmFamily,
		}, row.Model.ProductChannel)
	}
	return EnrichModelIdentityResult(ModelIdentityResult{
		Kind:  string(modelcatalog.KindScale),
		Code:  row.ModelCode,
		Title: row.ModelName,
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
			Kind:  domainreport.ScoreKindRawTotal,
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
		level := domainreport.LevelFromRisk(domainreport.RiskLevel(*row.RiskLevel))
		if level != nil {
			return &ResultLevelResult{Code: level.Code, Label: level.Label, Severity: level.Severity}
		}
	}
	return nil
}

func primaryScoreFromReportRow(row evaluationreadmodel.ReportRow) *ScoreValueResult {
	if row.PrimaryScore != nil {
		return &ScoreValueResult{
			Kind:  row.PrimaryScore.Kind,
			Value: row.PrimaryScore.Value,
			Label: row.PrimaryScore.Label,
			Max:   row.PrimaryScore.Max,
		}
	}
	if row.TotalScore != 0 || row.RiskLevel != "" {
		return &ScoreValueResult{Kind: domainreport.ScoreKindRawTotal, Value: row.TotalScore}
	}
	return nil
}

func levelFromReportRow(row evaluationreadmodel.ReportRow) *ResultLevelResult {
	if row.Level != nil {
		return &ResultLevelResult{
			Code:     row.Level.Code,
			Label:    row.Level.Label,
			Severity: row.Level.Severity,
		}
	}
	if row.RiskLevel != "" && domainreport.IsRiskLevelCode(row.RiskLevel) {
		level := domainreport.LevelFromRisk(domainreport.RiskLevel(row.RiskLevel))
		if level != nil {
			return &ResultLevelResult{Code: level.Code, Label: level.Label, Severity: level.Severity}
		}
	}
	if row.ModelExtra != nil && row.ModelExtra.TypeCode != "" {
		return &ResultLevelResult{Code: row.ModelExtra.TypeCode, Label: row.ModelExtra.TypeCode, Severity: "none"}
	}
	return nil
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

// Deprecated: 使用 RowToOutcomeResult。
func RowToV2Result(row evaluationreadmodel.AssessmentRow) (*AssessmentOutcomeResult, error) {
	return RowToOutcomeResult(row)
}

// Deprecated: 使用 RowsToOutcomeResults。
func RowsToV2Results(rows []evaluationreadmodel.AssessmentRow) ([]*AssessmentOutcomeResult, error) {
	return RowsToOutcomeResults(rows)
}
