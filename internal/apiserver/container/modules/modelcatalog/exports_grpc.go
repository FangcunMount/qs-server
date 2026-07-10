package modelcatalog

import grpctransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc"

// GRPCExports groups assessment-model gRPC transport dependencies.
type GRPCExports struct {
	AssessmentModelCatalog grpctransport.AssessmentModelCatalogDeps
	TypologyModel          grpctransport.TypologyModelDeps
}

// ExportGRPCDeps exposes assessment-model capabilities to gRPC transport.
func (m *Module) ExportGRPCDeps() GRPCExports {
	exports := GRPCExports{}
	if m == nil {
		return exports
	}
	exports.AssessmentModelCatalog.QueryService = m.Query
	if m.Typology != nil {
		exports.TypologyModel = m.Typology.ExportGRPCDeps()
	}
	return exports
}

// ExportGRPCDeps exposes typology-model capabilities to gRPC transport.
func (p *Typology) ExportGRPCDeps() grpctransport.TypologyModelDeps {
	deps := grpctransport.TypologyModelDeps{}
	if p == nil {
		return deps
	}
	deps.QueryService = p.QueryService
	return deps
}
