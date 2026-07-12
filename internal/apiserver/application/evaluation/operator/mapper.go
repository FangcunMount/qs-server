package operator

import (
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	evaluationoutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelbinding "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

func assessmentFromDomain(a *domainassessment.Assessment) (*Assessment, error) {
	if a == nil {
		return nil, nil
	}
	orgID, err := safeconv.Int64ToUint64(a.OrgID())
	if err != nil {
		return nil, evalerrors.DatabaseMessage("测评机构ID超出安全范围")
	}
	result := &Assessment{ID: a.ID().Uint64(), OrgID: orgID, TesteeID: a.TesteeID().Uint64(), QuestionnaireCode: a.QuestionnaireRef().Code().String(), QuestionnaireVersion: a.QuestionnaireRef().Version(), AnswerSheetID: a.AnswerSheetRef().ID().Uint64(), OriginType: a.OriginType().String(), OriginID: a.OriginID(), Status: a.Status().String(), TotalScore: a.TotalScore(), SubmittedAt: a.SubmittedAt(), EvaluatedAt: a.EvaluatedAt(), FailedAt: a.FailedAt(), FailureReason: a.FailureReason()}
	if risk := a.RiskLevel(); risk != nil {
		value := string(*risk)
		result.RiskLevel = &value
	}
	if model := a.EvaluationModelRef(); model != nil && !model.IsEmpty() {
		kind, sub, algorithm, code, version, title := model.Kind().String(), string(model.SubKind()), string(model.Algorithm()), model.Code().String(), model.Version(), model.Title()
		result.ModelKind, result.ModelSubKind, result.ModelAlgorithm, result.ModelCode, result.ModelVersion, result.ModelTitle = &kind, &sub, &algorithm, &code, &version, &title
	}
	return result, nil
}

func assessmentFromRow(row evaluationreadmodel.AssessmentRow) (*Assessment, error) {
	orgID, err := safeconv.Int64ToUint64(row.OrgID)
	if err != nil {
		return nil, evalerrors.DatabaseMessage("测评机构ID超出安全范围")
	}
	return &Assessment{ID: row.ID, OrgID: orgID, TesteeID: row.TesteeID, QuestionnaireCode: row.QuestionnaireCode, QuestionnaireVersion: row.QuestionnaireVersion, AnswerSheetID: row.AnswerSheetID, ModelKind: row.EvaluationModelKind, ModelSubKind: row.EvaluationModelSubKind, ModelAlgorithm: row.EvaluationModelAlgorithm, ModelCode: row.EvaluationModelCode, ModelVersion: row.EvaluationModelVersion, ModelTitle: row.EvaluationModelTitle, OriginType: row.OriginType, OriginID: row.OriginID, Status: row.Status, TotalScore: row.TotalScore, RiskLevel: row.RiskLevel, SubmittedAt: row.SubmittedAt, EvaluatedAt: row.EvaluatedAt, FailedAt: row.FailedAt, FailureReason: row.FailureReason}, nil
}

func outcomeFromRow(row evaluationreadmodel.AssessmentRow) (*OutcomeAssessment, error) {
	base, err := assessmentFromRow(row)
	if err != nil {
		return nil, err
	}
	return &OutcomeAssessment{ID: base.ID, OrgID: base.OrgID, TesteeID: base.TesteeID, QuestionnaireCode: base.QuestionnaireCode, QuestionnaireVersion: base.QuestionnaireVersion, AnswerSheetID: base.AnswerSheetID, Model: modelFromRow(row), PrimaryScore: primaryScoreFromRow(row), Level: levelFromRow(row), OriginType: base.OriginType, OriginID: base.OriginID, Status: base.Status, SubmittedAt: base.SubmittedAt, FailedAt: base.FailedAt, FailureReason: base.FailureReason}, nil
}

func modelFromRow(row evaluationreadmodel.AssessmentRow) ModelIdentity {
	kind, sub, algorithm := deref(row.EvaluationModelKind), deref(row.EvaluationModelSubKind), deref(row.EvaluationModelAlgorithm)
	if algorithm == "" && kind != "" {
		if k, s, a, ok := modelcatalog.LegacyKindMapping(modelcatalog.Kind(kind)); ok {
			kind = string(k)
			if sub == "" {
				sub = string(s)
			}
			algorithm = string(a)
		}
	}
	result := ModelIdentity{Kind: kind, SubKind: sub, Algorithm: algorithm, Code: deref(row.EvaluationModelCode), Version: deref(row.EvaluationModelVersion), Title: deref(row.EvaluationModelTitle)}
	k := modelbinding.Kind(result.Kind)
	result.ProductChannel = modelbinding.ProductChannelForIdentity(k, "")
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

func scoreFromFact(fact *evaluationoutcome.ScoreFact) *Score {
	result := &Score{AssessmentID: fact.AssessmentID, TotalScore: fact.TotalScore, RiskLevel: fact.RiskLevel, FactorScores: make([]FactorScore, 0, len(fact.FactorScores))}
	for _, factor := range fact.FactorScores {
		result.FactorScores = append(result.FactorScores, FactorScore{FactorCode: factor.FactorCode, FactorName: factor.FactorName, RawScore: factor.RawScore, MaxScore: factor.MaxScore, RiskLevel: factor.RiskLevel, IsTotalScore: factor.IsTotalScore})
	}
	return result
}

func runFromDomain(run evalrun.EvaluationRun) *Run {
	attempt := run.Attempt()
	result := &Run{RunID: run.ID().String(), AssessmentID: run.AssessmentID(), AttemptNo: attempt.Number, Status: attempt.Status.String(), Retryable: run.Retryable(), StartedAt: run.StartedAt(), FinishedAt: run.FinishedAt(), TraceID: run.TraceID(), InputSnapshotRef: run.InputSnapshotRef()}
	if failure := run.Failure(); failure != nil {
		result.ErrorCode, result.ErrorMessage, result.Retryable = failure.Kind.String(), failure.Message, failure.Retryable
	}
	return result
}

func assessmentList(items []*Assessment, total int64, page, pageSize int) (*AssessmentList, error) {
	count, err := safeconv.Int64ToInt(total)
	if err != nil {
		return nil, evalerrors.DatabaseMessage("测评总数超出安全范围")
	}
	return &AssessmentList{Items: items, Total: count, Page: page, PageSize: pageSize, TotalPages: pages(count, pageSize)}, nil
}

func normalizePagination(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func normalizeLimit(limit, defaultValue, max int) int {
	if limit <= 0 {
		return defaultValue
	}
	if limit > max {
		return max
	}
	return limit
}

func pages(total, pageSize int) int {
	if total == 0 {
		return 0
	}
	return (total + pageSize - 1) / pageSize
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
