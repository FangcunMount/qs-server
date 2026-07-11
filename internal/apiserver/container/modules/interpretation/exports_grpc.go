package interpretation

import grpctransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc"

func (m *Module) ExportGRPCDeps() grpctransport.InterpretationDeps {
	if m == nil {
		return grpctransport.InterpretationDeps{}
	}
	return grpctransport.InterpretationDeps{
		OutcomeReportService: m.OutcomeService(),
		ReportQueryService:   m.QueryService,
	}
}
