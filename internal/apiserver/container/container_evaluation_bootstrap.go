package container

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
	scaleCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
)

func (c *Container) buildEvaluationModuleInitializeParams() []interface{} {
	var scaleRepo interface{}
	if c != nil && c.ScaleModule != nil {
		scaleRepo = c.ScaleModule.Repo
	}

	var answerSheetRepo interface{}
	var questionnaireRepo interface{}
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

	var versionStore scaleCache.VersionTokenStore
	if queryRedisClient != nil {
		versionStore = scaleCache.NewRedisVersionTokenStoreWithKindAndObserver(
			c.CacheClient(redisplane.FamilyMeta),
			string(cachepolicy.PolicyAssessmentList),
			c.cacheObserver(),
		)
	}

	return []interface{}{
		c.mysqlDB,
		c.mongoDB,
		scaleRepo,
		answerSheetRepo,
		questionnaireRepo,
		c.eventPublisher,
		redisClient,
		c.CacheBuilder(redisplane.FamilyObject),
		c.CachePolicy(cachepolicy.PolicyAssessmentDetail),
		queryRedisClient,
		c.CacheBuilder(redisplane.FamilyQuery),
		c.CachePolicy(cachepolicy.PolicyAssessmentList),
		versionStore,
		c.cacheObserver(),
	}
}

// initEvaluationModule 初始化 Evaluation 模块。
func (c *Container) initEvaluationModule() error {
	evaluationModule := assembler.NewEvaluationModule()
	if err := evaluationModule.Initialize(c.buildEvaluationModuleInitializeParams()...); err != nil {
		return fmt.Errorf("failed to initialize evaluation module: %w", err)
	}

	c.EvaluationModule = evaluationModule
	c.registerModule("evaluation", evaluationModule)

	c.printf("📦 Evaluation module initialized\n")
	return nil
}
