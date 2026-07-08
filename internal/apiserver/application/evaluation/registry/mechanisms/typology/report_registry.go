package typology

import (
	"fmt"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/typology"
)

type reportBuilderFunc func(evaloutcome.Outcome) (*domainReport.InterpretReport, error)

// ReportAdapterRegistry 解析报告构建器 按 报告适配器 键。
type ReportAdapterRegistry struct {
	adapters map[modeltypology.ReportAdapterKey]reportBuilderFunc
}

// 默认ReportAdapterRegistry 返回内置 类型学 报告适配器。
func DefaultReportAdapterRegistry() ReportAdapterRegistry {
	return NewReportAdapterRegistry()
}

// NewReportAdapterRegistry 返回内置 类型学 报告适配器。
func NewReportAdapterRegistry() ReportAdapterRegistry {
	return ReportAdapterRegistry{
		adapters: map[modeltypology.ReportAdapterKey]reportBuilderFunc{
			modeltypology.ReportAdapterPersonalityType: buildTypologyReportAdapter(modeltypology.ReportAdapterPersonalityType),
			modeltypology.ReportAdapterTraitProfile:    buildTypologyReportAdapter(modeltypology.ReportAdapterTraitProfile),
		},
	}
}

// Len 报告数量 报告构建器 是 已注册。
func (r ReportAdapterRegistry) Len() int {
	return len(r.adapters)
}

// Register 返回注册表副本 使用 额外 或 覆盖 报告构建器。
func (r ReportAdapterRegistry) Register(key modeltypology.ReportAdapterKey, builder reportBuilderFunc) ReportAdapterRegistry {
	next := ReportAdapterRegistry{adapters: make(map[modeltypology.ReportAdapterKey]reportBuilderFunc, len(r.adapters)+1)}
	for k, v := range r.adapters {
		next.adapters[k] = v
	}
	next.adapters[key] = builder
	return next
}

func (r ReportAdapterRegistry) build(
	spec modeltypology.ReportSpec,
	mapping modeltypology.OutcomeMappingSpec,
	decisionKind modelcatalog.DecisionKind,
	outcome evaloutcome.Outcome,
) (*domainReport.InterpretReport, error) {
	adapterKey := spec.ResolvedAdapterKey(mapping, decisionKind)
	return r.buildByAdapter(adapterKey, outcome)
}

func (r ReportAdapterRegistry) buildByAdapter(
	adapterKey modeltypology.ReportAdapterKey,
	outcome evaloutcome.Outcome,
) (*domainReport.InterpretReport, error) {
	if adapterKey == "" {
		return nil, fmt.Errorf("report adapter key is required")
	}
	builder, ok := r.adapters[adapterKey]
	if !ok {
		return nil, fmt.Errorf("unsupported report adapter key: %s", adapterKey)
	}
	return builder(outcome)
}

// build类型学ReportAdapter returns 报告构建器 用于 固定适配器键。
func buildTypologyReportAdapter(adapterKey modeltypology.ReportAdapterKey) reportBuilderFunc {
	return func(outcome evaloutcome.Outcome) (*domainReport.InterpretReport, error) {
		return buildTypologyReport(adapterKey, outcome)
	}
}

func buildTypologyReport(
	adapterKey modeltypology.ReportAdapterKey,
	outcome evaloutcome.Outcome,
) (*domainReport.InterpretReport, error) {
	switch adapterKey {
	case modeltypology.ReportAdapterPersonalityType:
		return buildPersonalityTypeReport(adapterKey, outcome)
	case modeltypology.ReportAdapterTraitProfile:
		return buildTraitProfileReport(adapterKey, outcome)
	default:
		return nil, fmt.Errorf("unsupported report adapter key: %s", adapterKey)
	}
}
