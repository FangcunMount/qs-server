package registry

import (
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// MechanismReportBuilderKey 路由报告构建器按执行机制，可选 algorithm/productChannel 细化。
type MechanismReportBuilderKey struct {
	AlgorithmFamily modelcatalog.AlgorithmFamily
	DecisionKind    modelcatalog.DecisionKind
	ReportType      domainReport.ReportType
	Algorithm       modelcatalog.Algorithm
	ProductChannel  modelcatalog.ProductChannel
}

func (k MechanismReportBuilderKey) String() string {
	base := k.AlgorithmFamily.String() + "/" + string(k.DecisionKind) + "/" + string(k.ReportType)
	if k.Algorithm != "" {
		base += "/" + string(k.Algorithm)
	}
	if k.ProductChannel != "" {
		base += "/" + string(k.ProductChannel)
	}
	return base
}

// MechanismKeyedReportBuilder 暴露机制 路由 元数据 用于 报告构建器。
// MechanismKey 是主 路由 键; 键 保持 用于 旧版 表征。
type MechanismKeyedReportBuilder interface {
	ReportBuilder
	MechanismKey() MechanismReportBuilderKey
}

// MultiMechanismKeyedReportBuilder registers 额外 decision-granularity 机制键。
type MultiMechanismKeyedReportBuilder interface {
	MechanismKeyedReportBuilder
	MechanismKeys() []MechanismReportBuilderKey
}
