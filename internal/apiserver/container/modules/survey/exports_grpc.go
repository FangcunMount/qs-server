package survey

import grpctransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc"

// ExportGRPCDeps exposes survey capabilities to gRPC transport.
func (m *Module) ExportGRPCDeps() grpctransport.SurveyDeps {
	deps := grpctransport.SurveyDeps{}
	if m == nil {
		return deps
	}
	if m.AnswerSheet != nil {
		deps.AnswerSheetSubmissionService = m.AnswerSheet.SubmissionService
		deps.AnswerSheetManagementService = m.AnswerSheet.ManagementService
		deps.AnswerSheetScoringService = m.AnswerSheet.ScoringService
	}
	if m.Questionnaire != nil {
		deps.QuestionnaireQueryService = m.Questionnaire.QueryService
	}
	return deps
}
