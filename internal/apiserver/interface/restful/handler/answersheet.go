package handler

import (
	"strconv"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/response"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/gin-gonic/gin"
)

// Note: C端提交相关的 API (Submit, GetMyAnswerSheet, ListMyAnswerSheets)
// 由 gRPC 服务处理，不在此 RESTful Handler 中实现

// AnswerSheetHandler 答卷处理器
// RESTful Handler 只处理 B端管理的 API
// C端提交相关的 API 由 gRPC 服务处理
type AnswerSheetHandler struct {
	BaseHandler
	managementService answersheet.AnswerSheetManagementService
}

// NewAnswerSheetHandler 创建答卷处理器
func NewAnswerSheetHandler(
	managementService answersheet.AnswerSheetManagementService,
) *AnswerSheetHandler {
	return &AnswerSheetHandler{
		managementService: managementService,
	}
}

// ============= Management API (B端管理) =============

// GetByID 根据ID获取答卷详情
// @Summary 获取答卷详情
// @Description 管理员查看答卷的完整信息
// @Tags AnswerSheet-Management
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path int true "答卷ID"
// @Success 200 {object} handler.Response{data=response.AnswerSheetResponse}
// @Router /api/v1/admin/answersheets/{id} [get]
func (h *AnswerSheetHandler) GetByID(c *gin.Context) {
	answerSheetID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.ErrorResponse(c, errors.WithCode(code.ErrAnswerSheetInvalid, "无效的答卷ID"))
		return
	}

	result, err := h.managementService.GetByID(c.Request.Context(), answerSheetID)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewAnswerSheetResponse(result))
}

// List 查询答卷列表
// @Summary 查询答卷列表
// @Description 管理员查询答卷列表，支持多维度筛选
// @Tags AnswerSheet-Management
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(10)
// @Param questionnaire_code query string false "问卷编码"
// @Param filler_id query int false "填写人ID"
// @Param start_time query string false "开始时间"
// @Param end_time query string false "结束时间"
// @Success 200 {object} handler.Response{data=response.AnswerSheetListResponse}
// @Router /api/v1/admin/answersheets [get]
func (h *AnswerSheetHandler) List(c *gin.Context) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page <= 0 {
		h.ErrorResponse(c, errors.WithCode(code.ErrAnswerSheetInvalid, "页码必须为正整数"))
		return
	}

	pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	if err != nil || pageSize <= 0 || pageSize > 100 {
		h.ErrorResponse(c, errors.WithCode(code.ErrAnswerSheetInvalid, "每页数量必须为1-100的整数"))
		return
	}

	var fillerID *uint64
	if fillerIDStr := c.Query("filler_id"); fillerIDStr != "" {
		parsed, err := strconv.ParseUint(fillerIDStr, 10, 64)
		if err == nil {
			fillerID = &parsed
		}
	}

	var startTime *time.Time
	if startTimeStr := c.Query("start_time"); startTimeStr != "" {
		if t, err := time.Parse("2006-01-02", startTimeStr); err == nil {
			startTime = &t
		}
	}

	var endTime *time.Time
	if endTimeStr := c.Query("end_time"); endTimeStr != "" {
		if t, err := time.Parse("2006-01-02", endTimeStr); err == nil {
			endTime = &t
		}
	}

	dto := answersheet.ListAnswerSheetsDTO{
		Page:              page,
		PageSize:          pageSize,
		QuestionnaireCode: c.Query("questionnaire_code"),
		FillerID:          fillerID,
		StartTime:         startTime,
		EndTime:           endTime,
	}

	result, err := h.managementService.List(c.Request.Context(), dto)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewAnswerSheetListResponse(result))
}

// GetStatistics 获取答卷统计
// @Summary 获取答卷统计
// @Description 管理员查看某问卷的答卷统计数据
// @Tags AnswerSheet-Management
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code query string true "问卷编码"
// @Success 200 {object} handler.Response{data=response.AnswerSheetStatisticsResponse}
// @Router /api/v1/admin/answersheets/statistics [get]
func (h *AnswerSheetHandler) GetStatistics(c *gin.Context) {
	questionnaireCode := c.Query("code")
	if questionnaireCode == "" {
		h.ErrorResponse(c, errors.WithCode(code.ErrAnswerSheetInvalid, "问卷编码不能为空"))
		return
	}

	result, err := h.managementService.GetStatistics(c.Request.Context(), questionnaireCode)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewAnswerSheetStatisticsResponse(result))
}
