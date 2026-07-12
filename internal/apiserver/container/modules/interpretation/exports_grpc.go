package interpretation

import grpctransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc"

func (m *Module) ExportGRPCDeps() grpctransport.InterpretationDeps {
	if m == nil {
		return grpctransport.InterpretationDeps{}
	}
	return grpctransport.InterpretationDeps{
		AutomationService:    m.AutomationService(),
		ParticipantService:   m.ParticipantService(),
		ReportStatusReporter: m.ReportStatusReporter,
	}
}
