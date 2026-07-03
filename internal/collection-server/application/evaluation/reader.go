package evaluation

import "context"

// BFFReader 测评 BFF 读端口（application-owned DTO）。
type BFFReader interface {
	GetMyAssessment(ctx context.Context, testeeID, assessmentID uint64) (*AssessmentDetailResponse, error)
	GetMyAssessmentByAnswerSheetID(ctx context.Context, answerSheetID uint64) (*AssessmentDetailResponse, error)
	ListMyAssessments(ctx context.Context, testeeID uint64, status, scaleCode, riskLevel, dateFrom, dateTo, modelKind string, page, pageSize int32) (*ListAssessmentsResponse, error)
	GetAssessmentScores(ctx context.Context, testeeID, assessmentID uint64) ([]FactorScoreResponse, error)
	GetAssessmentReport(ctx context.Context, assessmentID uint64) (*AssessmentReportResponse, error)
	GetFactorTrend(ctx context.Context, testeeID uint64, factorCode string, limit int32) ([]TrendPointResponse, error)
	GetHighRiskFactors(ctx context.Context, testeeID, assessmentID uint64) ([]FactorScoreResponse, error)
	GetMyAssessmentV2(ctx context.Context, testeeID, assessmentID uint64) (*AssessmentDetailV2Response, error)
	ListMyAssessmentsV2(ctx context.Context, testeeID uint64, status, scaleCode, riskLevel, modelKind, algorithm string, page, pageSize int32) (*ListAssessmentsV2Response, error)
	GetAssessmentReportV2(ctx context.Context, testeeID, assessmentID uint64) (*AssessmentReportV2Response, error)
}
