// Package testee contains Evaluation queries performed by a participant.
package testee

import (
	"context"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

type service struct {
	assessments domainassessment.Repository
	reader      evaluationreadmodel.AssessmentReader
	scores      evaloutcome.ScoreFactReader
	accessCache AssessmentAccessCache
	detailCache AssessmentDetailCache
}

func NewService(assessments domainassessment.Repository, reader evaluationreadmodel.AssessmentReader, scores evaloutcome.ScoreFactReader) Service {
	return &service{assessments: assessments, reader: reader, scores: scores}
}

func NewServiceWithCaches(assessments domainassessment.Repository, reader evaluationreadmodel.AssessmentReader, scores evaloutcome.ScoreFactReader, accessCache AssessmentAccessCache, detailCache AssessmentDetailCache) Service {
	return &service{assessments: assessments, reader: reader, scores: scores, accessCache: accessCache, detailCache: detailCache}
}

func (s *service) AuthorizeAssessment(ctx context.Context, actor Actor, id uint64) error {
	if actor.TesteeID == 0 || id == 0 {
		return evalerrors.InvalidArgument("受试者ID和测评ID不能为空")
	}
	if s.assessments == nil {
		return evalerrors.ModuleNotConfigured("assessment repository is not configured")
	}
	loadOwner := func(loadCtx context.Context) (uint64, error) {
		a, err := s.assessments.FindByID(loadCtx, meta.FromUint64(id))
		if err != nil {
			return 0, evalerrors.AssessmentNotFound(err, "测评不存在")
		}
		return a.TesteeID().Uint64(), nil
	}
	var owner uint64
	var err error
	if s.accessCache != nil {
		owner, err = s.accessCache.ReadOwner(ctx, id, loadOwner)
	} else {
		owner, err = loadOwner(ctx)
	}
	if err != nil {
		return err
	}
	if owner != actor.TesteeID {
		return evalerrors.Forbidden("无权访问此测评")
	}
	return nil
}
func (s *service) GetAssessment(ctx context.Context, actor Actor, id uint64) (*Assessment, error) {
	if err := s.AuthorizeAssessment(ctx, actor, id); err != nil {
		return nil, err
	}
	if s.reader == nil {
		return nil, evalerrors.ModuleNotConfigured("assessment read model is not configured")
	}
	loadDetail := func(loadCtx context.Context) (*Assessment, error) {
		row, err := s.reader.GetAssessment(loadCtx, id)
		if err != nil {
			return nil, err
		}
		return assessmentFromRow(*row)
	}
	if s.detailCache == nil {
		return loadDetail(ctx)
	}
	result, err := s.detailCache.ReadDetail(ctx, id, loadDetail)
	return cloneAssessment(result), err
}

func cloneAssessment(value *Assessment) *Assessment {
	if value == nil {
		return nil
	}
	cloned := *value
	if value.PrimaryScore != nil {
		primary := *value.PrimaryScore
		if value.PrimaryScore.Max != nil {
			max := *value.PrimaryScore.Max
			primary.Max = &max
		}
		cloned.PrimaryScore = &primary
	}
	if value.Level != nil {
		level := *value.Level
		cloned.Level = &level
	}
	if value.OriginID != nil {
		originID := *value.OriginID
		cloned.OriginID = &originID
	}
	if value.SubmittedAt != nil {
		submittedAt := *value.SubmittedAt
		cloned.SubmittedAt = &submittedAt
	}
	if value.FailedAt != nil {
		failedAt := *value.FailedAt
		cloned.FailedAt = &failedAt
	}
	if value.FailureReason != nil {
		reason := *value.FailureReason
		cloned.FailureReason = &reason
	}
	return &cloned
}
func (s *service) ListAssessments(ctx context.Context, actor Actor, q ListQuery) (*AssessmentList, error) {
	if actor.TesteeID == 0 {
		return nil, evalerrors.InvalidArgument("受试者ID不能为空")
	}
	if s.reader == nil {
		return nil, evalerrors.ModuleNotConfigured("assessment read model is not configured")
	}
	page, size := normalizePagination(q.Page, q.PageSize)
	from, err := parseDate(q.DateFrom, false)
	if err != nil {
		return nil, evalerrors.InvalidArgument("date_from 格式不正确")
	}
	to, err := parseDate(q.DateTo, true)
	if err != nil {
		return nil, evalerrors.InvalidArgument("date_to 格式不正确")
	}
	rows, total, err := s.reader.ListAssessments(ctx, evaluationreadmodel.AssessmentFilter{TesteeID: &actor.TesteeID, Statuses: normalizeStatuses(q.Status), ScaleCode: q.ScaleCode, RiskLevel: q.RiskLevel, ModelKind: q.ModelKind, ModelKinds: q.ModelKinds, ModelCode: q.ModelCode, DateFrom: from, DateTo: to}, evaluationreadmodel.PageRequest{Page: page, PageSize: size})
	if err != nil {
		return nil, evalerrors.Database(err, "查询测评列表失败")
	}
	items := make([]*Assessment, 0, len(rows))
	for _, row := range rows {
		item, mapErr := assessmentFromRow(row)
		if mapErr != nil {
			return nil, mapErr
		}
		items = append(items, item)
	}
	count, err := safeconv.Int64ToInt(total)
	if err != nil {
		return nil, evalerrors.DatabaseMessage("测评总数超出安全范围")
	}
	return &AssessmentList{Items: items, Total: count, Page: page, PageSize: size, TotalPages: (count + size - 1) / size}, nil
}
func (s *service) GetScore(ctx context.Context, actor Actor, id uint64) (*Score, error) {
	if err := s.AuthorizeAssessment(ctx, actor, id); err != nil {
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
func (s *service) GetFactorTrend(ctx context.Context, actor Actor, q TrendQuery) (*FactorTrend, error) {
	if actor.TesteeID == 0 {
		return nil, evalerrors.InvalidArgument("受试者ID不能为空")
	}
	if s.scores == nil {
		return nil, evalerrors.ModuleNotConfigured("evaluation score fact reader is not configured")
	}
	fact, err := s.scores.Trend(ctx, actor.TesteeID, q.FactorCode, q.Limit)
	if err != nil {
		return nil, err
	}
	points := make([]TrendPoint, 0, len(fact.DataPoints))
	for _, p := range fact.DataPoints {
		points = append(points, TrendPoint{AssessmentID: p.AssessmentID, RawScore: p.RawScore, RiskLevel: p.RiskLevel})
	}
	return &FactorTrend{TesteeID: fact.TesteeID, FactorCode: fact.FactorCode, FactorName: fact.FactorName, DataPoints: points}, nil
}
func (s *service) GetHighRiskFactors(ctx context.Context, actor Actor, id uint64) (*HighRiskFactors, error) {
	if err := s.AuthorizeAssessment(ctx, actor, id); err != nil {
		return nil, err
	}
	if s.scores == nil {
		return nil, evalerrors.ModuleNotConfigured("evaluation score fact reader is not configured")
	}
	fact, err := s.scores.Get(ctx, id)
	if err != nil {
		return &HighRiskFactors{AssessmentID: id}, nil
	}
	score := scoreFromFact(fact)
	result := &HighRiskFactors{AssessmentID: id, HighRiskFactors: []FactorScore{}}
	for _, f := range score.FactorScores {
		if f.RiskLevel == "high" || f.RiskLevel == "severe" {
			result.HighRiskFactors = append(result.HighRiskFactors, f)
		}
	}
	result.HasHighRisk = len(result.HighRiskFactors) > 0 || score.RiskLevel == "high" || score.RiskLevel == "severe"
	result.NeedsUrgentCare = score.RiskLevel == "severe"
	return result, nil
}
