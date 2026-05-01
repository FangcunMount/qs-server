package container

import (
	"fmt"

	quesApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
)

func (c *Container) buildScaleModuleDeps() assembler.ScaleModuleDeps {
	var infra *surveyScaleInfra
	if c != nil {
		infra = c.surveyScaleInfra
	}

	deps := assembler.ScaleModuleDeps{
		EventPublisher:   c.eventPublisher,
		RankRedisClient:  c.CacheClient(cacheplane.FamilyRank),
		RankCacheBuilder: c.CacheBuilder(cacheplane.FamilyRank),
		IdentityService:  c.resolveIdentityService(),
		HotsetRecorder:   c.hotsetRecorder(),
	}
	if infra != nil {
		deps.Repo = infra.scaleRepo
		deps.Reader = infra.scaleReader
		deps.ListCache = infra.scaleListCache
		deps.QuestionnaireCatalog = quesApp.NewPublishedQuestionnaireCatalog(infra.questionnaireRepo)
	}
	return deps
}

func (c *Container) buildScaleModule() (*assembler.ScaleModule, error) {
	if _, err := c.ensureSurveyScaleInfra(); err != nil {
		return nil, err
	}
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
