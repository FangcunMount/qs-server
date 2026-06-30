package handler

import (
	"errors"
	"strconv"

	evaluationapp "github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GetMyAssessmentV2 获取我的 v2 测评详情。
// Deprecated: 请优先使用 /api/v1/personality-assessments 或未来的 scale-assessments 专用路由。
// @Summary 获取我的 v2 测评详情
// @Description 根据测评 ID 获取详情，响应使用 model/primary_score/level 投影
// @Tags 测评-V2
// @Produce json
// @Param id path int true "测评ID"
// @Param testee_id query int true "受试者ID"
// @Success 200 {object} core.Response{data=evaluation.AssessmentDetailV2Response}
// @Router /api/v2/assessments/{id} [get]
func (h *EvaluationHandler) GetMyAssessmentV2(c *gin.Context) {
	testeeIDStr := h.GetQueryParam(c, "testee_id")
	if testeeIDStr == "" {
		h.BadRequestResponse(c, "testee_id is required", nil)
		return
	}
	testeeID, err := strconv.ParseUint(testeeIDStr, 10, 64)
	if err != nil {
		h.BadRequestResponse(c, "invalid testee_id format", err)
		return
	}
	assessmentID, err := strconv.ParseUint(h.GetPathParam(c, "id"), 10, 64)
	if err != nil {
		h.BadRequestResponse(c, "invalid assessment id", err)
		return
	}

	result, err := h.queryService.GetMyAssessmentV2(c.Request.Context(), testeeID, assessmentID)
	if err != nil {
		h.InternalErrorResponse(c, "get assessment failed", err)
		return
	}
	if result == nil {
		h.NotFoundResponse(c, "assessment not found", nil)
		return
	}
	h.Success(c, result)
}

// ListMyAssessmentsV2 查询我的 v2 测评列表。
// Deprecated: 请优先使用 /api/v1/personality-assessments 或未来的 scale-assessments 专用路由。
// @Summary 查询我的 v2 测评列表
// @Description 分页查询测评列表，响应使用 model/primary_score/level 投影
// @Tags 测评-V2
// @Produce json
// @Param testee_id query int true "受试者ID"
// @Success 200 {object} core.Response{data=evaluation.ListAssessmentsV2Response}
// @Router /api/v2/assessments [get]
func (h *EvaluationHandler) ListMyAssessmentsV2(c *gin.Context) {
	testeeIDStr := h.GetQueryParam(c, "testee_id")
	if testeeIDStr == "" {
		h.BadRequestResponse(c, "testee_id is required", nil)
		return
	}
	testeeID, err := strconv.ParseUint(testeeIDStr, 10, 64)
	if err != nil {
		h.BadRequestResponse(c, "invalid testee_id format", err)
		return
	}

	var req evaluationapp.ListAssessmentsRequest
	if err := h.BindQuery(c, &req); err != nil {
		return
	}

	result, err := h.queryService.ListMyAssessmentsV2(c.Request.Context(), testeeID, &req)
	if err != nil {
		if errors.Is(err, evaluationapp.ErrInvalidAssessmentKind) {
			h.BadRequestResponse(c, err.Error(), err)
			return
		}
		h.InternalErrorResponse(c, "list assessments failed", err)
		return
	}
	h.Success(c, result)
}

// GetAssessmentReportV2 获取 v2 测评报告。
// Deprecated: 请优先使用 /api/v1/personality-assessments/{id}/report。
// @Summary 获取 v2 测评报告
// @Description 根据测评 ID 获取报告，响应使用 model/primary_score/level 投影。必须传 testee_id 校验归属。
// @Tags 测评-V2
// @Produce json
// @Param id path int true "测评ID"
// @Param testee_id query int true "受试者ID"
// @Success 200 {object} core.Response{data=evaluation.AssessmentReportV2Response}
// @Failure 400 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /api/v2/assessments/{id}/report [get]
func (h *EvaluationHandler) GetAssessmentReportV2(c *gin.Context) {
	testeeIDStr := h.GetQueryParam(c, "testee_id")
	if testeeIDStr == "" {
		h.BadRequestResponse(c, "testee_id is required", nil)
		return
	}
	testeeID, err := strconv.ParseUint(testeeIDStr, 10, 64)
	if err != nil {
		h.BadRequestResponse(c, "invalid testee_id format", err)
		return
	}
	assessmentID, err := strconv.ParseUint(h.GetPathParam(c, "id"), 10, 64)
	if err != nil {
		h.BadRequestResponse(c, "invalid assessment id", err)
		return
	}

	result, err := h.queryService.GetAssessmentReportV2(c.Request.Context(), testeeID, assessmentID)
	if err != nil {
		if status.Code(err) == codes.PermissionDenied {
			h.NotFoundResponse(c, "report not found", nil)
			return
		}
		h.InternalErrorResponse(c, "get report failed", err)
		return
	}
	if result == nil {
		h.NotFoundResponse(c, "report not found", nil)
		return
	}
	h.Success(c, result)
}
