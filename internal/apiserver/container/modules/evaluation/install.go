package evaluation

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/compose"
	surveymod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/survey"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/workbenchreadmodel"
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
	result, err := Wire(WireInput{
		MySQLDB:                             host.MySQLDB(),
		MongoDB:                             host.MongoDB(),
		EventPublisher:                      host.EventPublisher(),
		RedisClient:                         host.CacheClient(redisruntime.FamilyObject),
		CacheBuilder:                        host.CacheBuilder(redisruntime.FamilyObject),
		QueryRedisClient:                    host.CacheClient(redisruntime.FamilyQuery),
		QueryCacheBuilder:                   host.CacheBuilder(redisruntime.FamilyQuery),
		MetaRedisClient:                     host.CacheClient(redisruntime.FamilyMeta),
		AssessmentPolicy:                    host.CachePolicy(cachepolicy.CapabilityEvaluationAssessmentDetail),
		AssessmentListPolicy:                host.CachePolicy(cachepolicy.CapabilityEvaluationAssessmentList),
		DisableEvaluationCache:              host.DisableEvaluationCache(),
		Observer:                            host.CacheObserver(),
		TopicResolver:                       host.TopicResolver(),
		MySQLLimiter:                        host.MySQLLimiter(),
		MongoLimiter:                        host.MongoLimiter(),
		AssessmentOutboxRelayBatchSize:      host.OutboxRelayAssessmentBatchSize(),
		AssessmentOutboxRelayPublishWorkers: host.OutboxRelayAssessmentPublishWorkers(),
		AssessmentOutboxRelayImmediateMaxConcurrent: host.OutboxRelayAssessmentImmediateMaxConcurrent(),
		TesteeAccessChecker:                         NewTesteeAccessChecker(host.ActorPorts().TesteeAccess),
		OpsHandle:                                   host.CacheHandle(redisruntime.FamilyOps),
		SurveyRuntimeInfra:                          host.SurveyRuntimeInfra(),
		PublishedModelCatalog:                       host.PublishedModelCatalog(),
		StaticRedisClient:                           host.CacheClient(redisruntime.FamilyStatic),
		StaticCacheBuilder:                          host.CacheBuilder(redisruntime.FamilyStatic),
		PublishedModelPolicy:                        host.CachePolicy(cachepolicy.CapabilityModelCatalogPublished),
		RuntimeDescriptorRegistry:                   catalog.RuntimeDescriptorRegistry,
	})
	if err != nil {
		return err
	}
	if result.PublishedModelCatalog != nil {
		host.SetPublishedModelCatalog(result.PublishedModelCatalog)
	}
	host.SetEvaluationModule(result.Module)
	host.SetWorkbenchLatestRiskReader(result.WorkbenchLatestRiskReader)
	host.RegisterModule("evaluation", result.Module)
	host.Printf("📦 Evaluation module initialized\n")
	return nil
}
