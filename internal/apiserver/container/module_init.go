package container

import (
	"context"
	"fmt"
	bridge "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	actormysql "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"

	actoraccess "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	actortestee "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	evaluationoperator "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/operator"
	evaluationtestee "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/testee"
	interpretationadmin "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/administration"
	interpretationparticipant "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/participant"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reportprojection"
	modelcatalogHotRank "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/hotrank"
	actormod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/actor"
	evalmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/evaluation"
	reportmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/interpretation"
	ammod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/modelcatalog"
	planmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/plan"
	platformmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/platform"
	statmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/statistics"
	surveymod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/survey"
	interpretationpolicy "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
)

// initSurveyModule 初始化调查模块
func (c *Container) initSurveyModule() error {
	if err := surveymod.InstallFrom(c); err != nil {
		return fmt.Errorf("failed to initialize survey module: %w", err)
	}
	return nil
}

// initModelCatalogModule 初始化模型目录模块
func (c *Container) initModelCatalogModule() error {
	if err := ammod.InstallFrom(c); err != nil {
		return fmt.Errorf("failed to initialize model catalog module: %w", err)
	}
	if c.eventSubsystem != nil && c.AssessmentModelModule != nil && c.AssessmentModelModule.HotRank != nil {
		if err := c.eventSubsystem.RegisterConsumer("modelcatalog.hot_rank_projection", modelcatalogHotRank.NewEventConsumer(c.AssessmentModelModule.HotRank.Projection)); err != nil {
			return fmt.Errorf("register modelcatalog hot-rank event consumer: %w", err)
		}
	}
	return nil
}

// initActorModule 初始化演员模块
func (c *Container) initActorModule() error {
	if err := actormod.InstallFrom(c); err != nil {
		return fmt.Errorf("failed to initialize actor module: %w", err)
	}
	return nil
}

// initReportModule 初始化报告模块
func (c *Container) initReportModule() error {
	if err := reportmod.InstallFrom(c); err != nil {
		return fmt.Errorf("failed to initialize report module: %w", err)
	}
	return nil
}

// initEvaluationModule 初始化评估模块
func (c *Container) initEvaluationModule() error {
	if err := evalmod.InstallFrom(c); err != nil {
		return fmt.Errorf("failed to initialize evaluation module: %w", err)
	}
	if c.ReportModule == nil || c.EvaluationModule == nil {
		return fmt.Errorf("evaluation and interpretation modules must be installed before binding outcome reporting")
	}
	if c.ActorModule == nil {
		return fmt.Errorf("actor module must be installed before binding interpretation access")
	}
	if err := c.ReportModule.BindOutcomeRepository(c.EvaluationModule.OutcomeRepository()); err != nil {
		return fmt.Errorf("failed to bind interpretation outcome service: %w", err)
	}
	c.ReportModule.BindReportProjection(reportprojection.Mapper{})
	currentAIAccess := &bridge.CurrentAccess{Testees: c.ActorModule.ReadModel, Assessments: c.EvaluationModule.TesteeService}
	if links := c.ProfileLinkService(); links != nil {
		currentAIAccess.Links = links
	}
	c.ReportModule.BindCurrentAIAccess(currentAIAccess)
	if err := c.ReportModule.BindParticipantAccess(participantInterpretationAccess{testees: actortestee.NewSelfServiceQueryServiceWithAssessmentSummary(c.ActorModule.ReadModel, nil), assessments: c.EvaluationModule.TesteeService}); err != nil {
		return fmt.Errorf("failed to bind interpretation participant service: %w", err)
	}
	if err := c.ReportModule.BindAdministrationAccess(administrationInterpretationAccess{
		access: c.EvaluationModule.OperatorQuery,
		actors: c.ActorModule.TesteeAccessService,
	}); err != nil {
		return fmt.Errorf("failed to bind interpretation administration service: %w", err)
	}

	return nil
}

type participantInterpretationAccess struct {
	testees     actortestee.TesteeQueryService
	assessments evaluationtestee.Service
}

func (a participantInterpretationAccess) AuthorizeParticipant(ctx context.Context, actor interpretationparticipant.Actor) error {
	if a.testees == nil {
		return fmt.Errorf("participant testee query service is not configured")
	}
	testee, err := a.testees.GetByID(ctx, actor.TesteeID)
	if err != nil {
		return err
	}
	if testee == nil || testee.ID != actor.TesteeID {
		return fmt.Errorf("participant testee identity does not exist")
	}
	return nil
}

func (a participantInterpretationAccess) AuthorizeOwnAssessment(ctx context.Context, testeeID, assessmentID uint64) error {
	if err := a.AuthorizeParticipant(ctx, interpretationparticipant.Actor{TesteeID: testeeID}); err != nil {
		return err
	}
	if a.assessments == nil {
		return fmt.Errorf("participant assessment access service is not configured")
	}
	return a.assessments.AuthorizeAssessment(ctx, evaluationtestee.Actor{TesteeID: testeeID}, assessmentID)
}

