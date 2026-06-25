package container

import (
	"fmt"

	quesApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	mongoRuleset "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/ruleset"
	rulesetInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
)

func (c *Container) buildScaleModuleDeps() assembler.ScaleModuleDeps {
	if c == nil {
		return assembler.ScaleModuleDeps{}
	}
	infra := c.surveyScaleInfra

	deps := assembler.ScaleModuleDeps{
		EventPublisher:      c.eventPublisher,
		RankRedisClient:     c.CacheClient(cacheplane.FamilyRank),
		RankCacheBuilder:    c.CacheBuilder(cacheplane.FamilyRank),
		IdentityService:     c.resolveIdentityService(),
		HotsetRecorder:      c.hotsetRecorder(),
		CacheSignalNotifier: c.CacheSignalNotifier(),
	}
	if infra != nil {
		deps.Repo = infra.scaleRepo
		deps.Reader = infra.scaleReader
		deps.ListCache = infra.scaleListCache
		deps.HotListCache = infra.scaleHotListCache
		deps.QuestionnaireCatalog = quesApp.NewPublishedQuestionnaireCatalog(infra.questionnaireRepo)
	}
	if c.mongoDB != nil {
		mongoOpts := mongoBase.BaseRepositoryOptions{Limiter: c.backpressure.Mongo}
		writer := mongoRuleset.NewRepository(c.mongoDB, mongoOpts)
		deps.RuleSetPublisher = rulesetInfra.NewScaleRuleSetPublisher(writer)
	}
	if c.SurveyModule != nil && c.SurveyModule.Questionnaire != nil {
		deps.QuestionnairePublisher = c.SurveyModule.Questionnaire.LifecycleService
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
