package container

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
)

func (c *Container) buildScaleModuleDeps() assembler.ScaleModuleDeps {
	var questionnaireRepo domainQuestionnaire.Repository
	if c != nil && c.SurveyModule != nil && c.SurveyModule.Questionnaire != nil {
		questionnaireRepo = c.SurveyModule.Questionnaire.Repo
	}

	return assembler.ScaleModuleDeps{
		MongoDB:           c.mongoDB,
		EventPublisher:    c.eventPublisher,
		QuestionnaireRepo: questionnaireRepo,
		RedisClient:       c.CacheClient(cacheplane.FamilyStatic),
		CacheBuilder:      c.CacheBuilder(cacheplane.FamilyStatic),
		IdentityService:   c.resolveIdentityService(),
		ScalePolicy:       c.CachePolicy(cachepolicy.PolicyScale),
		ScaleListPolicy:   c.CachePolicy(cachepolicy.PolicyScaleList),
		HotsetRecorder:    c.hotsetRecorder(),
		Observer:          c.cacheObserver(),
		MongoLimiter:      c.backpressure.Mongo,
	}
}

func (c *Container) buildScaleModule() (*assembler.ScaleModule, error) {
	return assembler.NewScaleModule(c.buildScaleModuleDeps())
}

// initScaleModule 初始化 Scale 模块。
func (c *Container) initScaleModule() error {
	scaleModule, err := c.buildScaleModule()
	if err != nil {
		return fmt.Errorf("failed to initialize scale module: %w", err)
	}

	c.ScaleModule = scaleModule
	c.registerModule("scale", scaleModule)

	c.printf("📦 Scale module initialized\n")
	return nil
}
