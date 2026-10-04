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
	// Committed results remain owned by the MQ consumer while intake is closed.
	// Retired gRPC result writes are never re-exported by a configuration change.
	deps.AIMessagePayloads = m.aiMessagingReader
	return deps
}

func (m *Module) BindCurrentAIAccess(access *bridge.CurrentAccess) { m.aiCurrentAccess = access }
