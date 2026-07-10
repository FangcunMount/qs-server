package modelcatalog

import domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"

type catalogKindOption struct {
	Kind  domain.Kind
	Label string
}

var catalogKinds = []catalogKindOption{
	{Kind: domain.KindScale, Label: "医学量表"},
	{Kind: domain.KindTypology, Label: "人格测评"},
	{Kind: domain.KindBehavioralRating, Label: "行为评分"},
	{Kind: domain.KindCognitive, Label: "认知测评"},
}

func apiKindOptions() []Option {
	items := make([]Option, 0, len(catalogKinds))
	for _, item := range catalogKinds {
		items = append(items, Option{Label: item.Label, Value: DomainKindToAPIKind(item.Kind)})
	}
	return items
}

func catalogOptionsForKind(kind string) OptionsResult {
	result := OptionsResult{
		Kinds:             apiKindOptions(),
		ProductChannels:   productChannelOptions(),
		AlgorithmFamilies: algorithmFamilyOptions(),
		Algorithms:        algorithmOptions(kind),
		SubKinds:          subKindOptions(kind),
		Categories:        []Option{},
	}
	if kind == KindScale {
		result.Categories = scaleCategoryOptions()
		result.Stages = scaleStageOptions()
		result.ApplicableAges = scaleApplicableAgeOptions()
		result.Reporters = scaleReporterOptions()
		result.Tags = []Option{}
	}
	return result
}

func algorithmOptions(kind string) []Option {
	all := []Option{
		{Label: "默认量表", Value: string(domain.AlgorithmScaleDefault)},
		{Label: "MBTI", Value: string(domain.AlgorithmMBTI)},
		{Label: "SBTI", Value: string(domain.AlgorithmSBTI)},
		{Label: "Big Five", Value: string(domain.AlgorithmBigFive)},
		{Label: "BRIEF-2", Value: string(domain.AlgorithmBrief2)},
		{Label: "SPM", Value: string(domain.AlgorithmSPM)},
	}
	switch kind {
	case "", KindScale, KindTypology, KindBehavioralRating, KindCognitive:
	default:
		return []Option{}
	}
	if kind == "" {
		return all
	}
	filtered := make([]Option, 0, 3)
	for _, item := range all {
		switch kind {
		case KindScale:
			if item.Value == string(domain.AlgorithmScaleDefault) {
				filtered = append(filtered, item)
			}
		case KindTypology:
			if item.Value == string(domain.AlgorithmMBTI) || item.Value == string(domain.AlgorithmSBTI) || item.Value == string(domain.AlgorithmBigFive) {
				filtered = append(filtered, item)
			}
		case KindBehavioralRating:
			if item.Value == string(domain.AlgorithmBrief2) {
				filtered = append(filtered, item)
			}
		case KindCognitive:
			if item.Value == string(domain.AlgorithmSPM) {
				filtered = append(filtered, item)
			}
		}
	}
	return filtered
}

func subKindOptions(kind string) []Option {
	if kind == "" {
		return []Option{{Label: "量表评分", Value: SubKindScale}, {Label: "类型人格", Value: SubKindTypology}}
	}
	switch kind {
	case KindScale:
		return []Option{{Label: "量表评分", Value: SubKindScale}}
	case KindTypology:
		return []Option{{Label: "类型人格", Value: SubKindTypology}}
	default:
		return []Option{}
	}
}
