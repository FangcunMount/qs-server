package operator

import (
	"context"

	legacy "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
)

type Assessment = legacy.AssessmentResult
type AssessmentList = legacy.AssessmentListResult
type AssessmentOutcome = legacy.AssessmentOutcomeResult
type AssessmentOutcomeList = legacy.AssessmentOutcomeListResult
type Score = legacy.ScoreResult
type HighRiskFactors = legacy.HighRiskFactorsResult
type FactorTrend = legacy.FactorTrendResult
type Run = legacy.AssessmentRunResult
type RunList = legacy.AssessmentRunListResult
type ListQuery = legacy.ListAssessmentsDTO
type TrendQuery = legacy.GetFactorTrendDTO

type QueryService interface {
	GetAssessment(context.Context, Actor, uint64) (*Assessment, error)
	ListAssessments(context.Context, Actor, ListQuery) (*AssessmentList, error)
	GetAssessmentOutcome(context.Context, Actor, uint64) (*AssessmentOutcome, error)
	ListAssessmentsOutcome(context.Context, Actor, ListQuery) (*AssessmentOutcomeList, error)
	GetScores(context.Context, Actor, uint64) (*Score, error)
	GetHighRiskFactors(context.Context, Actor, uint64) (*HighRiskFactors, error)
	GetFactorTrend(context.Context, Actor, TrendQuery) (*FactorTrend, error)
	ListAssessmentRuns(context.Context, Actor, uint64, int) (*RunList, error)
	GetLatestAssessmentRun(context.Context, Actor, uint64) (*Run, error)
}

type queryService struct {
	delegate legacy.AssessmentProtectedQueryService
}

func NewQueryService(delegate legacy.AssessmentProtectedQueryService) QueryService {
	return &queryService{delegate: delegate}
}

func scope(actor Actor) legacy.ProtectedQueryScope {
	return legacy.ProtectedQueryScope{OrgID: actor.OrgID, OperatorUserID: actor.OperatorUserID}
}

func (s *queryService) GetAssessment(ctx context.Context, a Actor, id uint64) (*Assessment, error) {
	return s.delegate.GetAssessment(ctx, scope(a), id)
}
func (s *queryService) ListAssessments(ctx context.Context, a Actor, q ListQuery) (*AssessmentList, error) {
	return s.delegate.ListAssessments(ctx, scope(a), q)
}
func (s *queryService) GetAssessmentOutcome(ctx context.Context, a Actor, id uint64) (*AssessmentOutcome, error) {
	return s.delegate.GetAssessmentOutcome(ctx, scope(a), id)
}
func (s *queryService) ListAssessmentsOutcome(ctx context.Context, a Actor, q ListQuery) (*AssessmentOutcomeList, error) {
	return s.delegate.ListAssessmentsOutcome(ctx, scope(a), q)
}
func (s *queryService) GetScores(ctx context.Context, a Actor, id uint64) (*Score, error) {
	return s.delegate.GetScores(ctx, scope(a), id)
}
func (s *queryService) GetHighRiskFactors(ctx context.Context, a Actor, id uint64) (*HighRiskFactors, error) {
	return s.delegate.GetHighRiskFactors(ctx, scope(a), id)
}
func (s *queryService) GetFactorTrend(ctx context.Context, a Actor, q TrendQuery) (*FactorTrend, error) {
	return s.delegate.GetFactorTrend(ctx, scope(a), q)
}
func (s *queryService) ListAssessmentRuns(ctx context.Context, a Actor, id uint64, limit int) (*RunList, error) {
	return s.delegate.ListAssessmentRuns(ctx, scope(a), id, limit)
}
func (s *queryService) GetLatestAssessmentRun(ctx context.Context, a Actor, id uint64) (*Run, error) {
	return s.delegate.GetLatestAssessmentRun(ctx, scope(a), id)
}

type RecoveryService interface {
	Retry(context.Context, Actor, uint64) (*Assessment, error)
}

type recoveryService struct {
	delegate legacy.AssessmentOperatorRecoveryService
}

func NewRecoveryService(delegate legacy.AssessmentOperatorRecoveryService) RecoveryService {
	return &recoveryService{delegate: delegate}
}

func (s *recoveryService) Retry(ctx context.Context, actor Actor, id uint64) (*Assessment, error) {
	return s.delegate.Retry(ctx, actor.OrgID, id)
}
