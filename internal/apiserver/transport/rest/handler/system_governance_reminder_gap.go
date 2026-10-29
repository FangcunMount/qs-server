package handler

import (
	"net/http"

	systemgov "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	"github.com/gin-gonic/gin"
)

// AuthorizeReminderGapRecovery authorizes only a missing original recipient batch.
// @Summary 系统治理-授权恢复缺收件批次的原任务提醒
// @Description 仅 qs:admin。transaction 模式复用原通知业务服务和账本，需恢复及提醒开关；inspect 只读，resume 仅续办原请求。历史默认模式仍受旧发布表装配约束。
// @Tags System-Governance
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param request body systemgovernance.ReminderGapRecoveryRequest true "原Task、事件、版本、固定截点与审核依据"
// @Param mode query string false "当前事务操作模式" Enums(apply,inspect,resume)
// @Success 200 {object} core.Response{data=object} "inspect 返回 ReminderGapInspection；apply/resume 返回 ActionRunResult"
// @Failure 400 {object} core.ErrResponse
// @Router /internal/v1/system-governance/actions/reminder-gap-recoveries [post]
func (h *SystemGovernanceHandler) AuthorizeReminderGapRecovery(c *gin.Context) {
	h.reminderGapRecovery(c, false)
}

// ResolveReminderGapRecovery reads the original receipt, including with writes disabled.
// @Summary 系统治理-核对原任务提醒恢复回执
// @Description 只读匹配同机构、操作者和完整原输入；回执缺失不授权另建请求。
// @Tags System-Governance
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param request body systemgovernance.ReminderGapRecoveryRequest true "原操作完整输入"
// @Success 200 {object} core.Response{data=systemgovernance.ActionRunResult}
// @Failure 404 {object} core.ErrResponse
// @Router /internal/v1/system-governance/actions/reminder-gap-recoveries/resolve [post]
func (h *SystemGovernanceHandler) ResolveReminderGapRecovery(c *gin.Context) {
	h.reminderGapRecovery(c, true)
}

func (h *SystemGovernanceHandler) reminderGapRecovery(c *gin.Context, resolve bool) {
	recovery, ok := h.facade.(systemgov.ReminderGapRecovery)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "reminder recovery unavailable"})
		return
	}
	orgID, actorID, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	var req systemgov.ReminderGapRecoveryRequest
	if !h.bindJSON(c, &req) {
		return
	}
	if !resolve {
		switch c.Query("mode") {
		case "inspect", "resume":
			current, ok := h.facade.(systemgov.CurrentReminderGapRecovery)
			if !ok {
				c.JSON(http.StatusServiceUnavailable, gin.H{"message": "current reminder recovery unavailable"})
				return
			}
			if c.Query("mode") == "inspect" {
				inspection, err := current.InspectReminderGap(c.Request.Context(), orgID, uint64(actorID), req)
				if err != nil {
					h.Error(c, err)
					return
				}
				h.Success(c, inspection)
				return
			}
			result, err := current.ResumeReminderGap(c.Request.Context(), orgID, uint64(actorID), req)
			if err != nil {
				h.Error(c, err)
				return
			}
			h.Success(c, result)
			return
		case "", "apply":
		default:
			c.JSON(http.StatusBadRequest, gin.H{"message": "unsupported reminder recovery mode"})
			return
		}
	}
	var result *systemgov.ActionRunResult
	if resolve {
		var found bool
		result, found, err = recovery.ResolveReminderGap(c.Request.Context(), orgID, uint64(actorID), req)
		if err == nil && !found {
			c.JSON(http.StatusNotFound, gin.H{"message": "original reminder recovery receipt not found"})
			return
		}
	} else {
		result, err = recovery.AuthorizeReminderGap(c.Request.Context(), orgID, uint64(actorID), req)
	}
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, result)
}
