// Package testee contains Evaluation queries performed by a participant.
//
// The actor identity is part of every use case. Ownership checks, query
// normalization and score-fact projection therefore remain inside this
// package instead of leaking into gRPC transports.
package testee

import (
	"context"
	"time"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	legacy "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

type Actor struct {
	TesteeID uint64
}

type ListQuery struct {
	Page      int
	PageSize  int
	Status    string
	ScaleCode string
	RiskLevel string
	ModelKind string
	ModelCode string
	DateFrom  string
	DateTo    string
}

type TrendQuery struct {
	FactorCode string
	Limit      int
}

type Assessment = legacy.AssessmentOutcomeResult
type AssessmentList = legacy.AssessmentOutcomeListResult
type Score = legacy.ScoreResult
type FactorTrend = legacy.FactorTrendResult
type HighRiskFactors = legacy.HighRiskFactorsResult

// ScoreReader is an internal query mechanism consumed by the testee actor
// service. It is deliberately not exposed to transports or containers as an
// actor-neutral application service.
type ScoreReader interface {
	GetByAssessmentID(context.Context, uint64) (*legacy.ScoreResult, error)
	GetFactorTrend(context.Context, legacy.GetFactorTrendDTO) (*legacy.FactorTrendResult, error)
	GetHighRiskFactors(context.Context, uint64) (*legacy.HighRiskFactorsResult, error)
}

type OwnershipReader interface {
	GetMine(context.Context, uint64, uint64) (*legacy.AssessmentResult, error)
}

type Service interface {
	GetAssessment(context.Context, Actor, uint64) (*Assessment, error)
	ListAssessments(context.Context, Actor, ListQuery) (*AssessmentList, error)
	GetScore(context.Context, Actor, uint64) (*Score, error)
	GetFactorTrend(context.Context, Actor, TrendQuery) (*FactorTrend, error)
	GetHighRiskFactors(context.Context, Actor, uint64) (*HighRiskFactors, error)
}

type service struct {
	ownership OwnershipReader
	reader    evaluationreadmodel.AssessmentReader
	scores    ScoreReader
}

func NewService(ownership OwnershipReader, reader evaluationreadmodel.AssessmentReader, scores ScoreReader) Service {
	return &service{ownership: ownership, reader: reader, scores: scores}
}

func (s *service) GetAssessment(ctx context.Context, actor Actor, assessmentID uint64) (*Assessment, error) {
	if err := s.authorizeAssessment(ctx, actor, assessmentID); err != nil {
		return nil, err
	}
	if s.reader == nil {
		return nil, evalerrors.ModuleNotConfigured("assessment read model is not configured")
	}
	row, err := s.reader.GetAssessment(ctx, assessmentID)
	if err != nil {
		return nil, err
	}
	return legacy.RowToOutcomeResult(*row)
}

func (s *service) ListAssessments(ctx context.Context, actor Actor, query ListQuery) (*AssessmentList, error) {
	if actor.TesteeID == 0 {
		return nil, evalerrors.InvalidArgument("受试者ID不能为空")
	}
	if s.reader == nil {
		return nil, evalerrors.ModuleNotConfigured("assessment read model is not configured")
	}
	page, pageSize := normalizePagination(query.Page, query.PageSize)
	dateFrom, err := parseDate(query.DateFrom, false)
	if err != nil {
		return nil, evalerrors.InvalidArgument("date_from 格式不正确")
	}
	dateTo, err := parseDate(query.DateTo, true)
	if err != nil {
		return nil, evalerrors.InvalidArgument("date_to 格式不正确")
	}
	rows, total, err := s.reader.ListAssessments(ctx, evaluationreadmodel.AssessmentFilter{
		TesteeID:  &actor.TesteeID,
		Statuses:  normalizeStatuses(query.Status),
		ScaleCode: query.ScaleCode,
		RiskLevel: query.RiskLevel,
		ModelKind: query.ModelKind,
		ModelCode: query.ModelCode,
		DateFrom:  dateFrom,
		DateTo:    dateTo,
	}, evaluationreadmodel.PageRequest{Page: page, PageSize: pageSize})
	if err != nil {
		return nil, evalerrors.Database(err, "查询测评列表失败")
	}
	items, err := legacy.RowsToOutcomeResults(rows)
	if err != nil {
		return nil, err
	}
	totalInt, err := safeconv.Int64ToInt(total)
	if err != nil {
		return nil, evalerrors.DatabaseMessage("测评总数超出安全范围")
	}
	return &AssessmentList{Items: items, Total: totalInt, Page: page, PageSize: pageSize, TotalPages: (totalInt + pageSize - 1) / pageSize}, nil
}

func (s *service) GetScore(ctx context.Context, actor Actor, assessmentID uint64) (*Score, error) {
	if err := s.authorizeAssessment(ctx, actor, assessmentID); err != nil {
		return nil, err
	}
	if s.scores == nil {
		return nil, evalerrors.ModuleNotConfigured("score reader is not configured")
	}
	return s.scores.GetByAssessmentID(ctx, assessmentID)
}

func (s *service) GetFactorTrend(ctx context.Context, actor Actor, query TrendQuery) (*FactorTrend, error) {
	if actor.TesteeID == 0 {
		return nil, evalerrors.InvalidArgument("受试者ID不能为空")
	}
	if s.scores == nil {
		return nil, evalerrors.ModuleNotConfigured("score reader is not configured")
	}
	return s.scores.GetFactorTrend(ctx, legacy.GetFactorTrendDTO{TesteeID: actor.TesteeID, FactorCode: query.FactorCode, Limit: query.Limit})
}

func (s *service) GetHighRiskFactors(ctx context.Context, actor Actor, assessmentID uint64) (*HighRiskFactors, error) {
	if err := s.authorizeAssessment(ctx, actor, assessmentID); err != nil {
		return nil, err
	}
	if s.scores == nil {
		return nil, evalerrors.ModuleNotConfigured("score reader is not configured")
	}
	return s.scores.GetHighRiskFactors(ctx, assessmentID)
}

func (s *service) authorizeAssessment(ctx context.Context, actor Actor, assessmentID uint64) error {
	if actor.TesteeID == 0 || assessmentID == 0 {
		return evalerrors.InvalidArgument("受试者ID和测评ID不能为空")
	}
	if s.ownership == nil {
		return evalerrors.ModuleNotConfigured("assessment ownership reader is not configured")
	}
	_, err := s.ownership.GetMine(ctx, actor.TesteeID, assessmentID)
	return err
}

func normalizePagination(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func parseDate(raw string, endExclusive bool) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		parsed, err := time.Parse(layout, raw)
		if err != nil {
			continue
		}
		if layout == "2006-01-02" && endExclusive {
			parsed = parsed.Add(24 * time.Hour)
		}
		return &parsed, nil
	}
	return nil, evalerrors.InvalidArgument("日期格式不正确")
}

func normalizeStatuses(raw string) []string {
	switch raw {
	case "":
		return nil
	case "pending":
		return []string{"pending", "submitted"}
	case "done":
		return []string{"evaluated"}
	default:
		return []string{raw}
	}
}
