// Package grpcbridge 为 collection application 提供 gRPC 客户端类型与端口，隔离 infra 依赖。
//
// 定位说明：
//   - bridge = import 隔离层，将 application 与 infra/grpcclient 解耦；
//   - bridge ≠ ACL：默认仍暴露 grpcclient 别名类型；
//   - catalog / evaluation / answersheet / testee 的 ACL 适配在 port/grpcbridge（catalog）
//     或 port/acl（answersheet/testee）中，将 infra Output 转为 application DTO。
//
// ACL 试点范围（已完成）：
//   - catalog 三域：questionnaire / scale / personalitymodel
//   - BFF 读路径：evaluation / answersheet / testee
package grpcbridge

import (
	grpcclient "github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
)

type (
	AnswerInput                      = grpcclient.AnswerInput
	AnswerSheetOutput                = grpcclient.AnswerSheetOutput
	AssessmentDetailOutput           = grpcclient.AssessmentDetailOutput
	AssessmentDetailV2Output         = grpcclient.AssessmentDetailV2Output
	AssessmentReportOutput           = grpcclient.AssessmentReportOutput
	AssessmentReportV2Output         = grpcclient.AssessmentReportV2Output
	AssessmentSummaryOutput          = grpcclient.AssessmentSummaryOutput
	AssessmentSummaryV2Output        = grpcclient.AssessmentSummaryV2Output
	CreateTesteeRequest              = grpcclient.CreateTesteeRequest
	FactorOutput                     = grpcclient.FactorOutput
	FactorScoreOutput                = grpcclient.FactorScoreOutput
	ListAssessmentsV2Output          = grpcclient.ListAssessmentsV2Output
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
