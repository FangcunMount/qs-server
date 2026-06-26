package container

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachequery"
	evaluationinputInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/evaluationinput"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	rulesetInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/assessmentmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
)

func (c *Container) ensureRuleSetCatalog() (rulesetport.RuleSetCatalog, error) {
	if c == nil {
		return nil, fmt.Errorf("container is nil")
	}
	if c.ruleSetCatalog != nil {
		return c.ruleSetCatalog, nil
	}
	mongoOpts := mongoBase.BaseRepositoryOptions{Limiter: c.backpressure.Mongo}
	var scaleSource rulesetInfra.ScaleBindingSource
	if infra := c.surveyScaleInfra; infra != nil && infra.scaleRepo != nil {
		scaleSource = evaluationinputInfra.NewRepositoryScaleBindingSource(infra.scaleRepo)
	}
	catalog, err := rulesetInfra.NewCatalog(c.mongoDB, scaleSource, mongoOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize ruleset catalog: %w", err)
	}
	c.ruleSetCatalog = catalog
	return catalog, nil
}

func (c *Container) buildEvaluationModuleDeps() (assembler.EvaluationModuleDeps, error) {
	var infra *surveyScaleInfra
	if c != nil {
		infra = c.surveyScaleInfra
	}
	var inputResolver evaluationinput.Resolver
	var scaleCatalog evaluationinput.ScaleCatalog
	if infra != nil {
		catalog, err := c.ensureRuleSetCatalog()
		if err != nil {
			return assembler.EvaluationModuleDeps{}, err
		}
		resolver, err := evaluationinputInfra.NewRepositoryResolver(
			infra.scaleRepo,
			infra.answerSheetRepo,
			infra.questionnaireRepo,
			catalog,
			assembler.DefaultEvaluationDescriptors(),
		)
		if err != nil {
			return assembler.EvaluationModuleDeps{}, fmt.Errorf("failed to initialize evaluation input resolver: %w", err)
		}
		inputResolver = resolver
		scaleCatalog = resolver
	}

	redisClient := c.CacheClient(cacheplane.FamilyObject)
	queryRedisClient := c.CacheClient(cacheplane.FamilyQuery)
	if c.cacheOptions.DisableEvaluationCache {
		redisClient = nil
		queryRedisClient = nil
	}

	var versionStore cachequery.VersionTokenStore
	if queryRedisClient != nil {
		versionStore = cachequery.NewRedisVersionTokenStoreWithKindAndObserver(
			c.CacheClient(cacheplane.FamilyMeta),
			string(cachepolicy.PolicyAssessmentList),
			c.cacheObserver(),
		)
	}

	return assembler.EvaluationModuleDeps{
		MySQLDB:                        c.mysqlDB,
		MongoDB:                        c.mongoDB,
		InputResolver:                  inputResolver,
		ScaleCatalog:                   scaleCatalog,
		EventPublisher:                 c.eventPublisher,
		RedisClient:                    redisClient,
		CacheBuilder:                   c.CacheBuilder(cacheplane.FamilyObject),
		AssessmentPolicy:               c.CachePolicy(cachepolicy.PolicyAssessmentDetail),
		QueryRedisClient:               queryRedisClient,
		QueryCacheBuilder:              c.CacheBuilder(cacheplane.FamilyQuery),
		AssessmentListPolicy:           c.CachePolicy(cachepolicy.PolicyAssessmentList),
		VersionStore:                   versionStore,
		Observer:                       c.cacheObserver(),
		TopicResolver:                  c.eventCatalog,
		MySQLLimiter:                   c.backpressure.MySQL,
		MongoLimiter:                   c.backpressure.Mongo,
		AssessmentOutboxRelayBatchSize: c.outboxRelay.AssessmentBatchSize,
		TesteeAccessChecker:            newEvaluationTesteeAccessChecker(c.actorTesteeAccessService()),
		OpsHandle:                      c.CacheHandle(cacheplane.FamilyOps),
		ReportStatusConfig:             c.reportStatusConfig,
	}, nil
}

func (c *Container) buildEvaluationModule() (*assembler.EvaluationModule, error) {
	deps, err := c.buildEvaluationModuleDeps()
	if err != nil {
		return nil, err
	}
	return assembler.NewEvaluationModule(deps)
}

// initEvaluationModule 初始化 Evaluation 模块。
func (c *Container) initEvaluationModule() error {
	evaluationModule, err := c.buildEvaluationModule()
	if err != nil {
		return fmt.Errorf("failed to initialize evaluation module: %w", err)
	}

	c.EvaluationModule = evaluationModule
	c.registerModule("evaluation", evaluationModule)

	c.printf("📦 Evaluation module initialized\n")
	return nil
}
