package catalogcache

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/typologymodel"
)

const warmupTimeout = 30 * time.Second

type CatalogWarmupPlan struct {
	PersonalityList       bool
	PersonalityCategories bool
}

func DefaultCatalogWarmupPlan() CatalogWarmupPlan {
	return CatalogWarmupPlan{
		PersonalityList:       true,
		PersonalityCategories: true,
	}
}

// WarmCatalogOnStartup 预热压测高频 catalog 路径，减少开跑 L1 miss 尖刺。
func WarmCatalogOnStartup(
	personalitySvc *typologymodel.QueryService,
) {
	WarmCatalogOnStartupWithPlan(personalitySvc, DefaultCatalogWarmupPlan())
}

func WarmCatalogOnStartupWithPlan(
	personalitySvc *typologymodel.QueryService,
	plan CatalogWarmupPlan,
) {
	if personalitySvc == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), warmupTimeout)
		defer cancel()

		if personalitySvc != nil {
			if plan.PersonalityList {
				if _, err := personalitySvc.List(ctx, &typologymodel.ListTypologyModelsRequest{
					Page: 1, PageSize: 20,
				}); err != nil {
					log.Warnf("catalog warmup: personality list: %v", err)
				}
			}
			if plan.PersonalityCategories {
				if _, err := personalitySvc.GetCategories(ctx); err != nil {
					log.Warnf("catalog warmup: personality categories: %v", err)
				}
			}
		}
		log.Info("catalog L1 warmup finished")
	}()
}
