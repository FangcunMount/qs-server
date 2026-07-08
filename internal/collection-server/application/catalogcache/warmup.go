package catalogcache

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/scale"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/typologymodel"
)

const warmupTimeout = 30 * time.Second

type CatalogWarmupPlan struct {
	ScaleList             bool
	ScaleHot              bool
	ScaleCategories       bool
	PersonalityList       bool
	PersonalityCategories bool
}

func DefaultCatalogWarmupPlan() CatalogWarmupPlan {
	return CatalogWarmupPlan{
		ScaleList:             true,
		ScaleHot:              true,
		ScaleCategories:       true,
		PersonalityList:       true,
		PersonalityCategories: true,
	}
}

// WarmCatalogOnStartup 预热压测高频 catalog 路径，减少开跑 L1 miss 尖刺。
func WarmCatalogOnStartup(
	scaleSvc *scale.QueryService,
	personalitySvc *typologymodel.QueryService,
) {
	WarmCatalogOnStartupWithPlan(scaleSvc, personalitySvc, DefaultCatalogWarmupPlan())
}

func WarmCatalogOnStartupWithPlan(
	scaleSvc *scale.QueryService,
	personalitySvc *typologymodel.QueryService,
	plan CatalogWarmupPlan,
) {
	if scaleSvc == nil && personalitySvc == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), warmupTimeout)
		defer cancel()

		if scaleSvc != nil {
			if plan.ScaleList {
				if _, err := scaleSvc.List(ctx, &scale.ListScalesRequest{
					Page: 1, PageSize: 20, Status: "published",
				}); err != nil {
					log.Warnf("catalog warmup: scale list: %v", err)
				}
			}
			if plan.ScaleHot {
				if _, err := scaleSvc.ListHot(ctx, &scale.ListHotScalesRequest{Limit: 5, WindowDays: 30}); err != nil {
					log.Warnf("catalog warmup: scale hot: %v", err)
				}
			}
			if plan.ScaleCategories {
				if _, err := scaleSvc.GetCategories(ctx); err != nil {
					log.Warnf("catalog warmup: scale categories: %v", err)
				}
			}
		}
		if personalitySvc != nil {
			if plan.PersonalityList {
				if _, err := personalitySvc.List(ctx, &typologymodel.ListPersonalityModelsRequest{
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
