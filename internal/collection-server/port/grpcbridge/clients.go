package grpcbridge

import "context"

// QuestionnaireReader 问卷目录读端口。
type QuestionnaireReader interface {
	GetQuestionnaire(ctx context.Context, code, version string) (*QuestionnaireOutput, error)
	ListQuestionnaires(ctx context.Context, page, pageSize int32, status, title string) (*ListQuestionnairesOutput, error)
}

// EvaluationReader 测评读端口（collection BFF 使用的方法集合）。
type EvaluationReader interface {
	GetAssessmentScores(ctx context.Context, testeeID, assessmentID uint64) ([]FactorScoreOutput, error)
	GetFactorTrend(ctx context.Context, testeeID uint64, factorCode string, limit int32) ([]TrendPointOutput, error)
	GetHighRiskFactors(ctx context.Context, testeeID, assessmentID uint64) ([]FactorScoreOutput, error)
	GetMyAssessment(ctx context.Context, testeeID, assessmentID uint64) (*AssessmentDetailOutput, error)
	ListMyAssessments(ctx context.Context, testeeID uint64, status, scaleCode, riskLevel, dateFrom, dateTo, modelKind string, page, pageSize int32) (*ListAssessmentsOutput, error)
	ListMyAssessmentsByModelKinds(ctx context.Context, testeeID uint64, status string, modelKinds []string, page, pageSize int32) (*ListAssessmentsOutput, error)
}
type ParticipantReportReader interface {
	GetAssessmentReport(context.Context, uint64, uint64) (*AssessmentReportOutput, error)
}
type AssessmentIntakeReader interface {
	ResolveAssessmentByAnswerSheetID(context.Context, uint64) (testeeID, assessmentID uint64, readinessPhase string, err error)
}

// ActorReader 受试者读端口。
type ActorReader interface {
	GetTestee(ctx context.Context, testeeID uint64) (*TesteeResponse, error)
	TesteeExists(ctx context.Context, orgID, iamProfileID uint64) (exists bool, testeeID uint64, err error)
}

// ActorWriter 受试者写端口。
type ActorWriter interface {
	ActorReader
	CreateTestee(ctx context.Context, req *CreateTesteeRequest) (*TesteeResponse, error)
	GetTesteeCareContext(ctx context.Context, testeeID uint64) (*TesteeCareContextResponse, error)
	UpdateTestee(ctx context.Context, req *UpdateTesteeRequest) (*TesteeResponse, error)
	ListTesteesByUser(ctx context.Context, profileIDs []uint64, offset, limit int32) ([]*TesteeResponse, int64, error)
}

// AnswerSheetWriter 答卷写端口。
type AnswerSheetWriter interface {
	SaveAnswerSheet(ctx context.Context, input *SaveAnswerSheetInput) (*SaveAnswerSheetOutput, error)
	GetAnswerSheet(ctx context.Context, id uint64) (*AnswerSheetOutput, error)
}
