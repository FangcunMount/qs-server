package modelcatalog

import grpctransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc"

// GRPCExports 包含模型目录的gRPC传输依赖
type GRPCExports struct {
	AssessmentModelCatalog grpctransport.AssessmentModelCatalogDeps
}

// ExportGRPCDeps 暴露模型目录的gRPC传输能力
func (m *Module) ExportGRPCDeps() GRPCExports {
	exports := GRPCExports{}
	if m == nil {
		return exports
	}
	exports.AssessmentModelCatalog.QueryService = m.Query
	return exports
}
