package cachepolicy

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/model"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
)

// Spec is the single source of identity, ownership, routing and observability
// metadata for one apiserver cache capability.
type Spec struct {
	ID            sharedcache.Capability
	Owner         string
	Kind          sharedcache.CapabilityKind
	Layer         sharedcache.Layer
	Family        cachemodel.Family
	ConfigPath    string
	MetricLabel   string
	TopologyGroup string
	TopologyOrder int
	ReadModel     string
	Defaults      sharedcache.Policy
}

type Binding struct {
	Enabled bool
	Policy  sharedcache.Policy
}

var specs = []Spec{
	{ID: CapabilitySurveyQuestionnaire, Owner: "survey", Kind: sharedcache.KindCache, Layer: sharedcache.LayerL2, Family: cachemodel.FamilyStatic, ConfigPath: "cache.capabilities.survey.questionnaire", MetricLabel: "questionnaire", TopologyGroup: "questionnaire", TopologyOrder: 20, ReadModel: "questionnaire published Mongo read model", Defaults: sharedcache.Policy{TTL: 12 * time.Hour, Negative: sharedcache.PolicySwitchEnabled}},
	{ID: CapabilityModelCatalogPublished, Owner: "modelcatalog", Kind: sharedcache.KindCache, Layer: sharedcache.LayerL1L2, Family: cachemodel.FamilyStatic, ConfigPath: "cache.capabilities.modelcatalog.published_model", MetricLabel: "published_model", TopologyGroup: "published-model", TopologyOrder: 20, ReadModel: "published-model Mongo snapshot", Defaults: sharedcache.Policy{TTL: 24 * time.Hour, Negative: sharedcache.PolicySwitchDisabled}},
	{ID: CapabilityEvaluationAssessmentDetail, Owner: "evaluation", Kind: sharedcache.KindCache, Layer: sharedcache.LayerL2, Family: cachemodel.FamilyObject, ConfigPath: "cache.capabilities.evaluation.assessment_detail", MetricLabel: "assessment_detail", TopologyGroup: "assessment-detail", TopologyOrder: 20, ReadModel: "MySQL evaluation assessment read model", Defaults: sharedcache.Policy{TTL: 2 * time.Hour, Singleflight: sharedcache.PolicySwitchEnabled}},
	{ID: CapabilityEvaluationAssessmentAccess, Owner: "evaluation", Kind: sharedcache.KindCache, Layer: sharedcache.LayerL2, Family: cachemodel.FamilyObject, ConfigPath: "cache.capabilities.evaluation.assessment_access", MetricLabel: "assessment_access", TopologyGroup: "assessment-access", TopologyOrder: 20, ReadModel: "MySQL assessment ownership lookup", Defaults: sharedcache.Policy{TTL: 5 * time.Minute, Singleflight: sharedcache.PolicySwitchEnabled}},
	{ID: CapabilityActorTestee, Owner: "actor", Kind: sharedcache.KindCache, Layer: sharedcache.LayerL2, Family: cachemodel.FamilyObject, ConfigPath: "cache.capabilities.actor.testee", MetricLabel: "testee", Defaults: sharedcache.Policy{TTL: 30 * time.Minute, Negative: sharedcache.PolicySwitchEnabled}},
	{ID: CapabilityPlanDetail, Owner: "plan", Kind: sharedcache.KindCache, Layer: sharedcache.LayerL2, Family: cachemodel.FamilyObject, ConfigPath: "cache.capabilities.plan.detail", MetricLabel: "plan", Defaults: sharedcache.Policy{TTL: 2 * time.Hour, Singleflight: sharedcache.PolicySwitchEnabled}},
	{ID: CapabilityStatisticsQuery, Owner: "statistics", Kind: sharedcache.KindCache, Layer: sharedcache.LayerL2, Family: cachemodel.FamilyQuery, ConfigPath: "cache.capabilities.statistics.query", MetricLabel: "stats_query", Defaults: sharedcache.Policy{TTL: 26 * time.Hour, Singleflight: sharedcache.PolicySwitchDisabled}},
	{ID: CapabilityReportStatus, Owner: "interpretation", Kind: sharedcache.KindOperationalState, Layer: sharedcache.LayerRuntime, Family: cachemodel.FamilyOps, ConfigPath: "runtime_state.report_status", MetricLabel: "report_status", Defaults: sharedcache.Policy{TTL: 48 * time.Hour, Singleflight: sharedcache.PolicySwitchDisabled, Negative: sharedcache.PolicySwitchDisabled}},
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
	globalDefault  sharedcache.Policy
	familyDefaults map[cachemodel.Family]sharedcache.Policy
	bindings       map[sharedcache.Capability]Binding
}

func NewPolicyCatalog(globalDefault sharedcache.Policy, familyDefaults map[cachemodel.Family]sharedcache.Policy, bindings map[sharedcache.Capability]Binding) *PolicyCatalog {
	catalog := &PolicyCatalog{globalDefault: globalDefault, familyDefaults: make(map[cachemodel.Family]sharedcache.Policy), bindings: make(map[sharedcache.Capability]Binding)}
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
	binding.Policy = binding.Policy.MergeWith(c.familyDefaults[spec.Family].MergeWith(c.globalDefault.MergeWith(spec.Defaults)))
	return binding, true
}

func (c *PolicyCatalog) Effective(id sharedcache.Capability) (sharedcache.EffectiveCapability, bool) {
	spec, ok := Lookup(id)
	if !ok || c == nil {
		return sharedcache.EffectiveCapability{}, false
	}
	binding, _ := c.Resolve(id)
	layers := sharedcache.PolicyLayers{
		SpecDefault: spec.Defaults, GlobalDefault: c.globalDefault,
		FamilyDefault: c.familyDefaults[spec.Family], Override: c.bindings[id].Policy,
	}
	return sharedcache.EffectiveCapability{
		Capability: spec.ID, Owner: spec.Owner, Kind: spec.Kind, Layer: spec.Layer,
		Family: string(spec.Family), Enabled: binding.Enabled, Layers: layers, Policy: binding.Policy,
		Source: spec.ConfigPath, CatalogVersion: "v3", MetricLabel: spec.MetricLabel,
		TopologyGroup: spec.TopologyGroup, TopologyOrder: spec.TopologyOrder, ReadModel: spec.ReadModel,
	}, true
}
