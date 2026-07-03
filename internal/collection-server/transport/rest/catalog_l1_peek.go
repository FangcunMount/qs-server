package rest

import (
	"strings"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/personalitymodel"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/scale"
	"github.com/gin-gonic/gin"
)

// catalogL1Peek 判断 catalog 请求是否可由进程内 L1 直接响应（无需占用并发槽）。
func (r *Router) catalogL1Peek(c *gin.Context) bool {
	if r == nil || r.container == nil || c == nil || c.Request == nil {
		return false
	}
	if c.Request.Method != "GET" {
		return false
	}

	route := c.FullPath()
	if route == "" {
		route = c.Request.URL.Path
	}

	switch {
	case strings.HasSuffix(route, "/scales/:code"):
		svc := r.container.ScaleQueryService()
		return svc != nil && svc.HasCachedDetail(c.Param("code"))
	case route == "/api/v1/scales" || strings.HasSuffix(route, "/scales"):
		svc := r.container.ScaleQueryService()
		if svc == nil {
			return false
		}
		var req scale.ListScalesRequest
		if err := c.ShouldBindQuery(&req); err != nil {
			return false
		}
		return svc.HasCachedList(&req)
	case strings.HasSuffix(route, "/scales/hot"):
		svc := r.container.ScaleQueryService()
		if svc == nil {
			return false
		}
		var req scale.ListHotScalesRequest
		if err := c.ShouldBindQuery(&req); err != nil {
			return false
		}
		return svc.HasCachedHot(&req)
	case strings.HasSuffix(route, "/scales/categories"):
		svc := r.container.ScaleQueryService()
		return svc != nil && svc.HasCachedCategories()
	case strings.HasSuffix(route, "/personality-models/:code"):
		svc := r.container.PersonalityModelQueryService()
		return svc != nil && svc.HasCachedDetail(c.Param("code"))
	case route == "/api/v1/personality-models" || strings.HasSuffix(route, "/personality-models"):
		svc := r.container.PersonalityModelQueryService()
		if svc == nil {
			return false
		}
		var req personalitymodel.ListPersonalityModelsRequest
		if err := c.ShouldBindQuery(&req); err != nil {
			return false
		}
		return svc.HasCachedList(&req)
	case strings.HasSuffix(route, "/personality-models/categories"):
		svc := r.container.PersonalityModelQueryService()
		return svc != nil && svc.HasCachedCategories()
	case strings.HasSuffix(route, "/questionnaires/:code"):
		svc := r.container.QuestionnaireQueryService()
		return svc != nil && svc.HasCachedDetail(c.Param("code"), c.Query("version"))
	default:
		return false
	}
}
