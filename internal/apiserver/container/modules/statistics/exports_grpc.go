package statistics

import grpctransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc"

// ExportGRPCDeps exposes statistics capabilities to gRPC transport.
func (m *Module) ExportGRPCDeps() grpctransport.StatisticsDeps {
	deps := grpctransport.StatisticsDeps{}
	if m == nil || m.BehaviorProjectorService == nil {
		return deps
	}
	deps.BehaviorProjectorService = m.BehaviorProjectorService
	return deps
}
