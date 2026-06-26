package container

import (
	"fmt"

	actormod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/actor"
	ammod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/assessmentmodel"
	evalmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/evaluation"
	planmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/plan"
	platformmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/platform"
	reportmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/report"
	statmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/statistics"
	surveymod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/survey"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
)

func (c *Container) initSurveyModule() error {
	if err := surveymod.InstallFrom(c); err != nil {
		return fmt.Errorf("failed to initialize survey module: %w", err)
	}
	return nil
}

func (c *Container) initAssessmentModelModule() error {
	if err := ammod.InstallFrom(c); err != nil {
		return fmt.Errorf("failed to initialize assessment model module: %w", err)
	}
	return nil
}

func (c *Container) initActorModule() error {
	if err := actormod.InstallFrom(c); err != nil {
		return fmt.Errorf("failed to initialize actor module: %w", err)
	}
	return nil
}

func (c *Container) initReportModule() error {
	if err := reportmod.InstallFrom(c); err != nil {
		return fmt.Errorf("failed to initialize report module: %w", err)
	}
	return nil
}

func (c *Container) initEvaluationModule() error {
	if err := evalmod.InstallFrom(c); err != nil {
		return fmt.Errorf("failed to initialize evaluation module: %w", err)
	}
	return nil
}

func (c *Container) initPlanModule() error {
	if err := planmod.InstallFrom(c); err != nil {
		return fmt.Errorf("failed to initialize plan module: %w", err)
	}
	return nil
}

func (c *Container) initStatisticsModule() error {
	if err := statmod.InstallFrom(c); err != nil {
		return fmt.Errorf("failed to initialize statistics module: %w", err)
	}
	return nil
}

func (c *Container) initWarmupCoordinator() error {
	if c == nil {
		return nil
	}
	if c.cache != nil {
		c.cache.BindGovernance(newCacheGovernanceAdapter(c).bindings())
	}
	return nil
}

func (c *Container) initPlatformModule() {
	platformmod.InstallFrom(c)
}

func (c *Container) InitQRCodeService(wechatOptions *options.WeChatOptions, ossOptions *options.OSSOptions) error {
	return platformmod.InitQRCodeServiceFrom(c, wechatOptions, ossOptions)
}

func (c *Container) InitMiniProgramTaskNotificationService(wechatOptions *options.WeChatOptions) {
	platformmod.InitMiniProgramNotificationFrom(c, wechatOptions)
}
