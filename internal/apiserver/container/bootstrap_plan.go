package container

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
)

func (c *Container) buildPlanModuleDeps() assembler.PlanModuleDeps {
	var scaleRepo scale.Repository
	if c != nil && c.ScaleModule != nil {
		scaleRepo = c.ScaleModule.Repo
	}

	return assembler.PlanModuleDeps{
		MySQLDB:        c.mysqlDB,
		EventPublisher: c.eventPublisher,
		ScaleRepo:      scaleRepo,
		RedisClient:    c.CacheClient(redisplane.FamilyObject),
		CacheBuilder:   c.CacheBuilder(redisplane.FamilyObject),
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
