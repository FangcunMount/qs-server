package query

import (
	modelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/capability"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

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

func apiKindOptions() []modelcatalog.Option {
	items := make([]modelcatalog.Option, 0, len(catalogKinds))
	for _, item := range catalogKinds {
		items = append(items, modelcatalog.Option{Label: item.Label, Value: modelcatalog.DomainKindToAPIKind(item.Kind)})
	}
	return items
}

func catalogOptionsForKind(kind string) modelcatalog.OptionsResult {
	result := modelcatalog.OptionsResult{
		Kinds:             apiKindOptions(),
		ProductChannels:   modelcatalog.ProductChannelOptions(),
		AlgorithmFamilies: modelcatalog.AlgorithmFamilyOptions(),
		Algorithms:        algorithmOptions(kind),
		SubKinds:          subKindOptions(kind),
		ScoringStrategies: scoringStrategyOptions(kind),
		Categories:        []modelcatalog.Option{},
	}
	if kind == modelcatalog.KindScale {
		result.Categories = scaleCategoryOptions()
		result.Stages = scaleStageOptions()
		result.ApplicableAges = scaleApplicableAgeOptions()
		result.Reporters = scaleReporterOptions()
		result.Tags = []modelcatalog.Option{}
	}
	return result
}

func scoringStrategyOptions(kind string) []modelcatalog.Option {
	if kind == "" {
		// Union across all paths when kind is omitted.
		seen := make(map[string]struct{})
		out := make([]modelcatalog.Option, 0)
		for _, path := range capability.AllPaths() {
			for _, code := range capability.AuthoringStrategyCodes(path) {
				if _, ok := seen[code]; ok {
					continue
				}
				seen[code] = struct{}{}
				out = append(out, modelcatalog.Option{Label: code, Value: code})
			}
		}
		return out
	}
	domainKind, ok := modelcatalog.APIKindToDomainKind(kind)
	if !ok {
		return []modelcatalog.Option{}
	}
	path, ok := capability.PathForKind(string(domainKind))
	if !ok {
		return []modelcatalog.Option{}
	}
	codes := capability.AuthoringStrategyCodes(path)
	out := make([]modelcatalog.Option, 0, len(codes))
	for _, code := range codes {
		out = append(out, modelcatalog.Option{Label: code, Value: code})
	}
	return out
}

func algorithmOptions(kind string) []modelcatalog.Option {
	all := []modelcatalog.Option{
		{Label: "默认量表", Value: string(domain.AlgorithmScaleDefault)},
		{Label: "统一人格类型运行时", Value: string(domain.AlgorithmPersonalityTypology)},
		{Label: "BRIEF-2", Value: string(domain.AlgorithmBrief2)},
		{Label: "SPM（感觉统合）", Value: string(domain.AlgorithmSPMSensory)},
		{Label: "SPM", Value: string(domain.AlgorithmSPM)},
	}
	switch kind {
	case "", modelcatalog.KindScale, modelcatalog.KindTypology, modelcatalog.KindBehavioralRating, modelcatalog.KindCognitive:
	default:
		return []modelcatalog.Option{}
	}
	if kind == "" {
		return all
	}
	filtered := make([]modelcatalog.Option, 0, 3)
	for _, item := range all {
		switch kind {
		case modelcatalog.KindScale:
			if item.Value == string(domain.AlgorithmScaleDefault) {
				filtered = append(filtered, item)
			}
		case modelcatalog.KindTypology:
			if item.Value == string(domain.AlgorithmPersonalityTypology) {
				filtered = append(filtered, item)
			}
		case modelcatalog.KindBehavioralRating:
			if item.Value == string(domain.AlgorithmBrief2) || item.Value == string(domain.AlgorithmSPMSensory) {
				filtered = append(filtered, item)
			}
		case modelcatalog.KindCognitive:
			if item.Value == string(domain.AlgorithmSPM) {
				filtered = append(filtered, item)
			}
		}
	}
	return filtered
}

func subKindOptions(kind string) []modelcatalog.Option {
	if kind == "" {
		return []modelcatalog.Option{{Label: "量表评分", Value: modelcatalog.SubKindScale}, {Label: "类型人格", Value: modelcatalog.SubKindTypology}}
	}
	switch kind {
	case modelcatalog.KindScale:
		return []modelcatalog.Option{{Label: "量表评分", Value: modelcatalog.SubKindScale}}
	case modelcatalog.KindTypology:
		return []modelcatalog.Option{{Label: "类型人格", Value: modelcatalog.SubKindTypology}}
	default:
		return []modelcatalog.Option{}
	}
}
