// Package grpcbridge 为 collection application 提供 gRPC 客户端类型与端口，隔离 infra 依赖。
//
// 边界约定：
//   - grpcbridge = import 隔离层，将 application 与 infra/grpcclient 解耦；
//   - catalog / evaluation 读路径：在此包内直接将 infra Output 转为 application DTO；
//   - answersheet / testee：application DTO 与 gRPC 形状差异大，由 port/acl 做双向映射。
package grpcbridge

import (
	grpcclient "github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
)

type (
	AnswerInput                      = grpcclient.AnswerInput
	AnswerSheetOutput                = grpcclient.AnswerSheetOutput
	AssessmentDetailOutput         = grpcclient.AssessmentDetailOutput
	AssessmentReportOutput         = grpcclient.AssessmentReportOutput
	AssessmentSummaryOutput        = grpcclient.AssessmentSummaryOutput
	DimensionInterpretOutput         = grpcclient.DimensionInterpretOutput
	CreateTesteeRequest              = grpcclient.CreateTesteeRequest
	FactorOutput                     = grpcclient.FactorOutput
	FactorScoreOutput                = grpcclient.FactorScoreOutput
	ListAssessmentsOutput          = grpcclient.ListAssessmentsOutput
	ListHotScalesOutput              = grpcclient.ListHotScalesOutput
	ListPersonalityModelsOutput      = grpcclient.ListPersonalityModelsOutput
	ListQuestionnairesOutput         = grpcclient.ListQuestionnairesOutput
	ListScalesOutput                 = grpcclient.ListScalesOutput
	ModelExtraOutput                 = grpcclient.ModelExtraOutput
	ModelIdentityOutput              = grpcclient.ModelIdentityOutput
	PersonalityModelCategoriesOutput = grpcclient.PersonalityModelCategoriesOutput
	PersonalityModelOutput           = grpcclient.PersonalityModelOutput
	PersonalityModelSummaryOutput    = grpcclient.PersonalityModelSummaryOutput
	QuestionOutput                   = grpcclient.QuestionOutput
	QuestionnaireOutput              = grpcclient.QuestionnaireOutput
	ResultLevelOutput                = grpcclient.ResultLevelOutput
	SaveAnswerSheetInput             = grpcclient.SaveAnswerSheetInput
	SaveAnswerSheetOutput            = grpcclient.SaveAnswerSheetOutput
	ScaleCategoriesOutput            = grpcclient.ScaleCategoriesOutput
	ScaleOutput                      = grpcclient.ScaleOutput
	ScaleSummaryOutput               = grpcclient.ScaleSummaryOutput
	HotScaleSummaryOutput            = grpcclient.HotScaleSummaryOutput
	ScoreValueOutput                 = grpcclient.ScoreValueOutput
	SuggestionOutput                 = grpcclient.SuggestionOutput
	TesteeResponse                   = grpcclient.TesteeResponse
	TesteeCareContextResponse        = grpcclient.TesteeCareContextResponse
	TrendPointOutput                 = grpcclient.TrendPointOutput
	UpdateTesteeRequest              = grpcclient.UpdateTesteeRequest
)
