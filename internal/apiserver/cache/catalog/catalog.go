package cachepolicy

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/model"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
)

// Spec is the single source of identity, ownership, routing and observability
// metadata for one apiserver cache capability.
type Spec struct {
	ID          sharedcache.Capability
	Owner       string
	Kind        sharedcache.CapabilityKind
	Layer       sharedcache.Layer
	Family      cachemodel.Family
	ConfigPath  string
	MetricLabel string
	Defaults    sharedcache.Policy
}

type Binding struct {
	Enabled bool
	Policy  sharedcache.Policy
}

var specs = []Spec{
	{ID: CapabilitySurveyQuestionnaire, Owner: "survey", Kind: sharedcache.KindCache, Layer: sharedcache.LayerL2, Family: cachemodel.FamilyStatic, ConfigPath: "cache.capabilities.survey.questionnaire", MetricLabel: "questionnaire", Defaults: sharedcache.Policy{Negative: sharedcache.PolicySwitchEnabled}},
	{ID: CapabilityModelCatalogPublished, Owner: "modelcatalog", Kind: sharedcache.KindCache, Layer: sharedcache.LayerL2, Family: cachemodel.FamilyStatic, ConfigPath: "cache.capabilities.modelcatalog.published_model", MetricLabel: "published_model", Defaults: sharedcache.Policy{Negative: sharedcache.PolicySwitchEnabled}},
	{ID: CapabilityEvaluationAssessmentDetail, Owner: "evaluation", Kind: sharedcache.KindCache, Layer: sharedcache.LayerL2, Family: cachemodel.FamilyObject, ConfigPath: "cache.capabilities.evaluation.assessment_detail", MetricLabel: "assessment_detail", Defaults: sharedcache.Policy{Singleflight: sharedcache.PolicySwitchEnabled}},
	{ID: CapabilityEvaluationAssessmentList, Owner: "evaluation", Kind: sharedcache.KindCache, Layer: sharedcache.LayerL2, Family: cachemodel.FamilyQuery, ConfigPath: "cache.capabilities.evaluation.assessment_list", MetricLabel: "assessment_list", Defaults: sharedcache.Policy{Singleflight: sharedcache.PolicySwitchDisabled}},
	{ID: CapabilityActorTestee, Owner: "actor", Kind: sharedcache.KindCache, Layer: sharedcache.LayerL2, Family: cachemodel.FamilyObject, ConfigPath: "cache.capabilities.actor.testee", MetricLabel: "testee", Defaults: sharedcache.Policy{Negative: sharedcache.PolicySwitchEnabled}},
	{ID: CapabilityPlanDetail, Owner: "plan", Kind: sharedcache.KindCache, Layer: sharedcache.LayerL2, Family: cachemodel.FamilyObject, ConfigPath: "cache.capabilities.plan.detail", MetricLabel: "plan", Defaults: sharedcache.Policy{Singleflight: sharedcache.PolicySwitchEnabled}},
	{ID: CapabilityStatisticsQuery, Owner: "statistics", Kind: sharedcache.KindCache, Layer: sharedcache.LayerL2, Family: cachemodel.FamilyQuery, ConfigPath: "cache.capabilities.statistics.query", MetricLabel: "stats_query", Defaults: sharedcache.Policy{Singleflight: sharedcache.PolicySwitchDisabled}},
	{ID: CapabilityReportStatus, Owner: "interpretation", Kind: sharedcache.KindOperationalState, Layer: sharedcache.LayerRuntime, Family: cachemodel.FamilyOps, ConfigPath: "cache.capabilities.report_status", MetricLabel: "report_status"},
}

func Specs() []Spec { return append([]Spec(nil), specs...) }

func Lookup(id sharedcache.Capability) (Spec, bool) {
	for _, spec := range specs {
		if spec.ID == id {
			return spec, true
		}
	}
	return Spec{}, false
}

func Family(id sharedcache.Capability) cachemodel.Family {
	spec, ok := Lookup(id)
	if !ok {
		return cachemodel.FamilyDefault
	}
	return spec.Family
}

func MetricLabel(id sharedcache.Capability) string {
	spec, ok := Lookup(id)
	if !ok {
		return string(id)
	}
	return spec.MetricLabel
}

type PolicyCatalog struct {
	familyDefaults map[cachemodel.Family]sharedcache.Policy
	bindings       map[sharedcache.Capability]Binding
}

func NewPolicyCatalog(familyDefaults map[cachemodel.Family]sharedcache.Policy, bindings map[sharedcache.Capability]Binding) *PolicyCatalog {
	catalog := &PolicyCatalog{familyDefaults: make(map[cachemodel.Family]sharedcache.Policy), bindings: make(map[sharedcache.Capability]Binding)}
	for family, policy := range familyDefaults {
		catalog.familyDefaults[family] = policy
	}
	for id, binding := range bindings {
		catalog.bindings[id] = binding
	}
	return catalog
}

func (c *PolicyCatalog) Resolve(id sharedcache.Capability) (Binding, bool) {
	spec, ok := Lookup(id)
	if !ok || c == nil {
		return Binding{}, false
	}
	binding, configured := c.bindings[id]
	if !configured {
		binding.Enabled = spec.Kind == sharedcache.KindCache
	}
	binding.Policy = binding.Policy.MergeWith(spec.Defaults.MergeWith(c.familyDefaults[spec.Family]))
	return binding, true
}

func (c *PolicyCatalog) Policy(id sharedcache.Capability) sharedcache.Policy {
	binding, _ := c.Resolve(id)
	return binding.Policy
}
