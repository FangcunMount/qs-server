package container

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachequery"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
)

func (c *Container) buildEvaluationModuleDeps() assembler.EvaluationModuleDeps {
	var infra *surveyScaleInfra
	if c != nil {
		infra = c.surveyScaleInfra
	}
	var scaleRepo scale.Repository
	var answerSheetRepo answersheet.Repository
	var questionnaireRepo questionnaire.Repository
	if infra != nil {
		scaleRepo = infra.scaleRepo
		answerSheetRepo = infra.answerSheetRepo
		questionnaireRepo = infra.questionnaireRepo
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
		ScaleRepo:            scaleRepo,
		AnswerSheetRepo:      answerSheetRepo,
		QuestionnaireRepo:    questionnaireRepo,
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
		TesteeAccessService:  c.actorTesteeAccessService(),
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
