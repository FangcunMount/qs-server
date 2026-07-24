package grpcclient

import (
	actorpb "github.com/FangcunMount/qs-server/api/grpc/gen/actor"
	answersheetpb "github.com/FangcunMount/qs-server/api/grpc/gen/answersheet"
	assessmentmodelpb "github.com/FangcunMount/qs-server/api/grpc/gen/assessmentmodel"
	evaluationpb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	interpretationpb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	questionnairepb "github.com/FangcunMount/qs-server/api/grpc/gen/questionnaire"
)

// ACLAllowedMethods returns the complete least-privilege gRPC surface used by
// collection-server when calling qs-apiserver.
func ACLAllowedMethods() []string {
	return []string{
		answersheetpb.AnswerSheetService_SaveAnswerSheet_FullMethodName,
		answersheetpb.AnswerSheetService_LookupAnswerSheetSubmission_FullMethodName,
		answersheetpb.AnswerSheetService_GetAnswerSheet_FullMethodName,
		answersheetpb.AnswerSheetService_ListAnswerSheets_FullMethodName,

		questionnairepb.QuestionnaireService_GetQuestionnaire_FullMethodName,
		questionnairepb.QuestionnaireService_ListQuestionnaires_FullMethodName,

		evaluationpb.TesteeEvaluationService_GetMyAssessment_FullMethodName,
		evaluationpb.TesteeEvaluationService_ListMyAssessments_FullMethodName,
		evaluationpb.TesteeEvaluationService_GetAssessmentScores_FullMethodName,
		evaluationpb.TesteeEvaluationService_GetFactorTrend_FullMethodName,
		evaluationpb.TesteeEvaluationService_GetHighRiskFactors_FullMethodName,
		evaluationpb.AssessmentIntakeService_ResolveAssessmentByAnswerSheetID_FullMethodName,

		interpretationpb.ParticipantReportService_GetAssessmentReport_FullMethodName,

		actorpb.ActorService_CreateTestee_FullMethodName,
		actorpb.ActorService_GetTestee_FullMethodName,
		actorpb.ActorService_UpdateTestee_FullMethodName,
		actorpb.ActorService_TesteeExists_FullMethodName,
		actorpb.ActorService_ListTesteesByOrg_FullMethodName,
		actorpb.ActorService_ListTesteesByUser_FullMethodName,
		actorpb.ActorService_GetTesteeCareContext_FullMethodName,

		assessmentmodelpb.AssessmentModelCatalogService_GetPublishedModel_FullMethodName,
		assessmentmodelpb.AssessmentModelCatalogService_ListPublishedModels_FullMethodName,
		assessmentmodelpb.AssessmentModelCatalogService_ListHotPublishedModels_FullMethodName,
		assessmentmodelpb.AssessmentModelCatalogService_GetCatalogOptions_FullMethodName,
	}
}
