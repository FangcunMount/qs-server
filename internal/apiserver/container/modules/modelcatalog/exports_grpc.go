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
	if m.Scale != nil {
		exports.Scale = m.Scale.ExportGRPCDeps()
	}
	if m.Personality != nil {
		exports.PersonalityModel = m.Personality.ExportGRPCDeps()
	}
	return exports
}

// ExportGRPCDeps exposes scale capabilities to gRPC transport.
func (s *Scale) ExportGRPCDeps() grpctransport.ScaleDeps {
	deps := grpctransport.ScaleDeps{}
	if s == nil {
		return deps
	}
	deps.QueryService = s.QueryService
	deps.CategoryService = s.CategoryService
	return deps
}

// ExportGRPCDeps exposes personality-model capabilities to gRPC transport.
func (p *Personality) ExportGRPCDeps() grpctransport.PersonalityModelDeps {
	deps := grpctransport.PersonalityModelDeps{}
	if p == nil {
		return deps
	}
	deps.QueryService = p.QueryService
	return deps
}
