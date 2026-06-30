package evaluation

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/container/compose"
	surveymod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/survey"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
)

// ReportIntegrationPorts carries report-side integration ports without importing the report module.
type ReportIntegrationPorts = compose.ReportIntegrationPorts

// InstallHost extends the shared compose seam with evaluation module bindings.
type InstallHost interface {
	compose.Host
	SurveyScaleInfra() *surveymod.ScaleInfra
	SetEvaluationModule(*Module)
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
		RedisClient:                         host.CacheClient(cacheplane.FamilyObject),
		CacheBuilder:                        host.CacheBuilder(cacheplane.FamilyObject),
		QueryRedisClient:                    host.CacheClient(cacheplane.FamilyQuery),
		QueryCacheBuilder:                   host.CacheBuilder(cacheplane.FamilyQuery),
		MetaRedisClient:                     host.CacheClient(cacheplane.FamilyMeta),
		AssessmentPolicy:                    host.CachePolicy(cachepolicy.PolicyAssessmentDetail),
		AssessmentListPolicy:                host.CachePolicy(cachepolicy.PolicyAssessmentList),
		DisableEvaluationCache:              host.DisableEvaluationCache(),
		Observer:                            host.CacheObserver(),
		TopicResolver:                       host.TopicResolver(),
		MySQLLimiter:                        host.MySQLLimiter(),
		MongoLimiter:                        host.MongoLimiter(),
		AssessmentOutboxRelayBatchSize:      host.OutboxRelayAssessmentBatchSize(),
		AssessmentOutboxRelayPublishWorkers: host.OutboxRelayAssessmentPublishWorkers(),
		TesteeAccessChecker:                 NewTesteeAccessChecker(host.ActorPorts().TesteeAccess),
		OpsHandle:                           host.CacheHandle(cacheplane.FamilyOps),
		ReportStatusConfig:                  host.ReportStatusConfig(),
		ScaleInfra:                          host.SurveyScaleInfra(),
		RuleSetCatalog:                      host.RuleSetCatalog(),
		StaticRedisClient:                   host.CacheClient(cacheplane.FamilyStatic),
		StaticCacheBuilder:                  host.CacheBuilder(cacheplane.FamilyStatic),
		PublishedModelPolicy:                host.CachePolicy(cachepolicy.PolicyPublishedModel),
		ModelDescriptors:                    catalog.Descriptors,
		TypologyRegistry:                    catalog.TypologyRegistry,
		ReportPorts:                         host.ReportIntegrationPorts(),
	})
	if err != nil {
		return err
	}
	if result.RuleSetCatalog != nil {
		host.SetRuleSetCatalog(result.RuleSetCatalog)
	}
	host.SetEvaluationModule(result.Module)
	host.RegisterModule("evaluation", result.Module)
	host.Printf("📦 Evaluation module initialized\n")
	return nil
}
