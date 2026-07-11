package modelcatalog

import domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"

// PopulateModelSummaryIdentity adds API identity fields to a catalog summary.
func PopulateModelSummaryIdentity(summary *ModelSummary, kind domain.Kind, subKind domain.SubKind, algorithm domain.Algorithm, productChannel domain.ProductChannel) {
	if summary == nil {
		return
	}
	summary.ProductChannel = string(domain.ResolveProductChannel(kind, productChannel))
	if family, ok := domain.AlgorithmFamilyFromIdentity(kind, subKind, algorithm); ok {
		summary.AlgorithmFamily = string(family)
	}
}

// ProductChannelOptions returns the canonical product-channel catalog options.
func ProductChannelOptions() []Option {
	channels := domain.AllProductChannels()
	options := make([]Option, 0, len(channels))
	labels := map[domain.ProductChannel]string{
		domain.ProductChannelMedicalScale:    "医学量表",
		domain.ProductChannelTypology:        "人格测评",
		domain.ProductChannelBehaviorAbility: "行为能力",
	}
	for _, channel := range channels {
		label := labels[channel]
		if label == "" {
			label = string(channel)
		}
		options = append(options, Option{Label: label, Value: string(channel)})
	}
	return options
}

// AlgorithmFamilyOptions returns the canonical algorithm-family catalog options.
func AlgorithmFamilyOptions() []Option {
	families := domain.AllAlgorithmFamilies()
	options := make([]Option, 0, len(families))
	labels := map[domain.AlgorithmFamily]string{
		domain.AlgorithmFamilyFactorScoring:        "因子记分",
		domain.AlgorithmFamilyFactorClassification: "因子分类",
		domain.AlgorithmFamilyFactorNorm:           "因子+常模",
		domain.AlgorithmFamilyTaskPerformance:      "任务表现",
	}
	for _, family := range families {
		label := labels[family]
		if label == "" {
			label = string(family)
		}
		options = append(options, Option{Label: label, Value: string(family)})
	}
	return options
}
