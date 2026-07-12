package container

import (
	"context"
	"fmt"

	actoraccess "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	assessmentapp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	interpretationadmin "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/administration"
	interpretationclinician "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/clinician"
	actormod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/actor"
	evalmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/evaluation"
	reportmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/interpretation"
	ammod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/modelcatalog"
	planmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/plan"
	platformmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/platform"
	statmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/statistics"
	surveymod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/survey"
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
	if err := c.ReportModule.BindOutcomeRepository(c.EvaluationModule.OutcomeRepository()); err != nil {
		return fmt.Errorf("failed to bind interpretation outcome service: %w", err)
	}
	if err := c.ReportModule.BindParticipantAccess(participantInterpretationAccess{access: c.EvaluationModule.TesteeQueryService}); err != nil {
		return fmt.Errorf("failed to bind interpretation participant service: %w", err)
	}
	if err := c.ReportModule.BindAdministrationAccess(administrationInterpretationAccess{access: c.EvaluationModule.AccessQueryService}); err != nil {
		return fmt.Errorf("failed to bind interpretation administration service: %w", err)
	}
	if c.ActorModule == nil {
		return fmt.Errorf("actor module must be installed before binding interpretation clinician service")
	}
	if err := c.ReportModule.BindClinicianAccess(clinicianInterpretationAccess{relations: c.ActorModule.TesteeAccessService, ownership: c.EvaluationModule.TesteeQueryService}); err != nil {
		return fmt.Errorf("failed to bind interpretation clinician service: %w", err)
	}
	return nil
}

type participantInterpretationAccess struct {
	access assessmentapp.TesteeAssessmentQueryService
}

func (a participantInterpretationAccess) AuthorizeOwnAssessment(ctx context.Context, testeeID, assessmentID uint64) error {
	_, err := a.access.GetMine(ctx, testeeID, assessmentID)
	return err
}

type administrationInterpretationAccess struct {
	access assessmentapp.AssessmentAccessQueryService
}

type clinicianInterpretationAccess struct {
	relations actoraccess.TesteeAccessService
	ownership assessmentapp.TesteeAssessmentQueryService
}

func (a clinicianInterpretationAccess) AuthorizeParticipant(ctx context.Context, actor interpretationclinician.Actor, testeeID uint64) error {
	return a.relations.ValidateTesteeAccess(ctx, actor.OrgID, actor.OperatorUserID, testeeID)
}
func (a clinicianInterpretationAccess) AuthorizeParticipantAssessment(ctx context.Context, actor interpretationclinician.Actor, testeeID, assessmentID uint64) error {
	if err := a.AuthorizeParticipant(ctx, actor, testeeID); err != nil {
		return err
	}
	_, err := a.ownership.GetMine(ctx, testeeID, assessmentID)
	return err
}

func (a administrationInterpretationAccess) AuthorizeAssessment(ctx context.Context, actor interpretationadmin.Actor, assessmentID uint64) error {
	_, err := a.access.LoadAccessibleAssessment(ctx, actor.OrgID, actor.OperatorUserID, assessmentID)
	return err
}
func (a administrationInterpretationAccess) ScopeReports(ctx context.Context, actor interpretationadmin.Actor, testeeID uint64) (interpretationadmin.ListScope, error) {
	scope, err := a.access.ScopeTesteeList(ctx, actor.OrgID, actor.OperatorUserID, testeeID)
	if err != nil {
		return interpretationadmin.ListScope{}, err
	}
	return interpretationadmin.ListScope{TesteeID: scope.TesteeID, AccessibleTesteeIDs: scope.AccessibleTesteeIDs, Restricted: scope.RestrictToAccessScope}, nil
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
	if err := statmod.InstallFrom(c); err != nil {
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
