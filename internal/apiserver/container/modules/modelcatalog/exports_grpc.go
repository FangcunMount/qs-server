package modelcatalog

import grpctransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc"

// GRPCExports groups assessment-model gRPC transport dependencies.
type GRPCExports struct {
	Scale            grpctransport.ScaleDeps
	PersonalityModel grpctransport.PersonalityModelDeps
}

// ExportGRPCDeps exposes assessment-model capabilities to gRPC transport.
func (m *Module) ExportGRPCDeps() GRPCExports {
	exports := GRPCExports{}
	if m == nil {
		return exports
	}
	if m.Scoring != nil {
		exports.Scale = m.Scoring.ExportGRPCDeps()
	}
	if m.Typology != nil {
		exports.PersonalityModel = m.Typology.ExportGRPCDeps()
	}
	return exports
}

// ExportGRPCDeps exposes scoring capabilities to gRPC transport.
func (s *Scoring) ExportGRPCDeps() grpctransport.ScaleDeps {
	deps := grpctransport.ScaleDeps{}
	if s == nil {
		return deps
	}
	deps.QueryService = s.QueryService
	deps.CategoryService = s.CategoryService
	return deps
}

// ExportGRPCDeps exposes personality-model capabilities to gRPC transport.
func (p *Typology) ExportGRPCDeps() grpctransport.PersonalityModelDeps {
	deps := grpctransport.PersonalityModelDeps{}
	if p == nil {
		return deps
	}
	deps.QueryService = p.QueryService
	return deps
}
