package container

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
)

func (c *Container) buildPlanModuleInitializeParams() []interface{} {
	var scaleRepo scale.Repository
	if c != nil && c.ScaleModule != nil {
		scaleRepo = c.ScaleModule.Repo
	}

	return []interface{}{
		c.mysqlDB,
		c.eventPublisher,
		scaleRepo,
		c.CacheClient(redisplane.FamilyObject),
		c.CacheBuilder(redisplane.FamilyObject),
		c.CachePolicy(cachepolicy.PolicyPlan),
		c.planEntryURL,
		c.cacheObserver(),
	}
}

// initPlanModule 初始化 Plan 模块。
func (c *Container) initPlanModule() error {
	planModule := assembler.NewPlanModule()
	if err := planModule.Initialize(c.buildPlanModuleInitializeParams()...); err != nil {
		return fmt.Errorf("failed to initialize plan module: %w", err)
	}

	c.PlanModule = planModule
	c.registerModule("plan", planModule)

	c.printf("📦 Plan module initialized\n")
	return nil
}
