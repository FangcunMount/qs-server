package cachepolicy

import (
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
)

// NewEffectiveRegistry materializes the apiserver policy catalog with stable,
// path-derived capability IDs.
func NewEffectiveRegistry(catalog *PolicyCatalog) *sharedcache.Registry {
	type definition struct {
		key        CachePolicyKey
		id, source string
	}
	definitions := []definition{
		{PolicyScale, "catalog.scale", "capabilities.catalog.scale"},
		{PolicyQuestionnaire, "catalog.questionnaire", "capabilities.catalog.questionnaire"},
		{PolicyPublishedModel, "catalog.published_model", "capabilities.catalog.published_model"},
		{PolicyAssessmentDetail, "assessment.detail", "capabilities.assessment.detail"},
		{PolicyAssessmentList, "assessment.list", "capabilities.assessment.list"},
		{PolicyTestee, "actor.testee", "capabilities.actor.testee"},
		{PolicyPlan, "plan.detail", "capabilities.plan.detail"},
		{PolicyStatsQuery, "statistics.query", "capabilities.statistics.query"},
	}
	entries := make([]sharedcache.EffectiveCapability, 0, len(definitions))
	for _, definition := range definitions {
		entries = append(entries, sharedcache.EffectiveCapability{
			Capability: sharedcache.Capability(definition.id), Layer: sharedcache.LayerL2,
			Family: string(FamilyFor(definition.key)), Policy: catalog.Policy(definition.key),
			Source: "cache." + definition.source, Version: "v1",
		})
	}
	return sharedcache.NewRegistry(entries...)
}
