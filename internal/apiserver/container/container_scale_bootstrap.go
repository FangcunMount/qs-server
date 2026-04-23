package container

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
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
		c.staticRedisCache,
		redisHandleBuilder(c.staticRedisHandle),
		c.resolveIdentityService(),
		c.policyCatalog.Policy(cachepolicy.PolicyScale),
		c.policyCatalog.Policy(cachepolicy.PolicyScaleList),
		c.hotsetRecorder,
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
