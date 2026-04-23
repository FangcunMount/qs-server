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

func (c *Container) buildSurveyModuleInitializeParams() []interface{} {
	return []interface{}{
		c.mongoDB,
		c.eventPublisher,
		c.CacheClient(redisplane.FamilyStatic),
		c.CacheBuilder(redisplane.FamilyStatic),
		c.resolveIdentityService(),
		c.CachePolicy(cachepolicy.PolicyQuestionnaire),
		c.hotsetRecorder(),
		c.cacheObserver(),
	}
}

// initSurveyModule 初始化 Survey 模块（包含问卷和答卷子模块）。
func (c *Container) initSurveyModule() error {
	surveyModule := assembler.NewSurveyModule()
	if err := surveyModule.Initialize(c.buildSurveyModuleInitializeParams()...); err != nil {
		return fmt.Errorf("failed to initialize survey module: %w", err)
	}

	c.SurveyModule = surveyModule
	c.registerModule("survey", surveyModule)

	c.printf("📦 Survey module initialized (questionnaire + answersheet)\n")
	return nil
}

func (c *Container) wireSurveyScaleDependencies() {
	if c == nil || c.SurveyModule == nil || c.ScaleModule == nil {
		return
	}
	c.SurveyModule.SetScaleRepository(c.ScaleModule.Repo)
}
