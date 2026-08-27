package interpretation

import grpctransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc"

func (m *Module) ExportGRPCDeps() grpctransport.InterpretationDeps {
	if m == nil {
		return grpctransport.InterpretationDeps{}
	}
	return grpctransport.InterpretationDeps{
		AutomationService:          m.AutomationService(),
		AIExplanationExecutor:      m.aiExplanationExecutor,
		AIExplanationEvaluation:    m.aiOnlineEvalRunner,
		AIExplanationParticipant:   m.aiExplanationService,
		AIExplanationSubjectExport: m.aiSubjectExport,
		ParticipantService:         m.ParticipantService(),
		ReportStatusReporter:       m.ReportStatusReporter,
	}
}
