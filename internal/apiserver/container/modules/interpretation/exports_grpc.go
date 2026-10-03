package interpretation

import (
	bridge "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"

	grpctransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc"
)

func (m *Module) ExportGRPCDeps() grpctransport.InterpretationDeps {
	if m == nil {
		return grpctransport.InterpretationDeps{}
	}
	deps := grpctransport.InterpretationDeps{
		AutomationService:    m.AutomationService(),
		ParticipantService:   m.ParticipantService(),
		ParticipantRuntime:   m.participantRuntime,
		ReportStatusReporter: m.ReportStatusReporter,
	}
	deps.CurrentAccess = m.aiCurrentAccess
	deps.AIWorkflow = m.aiWorkflow
	// Continue accepting already committed AI results even while new intake is closed.
	if m.aiMessagingRuntime == nil {
		deps.AIWorkflowResults = m.aiBridge
	}
	deps.AIMessagePayloads = m.aiMessagingReader
	return deps
}

func (m *Module) BindCurrentAIAccess(access *bridge.CurrentAccess) { m.aiCurrentAccess = access }
