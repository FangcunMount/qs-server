package evaluation

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/compose"
	surveymod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/survey"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/workbenchreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
)

// InstallHost extends the shared compose seam with evaluation module bindings.
type InstallHost interface {
	compose.Host
	SurveyRuntimeInfra() *surveymod.SurveyRuntimeInfra
	SetEvaluationModule(*Module)
	SetWorkbenchLatestRiskReader(workbenchreadmodel.LatestRiskReader)
}

// InstallFrom wires and registers the evaluation module using composition-root host inputs.
func InstallFrom(host InstallHost) error {
	catalog, err := host.DefaultEvaluationCatalog()
	if err != nil {
		return err
	}
	provider := host.CachePolicyProvider()
	detail := compose.ResolveCacheCapability(provider, cachepolicy.CapabilityEvaluationAssessmentDetail)
	list := compose.ResolveCacheCapability(provider, cachepolicy.CapabilityEvaluationAssessmentList)
	objectRedis := host.CacheClient(redisruntime.FamilyObject)
	queryRedis := host.CacheClient(redisruntime.FamilyQuery)
	if !detail.Enabled {
		objectRedis = nil
	}
	if !list.Enabled {
		queryRedis = nil
	}
	result, err := Wire(WireInput{
		MySQLDB:                   host.MySQLDB(),
		MongoDB:                   host.MongoDB(),
		EventPublisher:            host.EventPublisher(),
		RedisClient:               objectRedis,
		CacheBuilder:              host.CacheBuilder(redisruntime.FamilyObject),
		QueryRedisClient:          queryRedis,
		QueryCacheBuilder:         host.CacheBuilder(redisruntime.FamilyQuery),
		MetaRedisClient:           host.CacheClient(redisruntime.FamilyMeta),
		CachePolicies:             provider,
		Observer:                  host.CacheObserver(),
		MySQLLimiter:              host.MySQLLimiter(),
		MongoLimiter:              host.MongoLimiter(),
		TesteeAccessChecker:       NewTesteeAccessChecker(host.ActorPorts().TesteeAccess),
		NormSubjectReader:         NewNormSubjectReader(host.ActorPorts().TesteeQuery),
		SurveyRuntimeInfra:        host.SurveyRuntimeInfra(),
		PublishedModelCatalog:     host.PublishedModelCatalog(),
		RuntimeDescriptorRegistry: catalog.RuntimeDescriptorRegistry,
		OutboxProfile:             host.EventProfile(eventcatalog.OutboxProfileAssessmentMySQL),
	})
	if err != nil {
		return err
	}
	host.SetEvaluationModule(result.Module)
	host.SetWorkbenchLatestRiskReader(result.WorkbenchLatestRiskReader)
	host.RegisterModule("evaluation", result.Module)
	host.Printf("📦 Evaluation module initialized\n")
	return nil
}
