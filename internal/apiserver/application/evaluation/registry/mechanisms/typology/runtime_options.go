package typology

import (
	personalityconfigured "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/typology/configured"
)

// PersonalityRuntimeOptions 配置injectable adapter 注册表 用于 类型学 execution。
type PersonalityRuntimeOptions struct {
	DetailRegistry  personalityconfigured.DetailAssemblerRegistry
	OutcomeRegistry OutcomeAdapterRegistry
	ReportRegistry  ReportAdapterRegistry
}

func resolvePersonalityRuntimeOptions(opts PersonalityRuntimeOptions) PersonalityRuntimeOptions {
	if opts.DetailRegistry.Len() == 0 {
		opts.DetailRegistry = personalityconfigured.DefaultDetailAssemblerRegistry()
	}
	if opts.OutcomeRegistry.Len() == 0 {
		opts.OutcomeRegistry = DefaultOutcomeAdapterRegistry()
	}
	if opts.ReportRegistry.Len() == 0 {
		opts.ReportRegistry = DefaultReportAdapterRegistry()
	}
	return opts
}
