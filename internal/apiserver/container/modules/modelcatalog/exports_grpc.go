package modelcatalog

import grpctransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc"

// GRPCExports groups assessment-model gRPC transport dependencies.
type GRPCExports struct {
	AssessmentModelCatalog grpctransport.AssessmentModelCatalogDeps
}

// ExportGRPCDeps exposes assessment-model capabilities to gRPC transport.
func (m *Module) ExportGRPCDeps() GRPCExports {
	exports := GRPCExports{}
	if m == nil {
		return exports
	}
	exports.AssessmentModelCatalog.QueryService = m.Query
	return exports
}
