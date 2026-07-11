package container

import (
	"fmt"

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
	return nil
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
