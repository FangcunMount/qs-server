package container

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachequery"
	evaluationinputInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
)

func (c *Container) buildEvaluationModuleDeps() assembler.EvaluationModuleDeps {
	var infra *surveyScaleInfra
	if c != nil {
		infra = c.surveyScaleInfra
	}
	var inputResolver evaluationinput.Resolver
	var scaleCatalog evaluationinput.ScaleCatalog
	if infra != nil {
		resolver := evaluationinputInfra.NewRepositoryResolver(
			infra.scaleRepo,
			infra.answerSheetRepo,
			infra.questionnaireRepo,
		)
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
		MySQLDB:              c.mysqlDB,
		MongoDB:              c.mongoDB,
		InputResolver:        inputResolver,
		ScaleCatalog:         scaleCatalog,
		EventPublisher:       c.eventPublisher,
		RedisClient:          redisClient,
		CacheBuilder:         c.CacheBuilder(cacheplane.FamilyObject),
		AssessmentPolicy:     c.CachePolicy(cachepolicy.PolicyAssessmentDetail),
		QueryRedisClient:     queryRedisClient,
		QueryCacheBuilder:    c.CacheBuilder(cacheplane.FamilyQuery),
		AssessmentListPolicy: c.CachePolicy(cachepolicy.PolicyAssessmentList),
		VersionStore:         versionStore,
		Observer:             c.cacheObserver(),
		TopicResolver:        c.eventCatalog,
		MySQLLimiter:         c.backpressure.MySQL,
		MongoLimiter:         c.backpressure.Mongo,
		TesteeAccessChecker:  newEvaluationTesteeAccessChecker(c.actorTesteeAccessService()),
	}
}

func (c *Container) buildEvaluationModule() (*assembler.EvaluationModule, error) {
	return assembler.NewEvaluationModule(c.buildEvaluationModuleDeps())
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
