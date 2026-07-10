package catalogpeek

import (
	"strings"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/typologymodel"
	"github.com/gin-gonic/gin"
)

// RegisterCatalogL1 注册 catalog 目录读的 L1 peek 规则。
func RegisterCatalogL1(
	registry *Registry,
	personalitySvc *typologymodel.QueryService,
	questionnaireSvc *questionnaire.QueryService,
) {
	if registry == nil {
		return
	}
	registry.Register(Entry{
		RouteMatch: func(route string) bool { return strings.HasSuffix(route, "/typology-models/:code") },
		HasCached: func(c *gin.Context) bool {
			return personalitySvc != nil && personalitySvc.HasCachedDetail(c.Param("code"))
		},
	})
	registry.Register(Entry{
		RouteMatch: func(route string) bool {
			return route == "/api/v1/typology-models" || strings.HasSuffix(route, "/typology-models")
		},
		HasCached: func(c *gin.Context) bool {
			if personalitySvc == nil {
				return false
			}
			var req typologymodel.ListTypologyModelsRequest
			if err := c.ShouldBindQuery(&req); err != nil {
				return false
			}
			return personalitySvc.HasCachedList(&req)
		},
	})
	registry.Register(Entry{
		RouteMatch: func(route string) bool { return strings.HasSuffix(route, "/typology-models/categories") },
		HasCached: func(c *gin.Context) bool {
			return personalitySvc != nil && personalitySvc.HasCachedCategories()
		},
	})
	registry.Register(Entry{
		RouteMatch: func(route string) bool { return strings.HasSuffix(route, "/questionnaires/:code") },
		HasCached: func(c *gin.Context) bool {
			return questionnaireSvc != nil && questionnaireSvc.HasCachedDetail(c.Param("code"), c.Query("version"))
		},
	})
}
