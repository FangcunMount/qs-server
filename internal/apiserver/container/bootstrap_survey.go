package container

import (
	"fmt"

	scaleApp "github.com/FangcunMount/qs-server/internal/apiserver/application/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
)

func (c *Container) resolveIdentityService() *iam.IdentityService {
	if c == nil || c.IAMModule == nil || !c.IAMModule.IsEnabled() {
		return nil
	}
	return c.IAMModule.IdentityService()
}

func (c *Container) buildSurveyModuleDeps() assembler.SurveyModuleDeps {
	var infra *surveyScaleInfra
	if c != nil {
		infra = c.surveyScaleInfra
	}
	deps := assembler.SurveyModuleDeps{
		MongoDB:          c.mongoDB,
		EventPublisher:   c.eventPublisher,
		RankRedisClient:  c.CacheClient(cacheplane.FamilyRank),
		RankCacheBuilder: c.CacheBuilder(cacheplane.FamilyRank),
		IdentityService:  c.resolveIdentityService(),
		HotsetRecorder:   c.hotsetRecorder(),
		TopicResolver:    c.eventCatalog,
		ScaleSyncer:      scaleApp.NewQuestionnaireBindingSyncer(nil),
	}
	if infra != nil {
		deps.ScaleSyncer = scaleApp.NewQuestionnaireBindingSyncer(infra.scaleRepo)
		deps.QuestionnaireRepo = infra.questionnaireRepo
		deps.QuestionnaireReader = infra.questionnaireReader
		deps.AnswerSheetRepo = infra.answerSheetRepo
		deps.AnswerSheetReader = infra.answerSheetReader
	}
	return deps
}

func (c *Container) buildSurveyModule() (*assembler.SurveyModule, error) {
	if _, err := c.ensureSurveyScaleInfra(); err != nil {
		return nil, err
	}
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
