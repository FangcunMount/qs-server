package container

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
)

func (c *Container) resolveIdentityService() *iam.IdentityService {
	if c == nil || c.IAMModule == nil || !c.IAMModule.IsEnabled() {
		return nil
	}
	return c.IAMModule.IdentityService()
}

func (c *Container) buildSurveyModuleDeps() assembler.SurveyModuleDeps {
	return assembler.SurveyModuleDeps{
		MongoDB:             c.mongoDB,
		EventPublisher:      c.eventPublisher,
		RedisClient:         c.CacheClient(redisplane.FamilyStatic),
		CacheBuilder:        c.CacheBuilder(redisplane.FamilyStatic),
		IdentityService:     c.resolveIdentityService(),
		QuestionnairePolicy: c.CachePolicy(cachepolicy.PolicyQuestionnaire),
		HotsetRecorder:      c.hotsetRecorder(),
		Observer:            c.cacheObserver(),
		TopicResolver:       c.eventCatalog,
		MongoLimiter:        c.backpressure.Mongo,
	}
}

func (c *Container) buildSurveyModule() (*assembler.SurveyModule, error) {
	return assembler.NewSurveyModule(c.buildSurveyModuleDeps())
}

// initSurveyModule 初始化 Survey 模块（包含问卷和答卷子模块）。
func (c *Container) initSurveyModule() error {
	surveyModule, err := c.buildSurveyModule()
	if err != nil {
		return fmt.Errorf("failed to initialize survey module: %w", err)
	}

	c.SurveyModule = surveyModule
	c.registerModule("survey", surveyModule)

	c.printf("📦 Survey module initialized (questionnaire + answersheet)\n")
	return nil
}
