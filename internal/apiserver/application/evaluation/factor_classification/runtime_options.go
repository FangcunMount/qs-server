package factor_classification

import (
	personalityconfigured "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/configured"
)

// PersonalityRuntimeOptions configures injectable adapter registries for typology execution.
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
