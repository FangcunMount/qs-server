package scale

import (
	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
)

type scaleClassification struct {
	category       domainScale.Category
	stages         []domainScale.Stage
	applicableAges []domainScale.ApplicableAge
	reporters      []domainScale.Reporter
	tags           []domainScale.Tag
}

func scaleClassificationFromDTO(category string, stages, applicableAges, reporters, tags []string) scaleClassification {
	classification := scaleClassification{
		category:       domainScale.NewCategory(category),
		stages:         make([]domainScale.Stage, 0, len(stages)),
		applicableAges: make([]domainScale.ApplicableAge, 0, len(applicableAges)),
		reporters:      make([]domainScale.Reporter, 0, len(reporters)),
		tags:           make([]domainScale.Tag, 0, len(tags)),
	}

	for _, stage := range stages {
		classification.stages = append(classification.stages, domainScale.NewStage(stage))
	}
	for _, age := range applicableAges {
		classification.applicableAges = append(classification.applicableAges, domainScale.NewApplicableAge(age))
	}
	for _, reporter := range reporters {
		classification.reporters = append(classification.reporters, domainScale.NewReporter(reporter))
	}
	for _, tag := range tags {
		classification.tags = append(classification.tags, domainScale.NewTag(tag))
	}

	return classification
}

func interpretRulesFromDTOs(dtos []InterpretRuleDTO) []domainScale.InterpretationRule {
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
