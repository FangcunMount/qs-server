package container

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachequery"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
)

func (c *Container) buildEvaluationModuleDeps() assembler.EvaluationModuleDeps {
	var scaleRepo scale.Repository
	if c != nil && c.ScaleModule != nil {
		scaleRepo = c.ScaleModule.Repo
	}

	var answerSheetRepo answersheet.Repository
	var questionnaireRepo questionnaire.Repository
	if c != nil && c.SurveyModule != nil {
		if c.SurveyModule.AnswerSheet != nil {
			answerSheetRepo = c.SurveyModule.AnswerSheet.Repo
		}
		if c.SurveyModule.Questionnaire != nil {
			questionnaireRepo = c.SurveyModule.Questionnaire.Repo
		}
	}

	redisClient := c.CacheClient(redisplane.FamilyObject)
	queryRedisClient := c.CacheClient(redisplane.FamilyQuery)
	if c.cacheOptions.DisableEvaluationCache {
		redisClient = nil
		queryRedisClient = nil
	}

	var versionStore cachequery.VersionTokenStore
	if queryRedisClient != nil {
		versionStore = cachequery.NewRedisVersionTokenStoreWithKindAndObserver(
			c.CacheClient(redisplane.FamilyMeta),
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
		CacheBuilder:         c.CacheBuilder(redisplane.FamilyObject),
		AssessmentPolicy:     c.CachePolicy(cachepolicy.PolicyAssessmentDetail),
		QueryRedisClient:     queryRedisClient,
		QueryCacheBuilder:    c.CacheBuilder(redisplane.FamilyQuery),
		AssessmentListPolicy: c.CachePolicy(cachepolicy.PolicyAssessmentList),
		VersionStore:         versionStore,
		Observer:             c.cacheObserver(),
		TopicResolver:        c.eventCatalog,
		MySQLLimiter:         c.backpressure.MySQL,
		MongoLimiter:         c.backpressure.Mongo,
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
