package shared

import (
	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/authoring/scale"
)

// Classification holds domain classification values assembled from DTO string slices.
type Classification struct {
	Category       domainScale.Category
	Stages         []domainScale.Stage
	ApplicableAges []domainScale.ApplicableAge
	Reporters      []domainScale.Reporter
	Tags           []domainScale.Tag
}

// ClassificationFromDTO maps flat string lists into domain classification types.
func ClassificationFromDTO(category string, stages, applicableAges, reporters, tags []string) Classification {
	c := Classification{
		Category:       domainScale.NewCategory(category),
		Stages:         make([]domainScale.Stage, 0, len(stages)),
		ApplicableAges: make([]domainScale.ApplicableAge, 0, len(applicableAges)),
		Reporters:      make([]domainScale.Reporter, 0, len(reporters)),
		Tags:           make([]domainScale.Tag, 0, len(tags)),
	}
	for _, stage := range stages {
		c.Stages = append(c.Stages, domainScale.NewStage(stage))
	}
	for _, age := range applicableAges {
		c.ApplicableAges = append(c.ApplicableAges, domainScale.NewApplicableAge(age))
	}
	for _, reporter := range reporters {
		c.Reporters = append(c.Reporters, domainScale.NewReporter(reporter))
	}
	for _, tag := range tags {
		c.Tags = append(c.Tags, domainScale.NewTag(tag))
	}
	return c
}

// InterpretRulesFromDTOs converts interpret rule DTOs to domain rules.
func InterpretRulesFromDTOs(dtos []InterpretRuleDTO) []domainScale.InterpretationRule {
	rules := make([]domainScale.InterpretationRule, 0, len(dtos))
	for _, dto := range dtos {
		rules = append(rules, domainScale.NewInterpretationRule(
			domainScale.NewScoreRange(dto.MinScore, dto.MaxScore),
			domainScale.RiskLevel(dto.RiskLevel),
			dto.Conclusion,
			dto.Suggestion,
		))
	}
	return rules
}
