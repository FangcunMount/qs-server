package interpretation

import grpctransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc"

func (m *Module) ExportGRPCDeps() grpctransport.InterpretationDeps {
	if m == nil {
		return grpctransport.InterpretationDeps{}
	}
	deps := grpctransport.InterpretationDeps{
		AutomationService:       m.AutomationService(),
		AIExplanationEvaluation: m.aiOnlineEvalRunner,
		ParticipantService:      m.ParticipantService(),
		ReportStatusReporter:    m.ReportStatusReporter,
	}
	if m.aiParticipantEnabled {
		deps.AIExplanationExecutor = m.aiExplanationExecutor
		deps.AIExplanationParticipant = m.aiExplanationService
		deps.AIExplanationSubjectExport = m.aiSubjectExport
	}
	return deps
}
