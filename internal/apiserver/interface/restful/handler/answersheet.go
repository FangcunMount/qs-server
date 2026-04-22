package handler

import (
	"strconv"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/request"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/response"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
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
	submissionService answersheet.AnswerSheetSubmissionService
}

// NewAnswerSheetHandler 创建答卷处理器
func NewAnswerSheetHandler(
	managementService answersheet.AnswerSheetManagementService,
	submissionService answersheet.AnswerSheetSubmissionService,
) *AnswerSheetHandler {
	return &AnswerSheetHandler{
		managementService: managementService,
		submissionService: submissionService,
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
// @Param id path string true "答卷ID"
// @Success 200 {object} core.Response{data=response.AnswerSheetResponse}
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/answersheets/{id} [get]
func (h *AnswerSheetHandler) GetByID(c *gin.Context) {
	answerSheetID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.Error(c, errors.WithCode(code.ErrAnswerSheetInvalid, "无效的答卷ID"))
		return
	}

	result, err := h.managementService.GetByID(c.Request.Context(), answerSheetID)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewAnswerSheetResponse(result))
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
// @Param filler_id query string false "填写人ID"
// @Param start_time query string false "开始时间"
// @Param end_time query string false "结束时间"
// @Success 200 {object} core.Response{data=response.AnswerSheetListResponse}
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/answersheets [get]
func (h *AnswerSheetHandler) List(c *gin.Context) {
	dto, err := buildAnswerSheetListDTO(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	result, err := h.managementService.List(c.Request.Context(), dto)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewAnswerSheetSummaryListResponse(result))
}

// AdminSubmit 管理员提交答卷
// @Summary 管理员提交答卷
// @Description 管理员绕过监护关系校验提交答卷
// @Tags AnswerSheet-Management
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param request body request.AdminSubmitAnswerSheetRequest true "答卷数据"
// @Success 200 {object} core.Response{data=response.AnswerSheetResponse}
// @Failure 429 {object} core.ErrResponse
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /api/v1/answersheets/admin-submit [post]
func (h *AnswerSheetHandler) AdminSubmit(c *gin.Context) {
	var req request.AdminSubmitAnswerSheetRequest
	if err := h.BindJSON(c, &req); err != nil {
		return
	}

	fillerID, ok := h.resolveAdminSubmitFillerID(c, req)
	if !ok {
		h.UnauthorizedResponse(c, "user not authenticated")
		return
	}
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	orgScope, err := safeconv.Int64ToUint64(orgID)
	if err != nil {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "org scope exceeds uint64"))
		return
	}
	dto := buildAdminSubmitDTO(req, fillerID, orgScope)

	result, err := h.submissionService.Submit(c.Request.Context(), dto)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewAnswerSheetResponse(result))
}

func buildAnswerSheetListDTO(c *gin.Context) (answersheet.ListAnswerSheetsDTO, error) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page <= 0 {
		return answersheet.ListAnswerSheetsDTO{}, errors.WithCode(code.ErrAnswerSheetInvalid, "页码必须为正整数")
	}
	pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	if err != nil || pageSize <= 0 || pageSize > 100 {
		return answersheet.ListAnswerSheetsDTO{}, errors.WithCode(code.ErrAnswerSheetInvalid, "每页数量必须为1-100的整数")
	}

	return answersheet.ListAnswerSheetsDTO{
		Page:              page,
		PageSize:          pageSize,
		QuestionnaireCode: c.Query("questionnaire_code"),
		FillerID:          optionalUint64Query(c.Query("filler_id")),
		StartTime:         optionalDateQuery(c.Query("start_time")),
		EndTime:           optionalDateQuery(c.Query("end_time")),
	}, nil
}

func optionalUint64Query(raw string) *uint64 {
	if raw == "" {
		return nil
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return nil
	}
	return &value
}

func optionalDateQuery(raw string) *time.Time {
	if raw == "" {
		return nil
	}
	value, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return nil
	}
	return &value
}

func (h *AnswerSheetHandler) resolveAdminSubmitFillerID(c *gin.Context, req request.AdminSubmitAnswerSheetRequest) (uint64, bool) {
	switch {
	case req.FillerID != 0:
		return req.FillerID, true
	case req.WriterID != 0:
		return req.WriterID, true
	default:
		userID, ok := h.GetUserIDUint64(c)
		return userID, ok && userID != 0
	}
}

func buildAdminSubmitDTO(req request.AdminSubmitAnswerSheetRequest, fillerID, orgID uint64) answersheet.SubmitAnswerSheetDTO {
	answers := make([]answersheet.AnswerDTO, 0, len(req.Answers))
	for _, answer := range req.Answers {
		answers = append(answers, answersheet.AnswerDTO{
			QuestionCode: answer.QuestionCode,
			QuestionType: answer.QuestionType,
			Value:        answer.Value,
		})
	}

	return answersheet.SubmitAnswerSheetDTO{
		QuestionnaireCode: req.QuestionnaireCode,
		QuestionnaireVer:  req.QuestionnaireVersion,
		TesteeID:          req.TesteeID,
		FillerID:          fillerID,
		OrgID:             orgID,
		TaskID:            req.TaskID,
		Answers:           answers,
	}
}
