package grpcbridge

import (
	"context"

	grpcclient "github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
)

// ScaleReader 量表目录读端口。
type ScaleReader interface {
	GetScale(ctx context.Context, code string) (*ScaleOutput, error)
	ListScales(ctx context.Context, page, pageSize int32, status, title, category string, stages, applicableAges, reporters, tags []string) (*ListScalesOutput, error)
	ListHotScales(ctx context.Context, limit, windowDays int32) (*ListHotScalesOutput, error)
	GetScaleCategories(ctx context.Context) (*ScaleCategoriesOutput, error)
}

// QuestionnaireReader 问卷目录读端口。
type QuestionnaireReader interface {
	GetQuestionnaire(ctx context.Context, code, version string) (*QuestionnaireOutput, error)
	ListQuestionnaires(ctx context.Context, page, pageSize int32, status, title string) (*ListQuestionnairesOutput, error)
}

// PersonalityModelReader 人格模型目录读端口。
type PersonalityModelReader interface {
	GetPersonalityModel(ctx context.Context, code string) (*PersonalityModelOutput, error)
	ListPersonalityModels(ctx context.Context, page, pageSize int32, algorithm string) (*ListPersonalityModelsOutput, error)
	GetPersonalityModelCategories(ctx context.Context) (*PersonalityModelCategoriesOutput, error)
}

// EvaluationReader 测评读端口（collection BFF 使用的方法集合）。
type EvaluationReader interface {
	GetMyAssessment(ctx context.Context, testeeID, assessmentID uint64) (*AssessmentDetailOutput, error)
	GetMyAssessmentByAnswerSheetID(ctx context.Context, answerSheetID uint64) (*AssessmentDetailOutput, error)
	ListMyAssessments(ctx context.Context, testeeID uint64, status, scaleCode, riskLevel, dateFrom, dateTo, modelKind string, page, pageSize int32) (*ListAssessmentsOutput, error)
	GetAssessmentScores(ctx context.Context, testeeID, assessmentID uint64) ([]FactorScoreOutput, error)
	GetAssessmentReport(ctx context.Context, assessmentID uint64) (*AssessmentReportOutput, error)
	GetFactorTrend(ctx context.Context, testeeID uint64, factorCode string, limit int32) ([]TrendPointOutput, error)
	GetHighRiskFactors(ctx context.Context, testeeID, assessmentID uint64) ([]FactorScoreOutput, error)
	GetMyAssessmentV2(ctx context.Context, testeeID, assessmentID uint64) (*AssessmentDetailV2Output, error)
	ListMyAssessmentsV2(ctx context.Context, testeeID uint64, status, scaleCode, riskLevel, modelKind, algorithm string, page, pageSize int32) (*ListAssessmentsV2Output, error)
	GetAssessmentReportV2(ctx context.Context, testeeID, assessmentID uint64) (*AssessmentReportV2Output, error)
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

type ListAssessmentsOutput = grpcclient.ListAssessmentsOutput
