package container

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
)

func (c *Container) buildPlanModuleDeps() assembler.PlanModuleDeps {
	var infra *surveyScaleInfra
	if c != nil {
		infra = c.surveyScaleInfra
	}
	var scaleRepo scale.Repository
	if infra != nil {
		scaleRepo = infra.scaleRepo
	}

	return assembler.PlanModuleDeps{
		MySQLDB:        c.mysqlDB,
		EventPublisher: c.eventPublisher,
		ScaleRepo:      scaleRepo,
		RedisClient:    c.CacheClient(cacheplane.FamilyObject),
		CacheBuilder:   c.CacheBuilder(cacheplane.FamilyObject),
		PlanPolicy:     c.CachePolicy(cachepolicy.PolicyPlan),
		EntryBaseURL:   c.planEntryURL,
		Observer:       c.cacheObserver(),
		MySQLLimiter:   c.backpressure.MySQL,
		TesteeAccess:   c.actorTesteeAccessService(),
	}
}

func (c *Container) buildPlanModule() (*assembler.PlanModule, error) {
	return assembler.NewPlanModule(c.buildPlanModuleDeps())
}

// initPlanModule 初始化 Plan 模块。
func (c *Container) initPlanModule() error {
	planModule, err := c.buildPlanModule()
	if err != nil {
		return fmt.Errorf("failed to initialize plan module: %w", err)
	}

	c.PlanModule = planModule
	c.registerModule("plan", planModule)

	c.printf("📦 Plan module initialized\n")
	return nil
}
