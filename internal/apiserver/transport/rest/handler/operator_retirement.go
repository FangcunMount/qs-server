package handler

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operatorretirement"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operatorretirement"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
)

type OperatorRetirementHandler struct {
	*BaseHandler
	service *app.Service
}

func NewOperatorRetirementHandler(service *app.Service) *OperatorRetirementHandler {
	return &OperatorRetirementHandler{NewBaseHandler(), service}
}

type RetireOperatorRequest struct {
	ExpectedVersion uint32 `json:"expected_version" binding:"required"`
	RequestID       string `json:"request_id" binding:"required,max=64"`
	Reason          string `json:"reason" binding:"required,max=500"`
}
type OperatorRetirementResponse struct {
	OperatorID      string `json:"operator_id"`
	RequestID       string `json:"request_id"`
	ExpectedVersion uint32 `json:"expected_version"`
	Reason          string `json:"reason"`
	Stage           string `json:"stage"`
	PolicyVersion   int64  `json:"policy_version"`
	NeedsAttention  bool   `json:"needs_attention"`
}

func retirementResponse(task *domain.Task) OperatorRetirementResponse {
	return OperatorRetirementResponse{
		OperatorID: strconv.FormatUint(task.OperatorID, 10), RequestID: task.RequestID, ExpectedVersion: task.ExpectedVersion, Reason: task.Reason,
		Stage: string(task.Stage), PolicyVersion: task.PolicyVersion, NeedsAttention: task.LastError != "",
	}
}
func (h *OperatorRetirementHandler) scope(c *gin.Context) (int64, int64, uint64, bool) {
	org, user, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return 0, 0, 0, false
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid operator ID"))
		return 0, 0, 0, false
	}
	if h.service == nil {
		h.Error(c, errors.WithCode(code.ErrInternalServerError, "operator retirement unavailable"))
		return 0, 0, 0, false
	}
	return org, user, id, true
}

// Retire closes operator admission and starts/resumes durable authorization retirement.
// @Summary 退出运营操作人
// @Tags Operators
// @Accept json
// @Produce json
// @Param id path string true "Operator ID"
// @Param request body RetireOperatorRequest true "退出请求"
// @Success 200 {object} OperatorRetirementResponse
// @Success 202 {object} OperatorRetirementResponse
// @Router /api/v1/operators/{id} [delete]
func (h *OperatorRetirementHandler) Retire(c *gin.Context) {
	org, user, id, ok := h.scope(c)
	if !ok {
		return
	}
	var req RetireOperatorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid operator retirement request"))
		return
	}
	task, err := h.service.Execute(c.Request.Context(), app.Command{OrgID: org, ActorID: user, OperatorID: id, ExpectedVersion: req.ExpectedVersion, RequestID: req.RequestID, Reason: req.Reason})
	if err != nil && (task == nil || errors.IsCode(err, code.ErrConflict) || errors.IsCode(err, code.ErrPermissionDenied) || errors.IsCode(err, code.ErrInvalidArgument)) {
		h.Error(c, err)
		return
	}
	if task == nil {
		h.Error(c, errors.WithCode(code.ErrInternalServerError, "missing retirement result"))
		return
	}
	if task.Stage != domain.Completed {
		c.JSON(http.StatusAccepted, gin.H{"code": 0, "message": "运营人员已停用，退出尚未完成，请查询任务状态后使用原请求重试", "data": retirementResponse(task)})
		return
	}
	h.Success(c, retirementResponse(task))
}

// Status reports durable progress, including completed soft-deleted operators.
// @Summary 查询运营操作人退出状态
// @Tags Operators
// @Produce json
// @Param id path string true "Operator ID"
// @Success 200 {object} OperatorRetirementResponse
// @Router /api/v1/operators/{id}/retirement [get]
func (h *OperatorRetirementHandler) Status(c *gin.Context) {
	org, user, id, ok := h.scope(c)
	if !ok {
		return
	}
	task, err := h.service.Status(c.Request.Context(), org, user, id)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, retirementResponse(task))
}
