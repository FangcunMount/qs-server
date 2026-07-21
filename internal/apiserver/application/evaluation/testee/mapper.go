package testee

import (
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	modelbinding "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

func assessmentFromRow(row evaluationreadmodel.AssessmentRow) (*Assessment, error) {
	org, err := safeconv.Int64ToUint64(row.OrgID)
	if err != nil {
		return nil, evalerrors.DatabaseMessage("机构ID超出 uint64 范围")
	}
	return &Assessment{ID: row.ID, OrgID: org, TesteeID: row.TesteeID, QuestionnaireCode: row.QuestionnaireCode, QuestionnaireVersion: row.QuestionnaireVersion, AnswerSheetID: row.AnswerSheetID, Model: modelFromRow(row), PrimaryScore: primaryScoreFromRow(row), Level: levelFromRow(row), OriginType: row.OriginType, OriginID: row.OriginID, Status: row.Status, SubmittedAt: row.SubmittedAt, FailedAt: row.FailedAt, FailureReason: row.FailureReason}, nil
}

func modelFromRow(row evaluationreadmodel.AssessmentRow) ModelIdentity {
	kind, sub, algorithm := deref(row.EvaluationModelKind), deref(row.EvaluationModelSubKind), deref(row.EvaluationModelAlgorithm)
	result := ModelIdentity{Kind: kind, SubKind: sub, Algorithm: algorithm, Code: deref(row.EvaluationModelCode), Version: deref(row.EvaluationModelVersion), Title: deref(row.EvaluationModelTitle)}
	k := modelbinding.Kind(result.Kind)
	result.ProductChannel = modelbinding.ProductChannelForIdentity(k, result.ProductChannel)
	result.AlgorithmFamily = modelbinding.AlgorithmFamilyStringFromIdentity(k, modelbinding.SubKind(result.SubKind), modelbinding.Algorithm(result.Algorithm))
	return result
}

func primaryScoreFromRow(row evaluationreadmodel.AssessmentRow) *ScoreValue {
	if row.PrimaryScoreKind != nil && row.PrimaryScoreValue != nil {
		return &ScoreValue{Kind: *row.PrimaryScoreKind, Value: *row.PrimaryScoreValue, Label: deref(row.PrimaryScoreLabel), Max: row.PrimaryScoreMax}
	}
	if row.TotalScore != nil {
		return &ScoreValue{Kind: string(domainoutcome.ScoreKindRawTotal), Value: *row.TotalScore}
	}
	return nil
}

func levelFromRow(row evaluationreadmodel.AssessmentRow) *ResultLevel {
	if row.LevelCode != nil {
		return &ResultLevel{Code: *row.LevelCode, Label: deref(row.LevelLabel), Severity: deref(row.Severity)}
	}
	if row.RiskLevel == nil || !domainassessment.IsRiskLevelCode(*row.RiskLevel) {
		return nil
	}
	severity := "none"
	switch domainassessment.RiskLevel(*row.RiskLevel) {
	case domainassessment.RiskLevelSevere, domainassessment.RiskLevelHigh:
		severity = "high"
	case domainassessment.RiskLevelMedium:
		severity = "medium"
	case domainassessment.RiskLevelLow:
		severity = "low"
	}
	return &ResultLevel{Code: *row.RiskLevel, Label: *row.RiskLevel, Severity: severity}
}

func scoreFromFact(f *evaloutcome.ScoreFact) *Score {
	factors := make([]FactorScore, 0, len(f.FactorScores))
	for _, v := range f.FactorScores {
		factors = append(factors, FactorScore{FactorCode: v.FactorCode, FactorName: v.FactorName, RawScore: v.RawScore, MaxScore: v.MaxScore, RiskLevel: v.RiskLevel, IsTotalScore: v.IsTotalScore})
	}
	return &Score{AssessmentID: f.AssessmentID, TotalScore: f.TotalScore, RiskLevel: f.RiskLevel, FactorScores: factors}
}

func deref(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
