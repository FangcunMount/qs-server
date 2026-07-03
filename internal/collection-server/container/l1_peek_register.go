package container

import (
	"strings"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/catalogl1"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/personalitymodel"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/scale"
	"github.com/gin-gonic/gin"
)

func registerCatalogL1Peek(
	registry *catalogl1.PeekRegistry,
	scaleSvc *scale.QueryService,
	personalitySvc *personalitymodel.QueryService,
	questionnaireSvc *questionnaire.QueryService,
) {
	if registry == nil {
		return
	}
	registry.Register(catalogl1.PeekEntry{
		RouteMatch: func(route string) bool { return strings.HasSuffix(route, "/scales/:code") },
		HasCached: func(c *gin.Context) bool {
			return scaleSvc != nil && scaleSvc.HasCachedDetail(c.Param("code"))
		},
	})
	registry.Register(catalogl1.PeekEntry{
		RouteMatch: func(route string) bool {
			return route == "/api/v1/scales" || strings.HasSuffix(route, "/scales")
		},
		HasCached: func(c *gin.Context) bool {
			if scaleSvc == nil {
				return false
			}
			var req scale.ListScalesRequest
			if err := c.ShouldBindQuery(&req); err != nil {
				return false
			}
			return scaleSvc.HasCachedList(&req)
		},
	})
	registry.Register(catalogl1.PeekEntry{
		RouteMatch: func(route string) bool { return strings.HasSuffix(route, "/scales/hot") },
		HasCached: func(c *gin.Context) bool {
			if scaleSvc == nil {
				return false
			}
			var req scale.ListHotScalesRequest
			if err := c.ShouldBindQuery(&req); err != nil {
				return false
			}
			return scaleSvc.HasCachedHot(&req)
		},
	})
	registry.Register(catalogl1.PeekEntry{
		RouteMatch: func(route string) bool { return strings.HasSuffix(route, "/scales/categories") },
		HasCached: func(c *gin.Context) bool {
			return scaleSvc != nil && scaleSvc.HasCachedCategories()
		},
	})
	registry.Register(catalogl1.PeekEntry{
		RouteMatch: func(route string) bool { return strings.HasSuffix(route, "/personality-models/:code") },
		HasCached: func(c *gin.Context) bool {
			return personalitySvc != nil && personalitySvc.HasCachedDetail(c.Param("code"))
		},
	})
	registry.Register(catalogl1.PeekEntry{
		RouteMatch: func(route string) bool {
			return route == "/api/v1/personality-models" || strings.HasSuffix(route, "/personality-models")
		},
		HasCached: func(c *gin.Context) bool {
			if personalitySvc == nil {
				return false
			}
			var req personalitymodel.ListPersonalityModelsRequest
			if err := c.ShouldBindQuery(&req); err != nil {
				return false
			}
			return personalitySvc.HasCachedList(&req)
		},
	})
	registry.Register(catalogl1.PeekEntry{
		RouteMatch: func(route string) bool { return strings.HasSuffix(route, "/personality-models/categories") },
		HasCached: func(c *gin.Context) bool {
			return personalitySvc != nil && personalitySvc.HasCachedCategories()
		},
	})
	registry.Register(catalogl1.PeekEntry{
		RouteMatch: func(route string) bool { return strings.HasSuffix(route, "/questionnaires/:code") },
		HasCached: func(c *gin.Context) bool {
			return questionnaireSvc != nil && questionnaireSvc.HasCachedDetail(c.Param("code"), c.Query("version"))
		},
	})
}
