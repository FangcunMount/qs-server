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

func (r *EvaluationBFFReader) GetMyAssessmentByAnswerSheetID(ctx context.Context, answerSheetID uint64) (*evaluation.AssessmentDetailResponse, error) {
	if r == nil || r.inner == nil {
		return nil, nil
	}
	out, err := r.inner.GetMyAssessmentByAnswerSheetID(ctx, answerSheetID)
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

func (r *EvaluationBFFReader) GetAssessmentReport(ctx context.Context, assessmentID uint64) (*evaluation.AssessmentReportResponse, error) {
	if r == nil || r.inner == nil {
		return nil, nil
	}
	out, err := r.inner.GetAssessmentReport(ctx, assessmentID)
	if err != nil {
		return nil, err
	}
	return toAssessmentReportResponse(out), nil
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

func (r *EvaluationBFFReader) GetMyAssessmentV2(ctx context.Context, testeeID, assessmentID uint64) (*evaluation.AssessmentDetailV2Response, error) {
	if r == nil || r.inner == nil {
		return nil, nil
	}
	out, err := r.inner.GetMyAssessmentV2(ctx, testeeID, assessmentID)
	if err != nil {
		return nil, err
	}
	return toAssessmentDetailV2Response(out), nil
}

func (r *EvaluationBFFReader) ListMyAssessmentsV2(ctx context.Context, testeeID uint64, status, scaleCode, riskLevel, modelKind, algorithm string, page, pageSize int32) (*evaluation.ListAssessmentsV2Response, error) {
	if r == nil || r.inner == nil {
		return nil, nil
	}
	out, err := r.inner.ListMyAssessmentsV2(ctx, testeeID, status, scaleCode, riskLevel, modelKind, algorithm, page, pageSize)
	if err != nil {
		return nil, err
	}
	return toListAssessmentsV2Response(out), nil
}

func (r *EvaluationBFFReader) GetAssessmentReportV2(ctx context.Context, testeeID, assessmentID uint64) (*evaluation.AssessmentReportV2Response, error) {
	if r == nil || r.inner == nil {
		return nil, nil
	}
	out, err := r.inner.GetAssessmentReportV2(ctx, testeeID, assessmentID)
	if err != nil {
		return nil, err
	}
	return toAssessmentReportV2Response(out), nil
}
