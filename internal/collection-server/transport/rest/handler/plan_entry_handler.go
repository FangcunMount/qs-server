package handler

import (
	"errors"
	"net/http"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/planentry"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/testeeaccess"
	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/gin-gonic/gin"
)

type PlanEntryHandler struct {
	*BaseHandler
	service *planentry.Service
}

func NewPlanEntryHandler(service *planentry.Service) *PlanEntryHandler {
	return &PlanEntryHandler{BaseHandler: NewBaseHandler(), service: service}
}

// Resolve returns task content only after login and an active User -> Testee
// relationship check. The task token alone conveys no access.
// @Summary 解析当前用户的计划任务测评入口
// @Tags Plan-Entry
// @Produce json
// @Param task_id path string true "任务ID"
// @Success 200 {object} core.Response{data=planentry.Entry}
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Router /api/v1/plan-task-entries/{task_id} [get]
func (h *PlanEntryHandler) Resolve(c *gin.Context) {
	claims := pkgmiddleware.GetUserClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}
	entry, err := h.service.Resolve(c.Request.Context(), claims.UserID, c.Param("task_id"), c.Param("token"))
	if err != nil {
		switch {
		case errors.Is(err, planentry.ErrInvalidEntry):
			c.JSON(http.StatusNotFound, gin.H{"error": "task entry not found"})
		case errors.Is(err, testeeaccess.ErrAccessDenied):
			c.JSON(http.StatusForbidden, gin.H{"error": "testee access denied"})
		default:
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "task entry temporarily unavailable"})
		}
		return
	}
	h.Success(c, gin.H{"task_id": entry.TaskID, "testee_id": entry.TesteeID, "q": entry.QuestionnaireCode, "questionnaire_version": entry.QuestionnaireVersion, "scale_code": entry.ScaleCode, "model_version": entry.ModelVersion, "entry_status": "active", "expires_at": entry.ExpiresAt, "plan_id": entry.PlanID, "entry_title": entry.Title})
}

// List returns authoritative pending tasks for an actively linked profile.
// @Summary 查询当前用户指定档案的待填写任务
// @Tags Plan-Entry
// @Produce json
// @Param testee_id query string true "档案ID"
// @Success 200 {object} core.Response
// @Failure 403 {object} core.ErrResponse
// @Failure 503 {object} core.ErrResponse
// @Router /api/v1/plan-tasks [get]
func (h *PlanEntryHandler) List(c *gin.Context) {
	claims := pkgmiddleware.GetUserClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}
	entries, err := h.service.List(c.Request.Context(), claims.UserID, c.Query("testee_id"))
	if err != nil {
		if errors.Is(err, testeeaccess.ErrAccessDenied) {
			c.JSON(http.StatusForbidden, gin.H{"error": "testee access denied"})
		} else {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "task list temporarily unavailable"})
		}
		return
	}
	h.Success(c, gin.H{"items": entries})
}

// ResolveLegacy keeps old links under exactly the same authenticated authorization.
// @Summary 兼容旧计划任务入口
// @Tags Plan-Entry
// @Produce json
// @Param task_id path string true "任务ID"
// @Param token path string true "历史入口参数，忽略且不授权"
// @Success 200 {object} core.Response{data=planentry.Entry}
// @Router /api/v1/plan-task-entries/{task_id}/{token} [get]
func (h *PlanEntryHandler) ResolveLegacy(c *gin.Context) { h.Resolve(c) }
