package modelcatalog

import domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"

func populateModelSummaryIdentity(summary *ModelSummary, kind domain.Kind, subKind domain.SubKind, algorithm domain.Algorithm, productChannel domain.ProductChannel) {
	if summary == nil {
		return
	}
	summary.ProductChannel = string(domain.ResolveProductChannel(kind, productChannel))
	if family, ok := domain.AlgorithmFamilyFromIdentity(kind, subKind, algorithm); ok {
		summary.AlgorithmFamily = string(family)
	}
}

func populateDefinitionIdentity(dto *DefinitionDTO, kind domain.Kind, subKind domain.SubKind, algorithm domain.Algorithm, productChannel domain.ProductChannel) {
	if dto == nil {
		return
	}
	dto.ProductChannel = string(domain.ResolveProductChannel(kind, productChannel))
	if family, ok := domain.AlgorithmFamilyFromIdentity(kind, subKind, algorithm); ok {
		dto.AlgorithmFamily = string(family)
	}
}

func productChannelOptions() []Option {
	channels := domain.AllProductChannels()
	options := make([]Option, 0, len(channels))
	labels := map[domain.ProductChannel]string{
		domain.ProductChannelMedicalScale:    "医学量表",
		domain.ProductChannelPersonality:     "人格探索",
		domain.ProductChannelBehaviorAbility: "行为能力",
		domain.ProductChannelCognitive:       "认知能力",
		domain.ProductChannelCustom:          "自定义",
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

func algorithmFamilyOptions() []Option {
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
