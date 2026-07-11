package registry

import (
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// MechanismReportBuilderKey 路由报告构建器按执行机制，可选 algorithm/productChannel/audience/reportProfile 细化。
type MechanismReportBuilderKey struct {
	AlgorithmFamily modelcatalog.AlgorithmFamily
	DecisionKind    modelcatalog.DecisionKind
	ReportType      domainReport.ReportType
	TemplateVersion policy.TemplateVersion
	Algorithm       modelcatalog.Algorithm
	ProductChannel  modelcatalog.ProductChannel
	Audience        policy.Audience
	ReportProfile   policy.ReportProfile
}

func (k MechanismReportBuilderKey) String() string {
	base := k.AlgorithmFamily.String() + "/" + string(k.DecisionKind) + "/" + string(k.ReportType) + "/" + k.TemplateVersion.String()
	if k.Algorithm != "" {
		base += "/" + string(k.Algorithm)
	}
	if k.ProductChannel != "" {
		base += "/" + string(k.ProductChannel)
	}
	if k.Audience != "" {
		base += "/" + string(k.Audience)
	}
	if k.ReportProfile != "" {
		base += "/" + string(k.ReportProfile)
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
