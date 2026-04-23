package container

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
)

func (c *Container) buildScaleModuleInitializeParams() []interface{} {
	var questionnaireRepo interface{}
	if c != nil && c.SurveyModule != nil && c.SurveyModule.Questionnaire != nil {
		questionnaireRepo = c.SurveyModule.Questionnaire.Repo
	}

	return []interface{}{
		c.mongoDB,
		c.eventPublisher,
		questionnaireRepo,
		c.CacheClient(redisplane.FamilyStatic),
		c.CacheBuilder(redisplane.FamilyStatic),
		c.resolveIdentityService(),
		c.CachePolicy(cachepolicy.PolicyScale),
		c.CachePolicy(cachepolicy.PolicyScaleList),
		c.hotsetRecorder(),
		c.cacheObserver(),
	}
}

// initScaleModule 初始化 Scale 模块。
func (c *Container) initScaleModule() error {
	scaleModule := assembler.NewScaleModule()
	if err := scaleModule.Initialize(c.buildScaleModuleInitializeParams()...); err != nil {
		return fmt.Errorf("failed to initialize scale module: %w", err)
	}

	c.ScaleModule = scaleModule
	c.registerModule("scale", scaleModule)

	c.printf("📦 Scale module initialized\n")
	return nil
}
