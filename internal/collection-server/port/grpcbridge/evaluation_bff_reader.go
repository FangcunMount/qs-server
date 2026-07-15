package grpcbridge

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
)

// EvaluationBFFReader 将 infra gRPC 输出转换为 evaluation application DTO。
type EvaluationBFFReader struct {
	evaluation EvaluationReader
	reports    ParticipantReportReader
	intake     AssessmentIntakeReader
}

// NewEvaluationBFFReader 构造测评 BFF ACL 适配器。
func NewEvaluationBFFReader(evaluation EvaluationReader, reports ParticipantReportReader, intake AssessmentIntakeReader) *EvaluationBFFReader {
	return &EvaluationBFFReader{evaluation: evaluation, reports: reports, intake: intake}
}

func (r *EvaluationBFFReader) GetAssessmentScores(ctx context.Context, testeeID, assessmentID uint64) ([]evaluation.FactorScoreResponse, error) {
	if r == nil || r.evaluation == nil {
		return nil, nil
	}
	out, err := r.evaluation.GetAssessmentScores(ctx, testeeID, assessmentID)
	if err != nil {
		return nil, err
	}
	return toFactorScoreResponses(out), nil
}

func (r *EvaluationBFFReader) GetFactorTrend(ctx context.Context, testeeID uint64, factorCode string, limit int32) ([]evaluation.TrendPointResponse, error) {
	if r == nil || r.evaluation == nil {
		return nil, nil
	}
	out, err := r.evaluation.GetFactorTrend(ctx, testeeID, factorCode, limit)
	if err != nil {
		return nil, err
	}
	return toTrendPointResponses(out), nil
}

func (r *EvaluationBFFReader) GetHighRiskFactors(ctx context.Context, testeeID, assessmentID uint64) ([]evaluation.FactorScoreResponse, error) {
	if r == nil || r.evaluation == nil {
		return nil, nil
	}
	out, err := r.evaluation.GetHighRiskFactors(ctx, testeeID, assessmentID)
	if err != nil {
		return nil, err
	}
	return toFactorScoreResponses(out), nil
}

func (r *EvaluationBFFReader) GetMyAssessment(ctx context.Context, testeeID, assessmentID uint64) (*evaluation.AssessmentDetailResponse, error) {
	if r == nil || r.evaluation == nil {
		return nil, nil
	}
	out, err := r.evaluation.GetMyAssessment(ctx, testeeID, assessmentID)
	if err != nil {
		return nil, err
	}
	return toAssessmentDetailResponse(out), nil
}

func (r *EvaluationBFFReader) ListMyAssessments(ctx context.Context, testeeID uint64, status, scaleCode, riskLevel, dateFrom, dateTo, modelKind string, page, pageSize int32) (*evaluation.ListAssessmentsResponse, error) {
	if r == nil || r.evaluation == nil {
		return nil, nil
	}
	out, err := r.evaluation.ListMyAssessments(ctx, testeeID, status, scaleCode, riskLevel, dateFrom, dateTo, modelKind, page, pageSize)
	if err != nil {
		return nil, err
	}
	return toListAssessmentsResponse(out), nil
}

func (r *EvaluationBFFReader) ListMyAssessmentsByModelKinds(ctx context.Context, testeeID uint64, status string, modelKinds []string, page, pageSize int32) (*evaluation.ListAssessmentsResponse, error) {
	if r == nil || r.evaluation == nil {
		return nil, nil
	}
	out, err := r.evaluation.ListMyAssessmentsByModelKinds(ctx, testeeID, status, modelKinds, page, pageSize)
	if err != nil {
		return nil, err
	}
	return toListAssessmentsResponse(out), nil
}

func (r *EvaluationBFFReader) GetAssessmentReport(ctx context.Context, testeeID, assessmentID uint64) (*evaluation.AssessmentReportResponse, error) {
	if r == nil || r.reports == nil {
		return nil, nil
	}
	out, err := r.reports.GetAssessmentReport(ctx, testeeID, assessmentID)
	if err != nil {
		return nil, err
	}
	return toAssessmentReportResponse(out), nil
}

func (r *EvaluationBFFReader) ResolveAssessmentByAnswerSheetID(ctx context.Context, answerSheetID uint64) (uint64, uint64, error) {
	if r == nil || r.intake == nil {
		return 0, 0, nil
	}
	return r.intake.ResolveAssessmentByAnswerSheetID(ctx, answerSheetID)
}