type administrationInterpretationAccess struct {
	access evaluationoperator.QueryService
	actors actoraccess.TesteeAccessService
}

const administrationDecisionSource = "actor.access.StoreScopeAccess"

func (a administrationInterpretationAccess) AuthorizeAssessment(ctx context.Context, actor interpretationadmin.Actor, assessmentID uint64) (interpretationadmin.ReportAccessDecision, error) {
	checker, ok := a.access.(evaluationoperator.AssessmentPermissionAuthorizer)
	if !ok {
		return interpretationadmin.ReportAccessDecision{}, fmt.Errorf("report assessment scope checker is not configured")
	}
	if err := checker.AuthorizeAssessmentResource(ctx, evaluationoperator.Actor{OrgID: actor.OrgID, OperatorUserID: actor.OperatorUserID}, assessmentID, "qs:evaluation:collection:reports", "read"); err != nil {
		return interpretationadmin.ReportAccessDecision{}, err
	}
	return a.decide(ctx, actor)
}
func (a administrationInterpretationAccess) ScopeReports(ctx context.Context, actor interpretationadmin.Actor, testeeID uint64) (interpretationadmin.ListScope, error) {
	checker, ok := a.actors.(actoraccess.StoreScopeAccess)
	if !ok {
		return interpretationadmin.ListScope{}, fmt.Errorf("report store scope checker is not configured")
	}
	var ids []uint64
	if testeeID != 0 {
		if err := checker.ValidateTesteeStoreAccess(ctx, actor.OrgID, actor.OperatorUserID, testeeID, "qs:evaluation:collection:reports", "list"); err != nil {
			return interpretationadmin.ListScope{}, err
		}
		ids = []uint64{testeeID}
	} else {
		var err error
		ids, err = checker.ListStoreScopedTesteeIDs(ctx, actor.OrgID, actor.OperatorUserID, "qs:evaluation:collection:reports", "list")
		if err != nil {
			return interpretationadmin.ListScope{}, err
		}
	}
	decision, err := a.decide(ctx, actor)
	if err != nil {
		return interpretationadmin.ListScope{}, err
	}
	return interpretationadmin.ListScope{OrgID: actor.OrgID, TesteeID: testeeID, RestrictToStoreScope: true, StoreScopedTesteeIDs: ids, Audience: decision.Audience, IsAdmin: decision.IsAdmin, DecisionSource: decision.DecisionSource}, nil
}
func (a administrationInterpretationAccess) decide(ctx context.Context, actor interpretationadmin.Actor) (interpretationadmin.ReportAccessDecision, error) {
	snapshot, ok := appauthz.FromContext(ctx)
	if !ok {
		return interpretationadmin.ReportAccessDecision{}, fmt.Errorf("report authorization snapshot required")
	}
	if snapshot.IsQSAdmin() {
		return interpretationadmin.ReportAccessDecision{Audience: interpretationpolicy.AudienceAdmin, IsAdmin: true, DecisionSource: administrationDecisionSource}, nil
	}
	return interpretationadmin.ReportAccessDecision{Audience: interpretationpolicy.AudienceOperator, Restricted: true, DecisionSource: administrationDecisionSource}, nil
}

// initPlanModule 初始化计划模块
func (c *Container) initPlanModule() error {
	if err := planmod.InstallFrom(c); err != nil {
		return fmt.Errorf("failed to initialize plan module: %w", err)
	}
	return nil
}

// initStatisticsModule 初始化统计模块
func (c *Container) initStatisticsModule() error {
	reader := actormysql.NewReadModel(c.MySQLDB())
	access := actoraccess.NewTesteeAccessService(reader, reader).(actoraccess.StoreScopeAccess)
	if err := statmod.InstallFrom(c, access); err != nil {
		return fmt.Errorf("failed to initialize statistics module: %w", err)
	}
	return nil
}

// initWarmupCoordinator 初始化预热协调器
func (c *Container) initWarmupCoordinator() error {
	if c == nil {
		return nil
	}
	if c.cache != nil {
		c.cache.BindGovernance(newCacheGovernanceAdapter(c).bindings())
	}
	return nil
}

// initPlatformModule 初始化平台模块
func (c *Container) initPlatformModule() {
	platformmod.InstallFrom(c)
}

// InitQRCodeService 初始化二维码服务
func (c *Container) InitQRCodeService(wechatOptions *options.WeChatOptions, ossOptions *options.OSSOptions) error {
	return platformmod.InitQRCodeServiceFrom(c, wechatOptions, ossOptions)
}

// InitMiniProgramTaskNotificationService 初始化小程序任务通知服务
func (c *Container) InitMiniProgramTaskNotificationService(wechatOptions *options.WeChatOptions) {
	platformmod.InitMiniProgramNotificationFrom(c, wechatOptions)
}
