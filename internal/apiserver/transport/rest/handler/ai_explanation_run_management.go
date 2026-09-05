package handler

import (
	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	administration "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/administration"
	appevaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/evaluation"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/gin-gonic/gin"
)

type AIExplanationEvaluationV2PageWire = appevaluation.EvidenceV2Page

type AIExplanationCancelEvaluationV2Request struct {
	ExpectedVersion int64  `json:"expected_version" binding:"required,min=1"`
	Reason          string `json:"reason" binding:"required,max=1000"`
	Discard         bool   `json:"discard"`
}

// ListEvaluationsV2 godoc
// @Summary 分页查询当前机构的 v2 评测 Run
// @Tags AI-Explanation-Administration
// @Produce json
// @Param status query string false "状态" Enums(requested,collecting,blocked,awaiting_review,approved,rejected,canceled)
// @Param cursor query string false "分页游标"
// @Param limit query int false "页大小，默认 20，最大 100"
// @Success 200 {object} core.Response{data=AIExplanationEvaluationV2PageWire}
// @Router /internal/v2/interpretation/ai-explanation/prompt-evaluations [get]
func (h *AIExplanationAdministrationHandler) ListEvaluationsV2(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	limit, err := administrationCatalogLimit(c, 20, 100)
	if err != nil {
		h.Error(c, err)
		return
	}
	var status *domainevaluation.EvidenceStatus
	if raw := c.Query("status"); raw != "" {
		value := domainevaluation.EvidenceStatus(raw)
		status = &value
	}
	page, err := h.service.ListEvaluationsV2(c.Request.Context(), actor, administration.EvaluationV2ListQuery{Status: status, Cursor: c.Query("cursor"), Limit: limit})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, page)
}

// CancelEvaluationV2 godoc
// @Summary 取消未完成的 Run 或废弃待审核的 Run，保留审计证据
// @Tags AI-Explanation-Administration
// @Accept json
// @Produce json
// @Param run_id path string true "Run ID"
// @Param request body AIExplanationCancelEvaluationV2Request true "版本和操作理由"
// @Success 200 {object} core.Response{data=AIExplanationEvaluationV2Wire}
// @Failure 409 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-explanation/prompt-evaluations/{run_id}/cancel [post]
func (h *AIExplanationAdministrationHandler) CancelEvaluationV2(c *gin.Context) {
	actor, runID, ok := h.actorAndRunID(c)
	if !ok {
		return
	}
	var request AIExplanationCancelEvaluationV2Request
	if err := c.ShouldBindJSON(&request); err != nil {
		h.Error(c, cberrors.WithCode(code.ErrInvalidArgument, "invalid cancellation request: %v", err))
		return
	}
	value, err := h.service.CancelEvaluationV2(c.Request.Context(), actor, runID, administration.CancelEvaluationV2Command{ExpectedVersion: request.ExpectedVersion, Reason: request.Reason, Discard: request.Discard})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, evaluationV2Wire(value))
}
