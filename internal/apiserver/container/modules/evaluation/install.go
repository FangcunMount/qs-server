package evaluation

import (
	cachepolicy "github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/compose"
	surveymod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/survey"
	mysqlEventTx "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventtransaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/workbenchreadmodel"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
)

// InstallHost extends the shared compose seam with evaluation module bindings.
type InstallHost interface {
	compose.Host
	SurveyRuntimeInfra() *surveymod.SurveyRuntimeInfra
	SetEvaluationModule(*Module)
	SetWorkbenchLatestRiskReader(workbenchreadmodel.LatestRiskReader)
}

// TransactionInstallHost supplies one host-owned qs.mysql runtime. The host
// retains Start/admission/drain/Close responsibilities for all sharing modules.
type TransactionInstallHost interface {
	EvaluationTransactionOwner() *mysqlEventTx.Committer
	EvaluationTransactionRoutes() eventcatalog.TopicResolver
}

// InstallFrom wires and registers the evaluation module using composition-root host inputs.
func InstallFrom(host InstallHost) error {
	catalog, err := host.DefaultEvaluationCatalog()
	if err != nil {
		return err
	}
	provider := host.CachePolicyProvider()
	detail := compose.ResolveCacheCapability(provider, cachepolicy.CapabilityEvaluationAssessmentDetail)
	access := compose.ResolveCacheCapability(provider, cachepolicy.CapabilityEvaluationAssessmentAccess)
	objectRedis := host.CacheClient(redisruntime.FamilyObject)
	if !detail.Enabled && !access.Enabled {
		objectRedis = nil
	}
	var owner *mysqlEventTx.Committer
	var routes eventcatalog.TopicResolver
	if transactions, ok := host.(TransactionInstallHost); ok {
		owner = transactions.EvaluationTransactionOwner()
		routes = transactions.EvaluationTransactionRoutes()
	}
	result, err := Wire(WireInput{
		MySQLDB:                   host.MySQLDB(),
		MongoDB:                   host.MongoDB(),
		RedisClient:               objectRedis,
		CacheBuilder:              host.CacheBuilder(redisruntime.FamilyObject),
		CachePolicies:             provider,
		Observer:                  host.CacheObserver(),
		MySQLLimiter:              host.MySQLLimiter(),
		MongoLimiter:              host.MongoLimiter(),
		TesteeAccessChecker:       NewTesteeAccessChecker(host.ActorPorts().TesteeAccess),
		NormSubjectReader:         NewNormSubjectReader(host.ActorPorts().TesteeReader),
		SurveyRuntimeInfra:        host.SurveyRuntimeInfra(),
		PublishedModelCatalog:     host.PublishedModelCatalog(),
		RuntimeDescriptorRegistry: catalog.RuntimeDescriptorRegistry,
		OutboxProfile:             host.EventProfile(eventcatalog.OutboxProfileAssessmentMySQL),
		EventTransactionOwner:     owner,
		TransactionRoutes:         routes,
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
