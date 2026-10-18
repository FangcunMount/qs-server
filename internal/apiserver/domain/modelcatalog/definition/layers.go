package definition

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
)

// InterpretationAssetsFrom projects Definition Outcomes/Profiles/ReportMap into
// InterpretationAssets (MC-R016). Decision bounds are not included.
func InterpretationAssetsFrom(def *Definition) interpretationassets.Assets {
	if def == nil {
		return interpretationassets.Assets{}
	}
	assets := interpretationassets.Assets{
		Outcomes: make([]interpretationassets.OutcomePresentation, 0, len(def.Outcomes)),
	}
	seen := make(map[string]struct{})
	for _, outcome := range def.Outcomes {
		if outcome.Code == "" {
			continue
		}
		assets.Outcomes = append(assets.Outcomes, interpretationassets.OutcomePresentation{
			OutcomeCode: outcome.Code,
			Title:       outcome.Title,
			Summary:     outcome.Summary,
			Description: outcome.Description,
		})
		seen[outcome.Code] = struct{}{}
	}
	// Score-range inline copy is still authored on rules; project as presentation
	// keyed by OutcomeCode when registry entry is missing.
	for _, item := range def.Conclusions {
		switch c := item.(type) {
		case conclusion.RiskConclusion:
			assets.Outcomes = append(assets.Outcomes, scoreRangePresentations(c.Rules, seen)...)
		case conclusion.NormConclusion:
			assets.Outcomes = append(assets.Outcomes, scoreRangePresentations(c.Rules, seen)...)
		case conclusion.AbilityConclusion:
			assets.Outcomes = append(assets.Outcomes, scoreRangePresentations(c.Rules, seen)...)
		case conclusion.TypeConclusion:
			for _, profile := range c.Profiles {
				assets.Profiles = append(assets.Profiles, interpretationassets.TypeProfilePresentation{
					OutcomeCode: profile.OutcomeCode,
					Pattern:     profile.Pattern,
					Traits:      append([]string(nil), profile.Traits...),
					Strengths:   append([]string(nil), profile.Strengths...),
					Weaknesses:  append([]string(nil), profile.Weaknesses...),
					Suggestions: append([]string(nil), profile.Suggestions...),
					ImageURL:    profile.ImageURL,
					Image:       profile.Image,
					Rarity: interpretationassets.RarityPresentation{
						Percent: profile.Rarity.Percent,
						Label:   profile.Rarity.Label,
						OneInX:  profile.Rarity.OneInX,
					},
					IsSpecial:  profile.IsSpecial,
					Trigger:    profile.Trigger,
					Commentary: profile.Commentary,
				})
			}
			for _, outcome := range c.Outcomes {
				if outcome.Code == "" {
					continue
				}
				if _, ok := seen[outcome.Code]; ok {
					continue
				}
				assets.Outcomes = append(assets.Outcomes, interpretationassets.OutcomePresentation{
					OutcomeCode: outcome.Code,
					Title:       outcome.Title,
					Summary:     outcome.Summary,
					Description: outcome.Description,
				})
				seen[outcome.Code] = struct{}{}
			}
		}
	}
	assets.ReportSpec = reportSpecFrom(def.ReportMap)
	return assets
}

func scoreRangePresentations(rules []conclusion.ScoreRangeOutcome, seen map[string]struct{}) []interpretationassets.OutcomePresentation {
	out := make([]interpretationassets.OutcomePresentation, 0)
	for _, rule := range rules {
		code := rule.OutcomeCode
		if code == "" {
			code = rule.Level
		}
		if code == "" {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		if rule.Title == "" && rule.Summary == "" && rule.Description == "" {
			continue
		}
		out = append(out, interpretationassets.OutcomePresentation{
			OutcomeCode: code,
			Title:       rule.Title,
			Summary:     rule.Summary,
			Description: rule.Description,
		})
		seen[code] = struct{}{}
	}
	return out
}

func reportSpecFrom(reportMap ReportMap) interpretationassets.ReportSpec {
	if len(reportMap.Sections) == 0 {
		return interpretationassets.ReportSpec{}
	}
	sections := make([]interpretationassets.ReportSection, 0, len(reportMap.Sections))
	for _, section := range reportMap.Sections {
		sections = append(sections, interpretationassets.ReportSection{
			Code: section.Code, Title: section.Title, SourceRefs: append([]string(nil), section.SourceRefs...),
			Kind: section.Kind, AdapterKey: section.AdapterKey, TemplateID: section.TemplateID,
			TemplateVersion: section.TemplateVersion, CategoryLabel: section.CategoryLabel,
		})
	}
	return interpretationassets.ReportSpec{Sections: sections}
}
