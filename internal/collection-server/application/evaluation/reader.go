package evaluation

import "context"

// BFFReader 测评 BFF 读端口（application-owned DTO）。
type BFFReader interface {
	GetAssessmentScores(ctx context.Context, testeeID, assessmentID uint64) ([]FactorScoreResponse, error)
	GetFactorTrend(ctx context.Context, testeeID uint64, factorCode string, limit int32) ([]TrendPointResponse, error)
	GetHighRiskFactors(ctx context.Context, testeeID, assessmentID uint64) ([]FactorScoreResponse, error)
	GetMyAssessment(ctx context.Context, testeeID, assessmentID uint64) (*AssessmentDetailResponse, error)
	ListMyAssessments(ctx context.Context, testeeID uint64, status, scaleCode, riskLevel, dateFrom, dateTo, modelKind, algorithm string, page, pageSize int32) (*ListAssessmentsResponse, error)
	GetAssessmentReport(ctx context.Context, testeeID, assessmentID uint64) (*AssessmentReportResponse, error)
	ResolveAssessmentByAnswerSheetID(ctx context.Context, answerSheetID uint64) (testeeID, assessmentID uint64, err error)
}
