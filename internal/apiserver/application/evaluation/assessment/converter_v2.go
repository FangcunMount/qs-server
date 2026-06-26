package assessment

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
)

func assessmentRowToV2Result(row evaluationreadmodel.AssessmentRow) (*AssessmentV2Result, error) {
	base, err := assessmentRowToResult(row)
	if err != nil || base == nil {
		return nil, err
	}
	return &AssessmentV2Result{
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

func assessmentRowsToV2Results(rows []evaluationreadmodel.AssessmentRow) ([]*AssessmentV2Result, error) {
	results := make([]*AssessmentV2Result, 0, len(rows))
	for _, row := range rows {
		result, err := assessmentRowToV2Result(row)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func reportRowToV2Result(row evaluationreadmodel.ReportRow) *ReportV2Result {
	base := reportRowToResult(row)
	if base == nil {
		return nil
	}
	return &ReportV2Result{
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
		if mappedKind, mappedSubKind, mappedAlgorithm, ok := assessmentmodel.LegacyKindMapping(assessmentmodel.Kind(kind)); ok {
			kind = string(mappedKind)
			if subKind == "" {
				subKind = string(mappedSubKind)
			}
			algorithm = string(mappedAlgorithm)
		}
	}
	if kind == "" && row.MedicalScaleCode != nil {
		kind = string(assessmentmodel.KindScale)
		algorithm = string(assessmentmodel.AlgorithmScaleDefault)
	}
	return ModelIdentityResult{
		Kind:      kind,
		SubKind:   subKind,
		Algorithm: algorithm,
		Code:      firstNonEmpty(derefString(row.EvaluationModelCode), derefString(row.MedicalScaleCode)),
		Version:   derefString(row.EvaluationModelVersion),
		Title:     firstNonEmpty(derefString(row.EvaluationModelTitle), derefString(row.MedicalScaleName)),
	}
}

func modelIdentityFromReportRow(row evaluationreadmodel.ReportRow) ModelIdentityResult {
	if row.Model.Kind != "" || row.Model.Code != "" {
		return ModelIdentityResult{
			Kind:      row.Model.Kind,
			SubKind:   row.Model.SubKind,
			Algorithm: row.Model.Algorithm,
			Code:      row.Model.Code,
			Version:   row.Model.Version,
			Title:     row.Model.Title,
		}
	}
	return ModelIdentityResult{
		Kind:  string(assessmentmodel.KindScale),
		Code:  row.ModelCode,
		Title: row.ModelName,
	}
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

// RowToV2Result exports read-model row conversion for transport layers.
func RowToV2Result(row evaluationreadmodel.AssessmentRow) (*AssessmentV2Result, error) {
	return assessmentRowToV2Result(row)
}

// RowsToV2Results exports batch read-model row conversion for transport layers.
func RowsToV2Results(rows []evaluationreadmodel.AssessmentRow) ([]*AssessmentV2Result, error) {
	return assessmentRowsToV2Results(rows)
}
