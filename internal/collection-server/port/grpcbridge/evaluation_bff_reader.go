package grpcbridge

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
)

// EvaluationBFFReader 将 infra gRPC 输出转换为 evaluation application DTO。
type EvaluationBFFReader struct {
	inner EvaluationReader
}

// NewEvaluationBFFReader 构造测评 BFF ACL 适配器。
func NewEvaluationBFFReader(inner EvaluationReader) *EvaluationBFFReader {
	return &EvaluationBFFReader{inner: inner}
}

func (r *EvaluationBFFReader) GetAssessmentScores(ctx context.Context, testeeID, assessmentID uint64) ([]evaluation.FactorScoreResponse, error) {
	if r == nil || r.inner == nil {
		return nil, nil
	}
	out, err := r.inner.GetAssessmentScores(ctx, testeeID, assessmentID)
	if err != nil {
		return nil, err
	}
	return toFactorScoreResponses(out), nil
}

func (r *EvaluationBFFReader) GetFactorTrend(ctx context.Context, testeeID uint64, factorCode string, limit int32) ([]evaluation.TrendPointResponse, error) {
	if r == nil || r.inner == nil {
		return nil, nil
	}
	out, err := r.inner.GetFactorTrend(ctx, testeeID, factorCode, limit)
	if err != nil {
		return nil, err
	}
	return toTrendPointResponses(out), nil
}

func (r *EvaluationBFFReader) GetHighRiskFactors(ctx context.Context, testeeID, assessmentID uint64) ([]evaluation.FactorScoreResponse, error) {
	if r == nil || r.inner == nil {
		return nil, nil
	}
	out, err := r.inner.GetHighRiskFactors(ctx, testeeID, assessmentID)
	if err != nil {
		return nil, err
	}
	return toFactorScoreResponses(out), nil
}

func (r *EvaluationBFFReader) GetMyAssessment(ctx context.Context, testeeID, assessmentID uint64) (*evaluation.AssessmentDetailResponse, error) {
	if r == nil || r.inner == nil {
		return nil, nil
	}
	out, err := r.inner.GetMyAssessment(ctx, testeeID, assessmentID)
	if err != nil {
		return nil, err
	}
	return toAssessmentDetailResponse(out), nil
}

func (r *EvaluationBFFReader) ListMyAssessments(ctx context.Context, testeeID uint64, status, scaleCode, riskLevel, dateFrom, dateTo, modelKind string, page, pageSize int32) (*evaluation.ListAssessmentsResponse, error) {
	if r == nil || r.inner == nil {
		return nil, nil
	}
	out, err := r.inner.ListMyAssessments(ctx, testeeID, status, scaleCode, riskLevel, dateFrom, dateTo, modelKind, page, pageSize)
	if err != nil {
		return nil, err
	}
	return toListAssessmentsResponse(out), nil
}

func (r *EvaluationBFFReader) GetAssessmentReport(ctx context.Context, testeeID, assessmentID uint64) (*evaluation.AssessmentReportResponse, error) {
	if r == nil || r.inner == nil {
		return nil, nil
	}
	out, err := r.inner.GetAssessmentReport(ctx, testeeID, assessmentID)
	if err != nil {
		return nil, err
	}
	return toAssessmentReportResponse(out), nil
}

func (r *EvaluationBFFReader) ResolveAssessmentByAnswerSheetID(ctx context.Context, answerSheetID uint64) (uint64, uint64, error) {
	if r == nil || r.inner == nil {
		return 0, 0, nil
	}
	return r.inner.ResolveAssessmentByAnswerSheetID(ctx, answerSheetID)
}
