package operator

import (
	"context"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	evaluationoutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

func (s *queryService) ScopeTesteeList(ctx context.Context, actor Actor, testeeID uint64) (TesteeListScope, error) {
	result := TesteeListScope{TesteeID: testeeID}
	if testeeID != 0 {
		return result, s.ValidateTesteeAccess(ctx, actor, testeeID)
	}
	if s.access == nil {
		return result, evalerrors.ModuleNotConfigured("testee access checker is not configured")
	}
	scope, err := s.access.ResolveAccessScope(ctx, actor.OrgID, actor.OperatorUserID)
	if err != nil {
		return result, err
	}
	if scope != nil && scope.IsAdmin {
		return result, nil
	}
	result.AccessibleTesteeIDs, err = s.access.ListAccessibleTesteeIDs(ctx, actor.OrgID, actor.OperatorUserID)
	result.Restricted = true
	return result, err
}

type queryService struct {
	assessments domainassessment.Repository
	reader      evaluationreadmodel.AssessmentReader
	access      AccessChecker
	scores      evaluationoutcome.ScoreFactReader
	runs        evaluationrun.Repository
}

func NewQueryService(assessments domainassessment.Repository, reader evaluationreadmodel.AssessmentReader, access AccessChecker, scores evaluationoutcome.ScoreFactReader, runs evaluationrun.Repository) QueryService {
	return &queryService{assessments: assessments, reader: reader, access: access, scores: scores, runs: runs}
}

func (s *queryService) ValidateTesteeAccess(ctx context.Context, actor Actor, testeeID uint64) error {
	if actor.OrgID == 0 || actor.OperatorUserID == 0 {
		return evalerrors.InvalidArgument("操作者范围不能为空")
	}
	if s.access == nil {
		return evalerrors.ModuleNotConfigured("testee access checker is not configured")
	}
	return s.access.ValidateTesteeAccess(ctx, actor.OrgID, actor.OperatorUserID, testeeID)
}

func (s *queryService) loadAccessible(ctx context.Context, actor Actor, id uint64) (*domainassessment.Assessment, error) {
	return (authorizer{assessments: s.assessments, access: s.access}).loadAssessment(ctx, actor, id)
}

func (s *queryService) GetAssessment(ctx context.Context, actor Actor, id uint64) (*Assessment, error) {
	a, err := s.loadAccessible(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	return assessmentFromDomain(a)
}

func (s *queryService) scopedList(ctx context.Context, actor Actor, q ListQuery) (ListQuery, error) {
	if actor.OrgID == 0 || actor.OperatorUserID == 0 {
		return q, evalerrors.InvalidArgument("操作者范围不能为空")
	}
	if s.access == nil {
		return q, evalerrors.ModuleNotConfigured("testee access checker is not configured")
	}
	if q.TesteeID != nil {
		if err := s.access.ValidateTesteeAccess(ctx, actor.OrgID, actor.OperatorUserID, *q.TesteeID); err != nil {
			return q, err
		}
		return q, nil
	}
	scope, err := s.access.ResolveAccessScope(ctx, actor.OrgID, actor.OperatorUserID)
	if err != nil {
		return q, err
	}
	if scope != nil && scope.IsAdmin {
		return q, nil
	}
	q.AccessibleTesteeIDs, err = s.access.ListAccessibleTesteeIDs(ctx, actor.OrgID, actor.OperatorUserID)
	q.RestrictToAccessScope = true
	return q, err
}

func (s *queryService) listRows(ctx context.Context, actor Actor, q ListQuery) ([]evaluationreadmodel.AssessmentRow, int64, int, int, error) {
	if s.reader == nil {
		return nil, 0, 0, 0, evalerrors.ModuleNotConfigured("assessment read model is not configured")
	}
	q, err := s.scopedList(ctx, actor, q)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	page, pageSize := normalizePagination(q.Page, q.PageSize)
	filter := evaluationreadmodel.AssessmentFilter{OrgID: actor.OrgID, TesteeID: q.TesteeID, AccessibleTesteeIDs: q.AccessibleTesteeIDs, RestrictToAccessScope: q.RestrictToAccessScope}
	if q.Status != "" {
		status := domainassessment.Status(q.Status)
		if !status.IsValid() {
			return []evaluationreadmodel.AssessmentRow{}, 0, page, pageSize, nil
		}
		filter.Statuses = []string{status.String()}
	}
	rows, total, err := s.reader.ListAssessments(ctx, filter, evaluationreadmodel.PageRequest{Page: page, PageSize: pageSize})
	if err != nil {
		return nil, 0, 0, 0, evalerrors.Database(err, "查询测评列表失败")
	}
	return rows, total, page, pageSize, nil
}

func (s *queryService) ListAssessments(ctx context.Context, actor Actor, q ListQuery) (*AssessmentList, error) {
	rows, total, page, pageSize, err := s.listRows(ctx, actor, q)
	if err != nil {
		return nil, err
	}
	items := make([]*Assessment, 0, len(rows))
	for _, row := range rows {
		item, mapErr := assessmentFromRow(row)
		if mapErr != nil {
			return nil, mapErr
		}
		items = append(items, item)
	}
	return assessmentList(items, total, page, pageSize)
}

func (s *queryService) GetAssessmentOutcome(ctx context.Context, actor Actor, id uint64) (*OutcomeAssessment, error) {
	if _, err := s.loadAccessible(ctx, actor, id); err != nil {
		return nil, err
	}
	if s.reader == nil {
		return nil, evalerrors.ModuleNotConfigured("assessment read model is not configured")
	}
	row, err := s.reader.GetAssessment(ctx, id)
	if err != nil {
		return nil, evalerrors.AssessmentNotFound(err, "测评不存在")
	}
	return outcomeFromRow(*row)
}

func (s *queryService) ListAssessmentsOutcome(ctx context.Context, actor Actor, q ListQuery) (*OutcomeAssessmentList, error) {
	rows, total, page, pageSize, err := s.listRows(ctx, actor, q)
	if err != nil {
		return nil, err
	}
	items := make([]*OutcomeAssessment, 0, len(rows))
	for _, row := range rows {
		item, mapErr := outcomeFromRow(row)
		if mapErr != nil {
			return nil, mapErr
		}
		items = append(items, item)
	}
	count, err := safeconv.Int64ToInt(total)
	if err != nil {
		return nil, evalerrors.DatabaseMessage("测评总数超出安全范围")
	}
	return &OutcomeAssessmentList{Items: items, Total: count, Page: page, PageSize: pageSize, TotalPages: pages(count, pageSize)}, nil
}

func (s *queryService) GetScores(ctx context.Context, actor Actor, id uint64) (*Score, error) {
	if _, err := s.loadAccessible(ctx, actor, id); err != nil {
		return nil, err
	}
	if s.scores == nil {
		return nil, evalerrors.ModuleNotConfigured("evaluation score fact reader is not configured")
	}
	fact, err := s.scores.Get(ctx, id)
	if err != nil {
		return nil, evalerrors.AssessmentScoreNotFound(err, "得分不存在")
	}
	return scoreFromFact(fact), nil
}

func (s *queryService) GetHighRiskFactors(ctx context.Context, actor Actor, id uint64) (*HighRiskFactors, error) {
	if _, err := s.loadAccessible(ctx, actor, id); err != nil {
		return nil, err
	}
	if s.scores == nil {
		return nil, evalerrors.ModuleNotConfigured("evaluation score fact reader is not configured")
	}
	fact, err := s.scores.Get(ctx, id)
	if err != nil {
		return &HighRiskFactors{AssessmentID: id, HighRiskFactors: []FactorScore{}}, nil
	}
	score := scoreFromFact(fact)
	result := &HighRiskFactors{AssessmentID: id, HighRiskFactors: []FactorScore{}}
	for _, factor := range score.FactorScores {
		if factor.RiskLevel == "high" || factor.RiskLevel == "severe" {
			result.HighRiskFactors = append(result.HighRiskFactors, factor)
		}
	}
	result.HasHighRisk = len(result.HighRiskFactors) > 0 || score.RiskLevel == "high" || score.RiskLevel == "severe"
	result.NeedsUrgentCare = score.RiskLevel == "severe"
	return result, nil
}

func (s *queryService) GetFactorTrend(ctx context.Context, actor Actor, q TrendQuery) (*FactorTrend, error) {
	if err := s.ValidateTesteeAccess(ctx, actor, q.TesteeID); err != nil {
		return nil, err
	}
	if s.scores == nil {
		return nil, evalerrors.ModuleNotConfigured("evaluation score fact reader is not configured")
	}
	fact, err := s.scores.Trend(ctx, q.TesteeID, q.FactorCode, q.Limit)
	if err != nil {
		return nil, err
	}
	result := &FactorTrend{TesteeID: fact.TesteeID, FactorCode: fact.FactorCode, FactorName: fact.FactorName, DataPoints: make([]TrendPoint, 0, len(fact.DataPoints))}
	for _, point := range fact.DataPoints {
		result.DataPoints = append(result.DataPoints, TrendPoint{AssessmentID: point.AssessmentID, RawScore: point.RawScore, RiskLevel: point.RiskLevel})
	}
	return result, nil
}

func (s *queryService) ListAssessmentRuns(ctx context.Context, actor Actor, id uint64, limit int) (*RunList, error) {
	if _, err := s.loadAccessible(ctx, actor, id); err != nil {
		return nil, err
	}
	if s.runs == nil {
		return nil, evalerrors.ModuleNotConfigured("evaluation run repository is not configured")
	}
	limit = normalizeLimit(limit, 20, 100)
	runs, err := s.runs.ListByAssessmentID(ctx, id, limit)
	if err != nil {
		return nil, err
	}
	items := make([]*Run, 0, len(runs))
	for _, run := range runs {
		items = append(items, runFromDomain(run))
	}
	return &RunList{Items: items}, nil
}

func (s *queryService) GetLatestAssessmentRun(ctx context.Context, actor Actor, id uint64) (*Run, error) {
	if _, err := s.loadAccessible(ctx, actor, id); err != nil {
		return nil, err
	}
	if s.runs == nil {
		return nil, evalerrors.ModuleNotConfigured("evaluation run repository is not configured")
	}
	run, err := s.runs.FindLatestByAssessmentID(ctx, id)
	if err != nil || run == nil {
		return nil, err
	}
	return runFromDomain(*run), nil
}

func (s *queryService) ListRetryableFailedRuns(ctx context.Context, actor Actor, limit int, cursor uint64) (*RetryableFailedRunList, error) {
	if actor.OrgID <= 0 || actor.OperatorUserID <= 0 {
		return nil, evalerrors.InvalidArgument("操作者范围不能为空")
	}
	if s.runs == nil {
		return nil, evalerrors.ModuleNotConfigured("evaluation run repository is not configured")
	}
	page, err := s.runs.ListRetryableFailed(ctx, evaluationrun.ListRetryableFailedParams{OrgID: actor.OrgID, Limit: normalizeLimit(limit, 50, 200), Cursor: cursor})
	if err != nil {
		return nil, err
	}
	result := &RetryableFailedRunList{}
	if page == nil {
		return result, nil
	}
	result.NextCursor = page.NextCursor
	result.Items = make([]*RetryableFailedRun, 0, len(page.Items))
	for _, item := range page.Items {
		result.Items = append(result.Items, &RetryableFailedRun{Run: *runFromDomain(item.Run), OrgID: item.OrgID})
	}
	return result, nil
}
