package grpcclient

import (
	evaluationpb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	internalpb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
	interpretationpb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
)

// ACLAllowedMethods returns the complete least-privilege gRPC surface used by
// worker runtime paths when calling qs-apiserver.
func ACLAllowedMethods() []string {
	return []string{
		evaluationpb.AssessmentIntakeService_EnsureAssessment_FullMethodName,
		evaluationpb.EvaluationWorkerService_ExecuteEvaluation_FullMethodName,
		interpretationpb.InterpretationAutomationService_GenerateReportFromOutcome_FullMethodName,
		internalpb.InternalService_SyncAssessmentAttention_FullMethodName,
		internalpb.InternalService_HandleQuestionnairePublishedPostActions_FullMethodName,
		internalpb.InternalService_HandleScalePublishedPostActions_FullMethodName,
		internalpb.InternalService_SendTaskOpenedMiniProgramNotification_FullMethodName,
	}
}
