package shared

import (
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/scale/definition"
)

// Classification holds domain classification values assembled from DTO string slices.
type Classification struct {
	Category       scaledefinition.Category
	Stages         []scaledefinition.Stage
	ApplicableAges []scaledefinition.ApplicableAge
	Reporters      []scaledefinition.Reporter
	Tags           []scaledefinition.Tag
}

// ClassificationFromDTO maps flat string lists into domain classification types.
func ClassificationFromDTO(category string, stages, applicableAges, reporters, tags []string) Classification {
	c := Classification{
		Category:       scaledefinition.NewCategory(category),
		Stages:         make([]scaledefinition.Stage, 0, len(stages)),
		ApplicableAges: make([]scaledefinition.ApplicableAge, 0, len(applicableAges)),
		Reporters:      make([]scaledefinition.Reporter, 0, len(reporters)),
		Tags:           make([]scaledefinition.Tag, 0, len(tags)),
	}
	for _, stage := range stages {
		c.Stages = append(c.Stages, scaledefinition.NewStage(stage))
	}
	for _, age := range applicableAges {
		c.ApplicableAges = append(c.ApplicableAges, scaledefinition.NewApplicableAge(age))
	}
	for _, reporter := range reporters {
		c.Reporters = append(c.Reporters, scaledefinition.NewReporter(reporter))
	}
	for _, tag := range tags {
		c.Tags = append(c.Tags, scaledefinition.NewTag(tag))
	}
	return c
}

// InterpretRulesFromDTOs converts interpret rule DTOs to domain rules.
func InterpretRulesFromDTOs(dtos []InterpretRuleDTO) []scaledefinition.InterpretationRule {
	rules := make([]scaledefinition.InterpretationRule, 0, len(dtos))
	for _, dto := range dtos {
		rules = append(rules, scaledefinition.NewInterpretationRule(
			scaledefinition.NewScoreRange(dto.MinScore, dto.MaxScore),
			scaledefinition.RiskLevel(dto.RiskLevel),
			dto.Conclusion,
			dto.Suggestion,
		))
	}
	return rules
}
