package handler

import (
	"strconv"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/request"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/response"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/asaskevich/govalidator"
	"github.com/gin-gonic/gin"
)

// ScaleHandler 量表处理器
// 对接按行为者组织的应用服务层
type ScaleHandler struct {
	BaseHandler
	lifecycleService scale.ScaleLifecycleService
	factorService    scale.ScaleFactorService
	queryService     scale.ScaleQueryService
}

// NewScaleHandler 创建量表处理器
func NewScaleHandler(
	lifecycleService scale.ScaleLifecycleService,
	factorService scale.ScaleFactorService,
	queryService scale.ScaleQueryService,
) *ScaleHandler {
	return &ScaleHandler{
		lifecycleService: lifecycleService,
		factorService:    factorService,
		queryService:     queryService,
	}
}

// ============= Lifecycle API (生命周期管理) =============

// Create 创建量表
// @Summary 创建量表
// @Description 创建新量表，初始状态为草稿
// @Tags Scale-Lifecycle
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param request body request.CreateScaleRequest true "创建量表请求"
// @Success 200 {object} core.Response{data=response.ScaleResponse}
// @Router /api/v1/scales [post]
func (h *ScaleHandler) Create(c *gin.Context) {
	var req request.CreateScaleRequest
	if err := h.BindJSON(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}
	if ok, err := govalidator.ValidateStruct(req); !ok {
		h.ErrorResponse(c, err)
		return
	}

	dto := scale.CreateScaleDTO{
		Title:                req.Title,
		Description:          req.Description,
		QuestionnaireCode:    req.QuestionnaireCode,
		QuestionnaireVersion: req.QuestionnaireVersion,
	}

	result, err := h.lifecycleService.Create(c.Request.Context(), dto)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewScaleResponse(result))
}

// UpdateBasicInfo 更新量表基本信息
// @Summary 更新量表基本信息
// @Description 更新量表的标题、描述
// @Tags Scale-Lifecycle
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "量表编码"
// @Param request body request.UpdateScaleBasicInfoRequest true "更新请求"
// @Success 200 {object} core.Response{data=response.ScaleResponse}
// @Router /api/v1/scales/{code}/basic-info [put]
func (h *ScaleHandler) UpdateBasicInfo(c *gin.Context) {
	scaleCode := c.Param("code")
	if scaleCode == "" {
		h.ErrorResponse(c, errors.WithCode(code.ErrInvalidArgument, "量表编码不能为空"))
		return
	}

	var req request.UpdateScaleBasicInfoRequest
	if err := h.BindJSON(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}
	if ok, err := govalidator.ValidateStruct(req); !ok {
		h.ErrorResponse(c, err)
		return
	}

	dto := scale.UpdateScaleBasicInfoDTO{
		Code:        scaleCode,
		Title:       req.Title,
		Description: req.Description,
	}

	result, err := h.lifecycleService.UpdateBasicInfo(c.Request.Context(), dto)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewScaleResponse(result))
}

// UpdateQuestionnaire 更新关联的问卷
// @Summary 更新关联的问卷
// @Description 更新量表关联的问卷编码和版本
// @Tags Scale-Lifecycle
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "量表编码"
// @Param request body request.UpdateScaleQuestionnaireRequest true "更新请求"
// @Success 200 {object} core.Response{data=response.ScaleResponse}
// @Router /api/v1/scales/{code}/questionnaire [put]
func (h *ScaleHandler) UpdateQuestionnaire(c *gin.Context) {
	scaleCode := c.Param("code")
	if scaleCode == "" {
		h.ErrorResponse(c, errors.WithCode(code.ErrInvalidArgument, "量表编码不能为空"))
		return
	}

	var req request.UpdateScaleQuestionnaireRequest
	if err := h.BindJSON(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}
	if ok, err := govalidator.ValidateStruct(req); !ok {
		h.ErrorResponse(c, err)
		return
	}

	dto := scale.UpdateScaleQuestionnaireDTO{
		Code:                 scaleCode,
		QuestionnaireCode:    req.QuestionnaireCode,
		QuestionnaireVersion: req.QuestionnaireVersion,
	}

	result, err := h.lifecycleService.UpdateQuestionnaire(c.Request.Context(), dto)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewScaleResponse(result))
}

// Publish 发布量表
// @Summary 发布量表
// @Description 发布量表使其可用
// @Tags Scale-Lifecycle
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "量表编码"
// @Success 200 {object} core.Response{data=response.ScaleResponse}
// @Router /api/v1/scales/{code}/publish [post]
func (h *ScaleHandler) Publish(c *gin.Context) {
	scaleCode := c.Param("code")
	if scaleCode == "" {
		h.ErrorResponse(c, errors.WithCode(code.ErrInvalidArgument, "量表编码不能为空"))
		return
	}

	result, err := h.lifecycleService.Publish(c.Request.Context(), scaleCode)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewScaleResponse(result))
}

// Unpublish 下架量表
// @Summary 下架量表
// @Description 下架量表使其不可用
// @Tags Scale-Lifecycle
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "量表编码"
// @Success 200 {object} core.Response{data=response.ScaleResponse}
// @Router /api/v1/scales/{code}/unpublish [post]
func (h *ScaleHandler) Unpublish(c *gin.Context) {
	scaleCode := c.Param("code")
	if scaleCode == "" {
		h.ErrorResponse(c, errors.WithCode(code.ErrInvalidArgument, "量表编码不能为空"))
		return
	}

	result, err := h.lifecycleService.Unpublish(c.Request.Context(), scaleCode)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewScaleResponse(result))
}

// Archive 归档量表
// @Summary 归档量表
// @Description 归档量表
// @Tags Scale-Lifecycle
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "量表编码"
// @Success 200 {object} core.Response{data=response.ScaleResponse}
// @Router /api/v1/scales/{code}/archive [post]
func (h *ScaleHandler) Archive(c *gin.Context) {
	scaleCode := c.Param("code")
	if scaleCode == "" {
		h.ErrorResponse(c, errors.WithCode(code.ErrInvalidArgument, "量表编码不能为空"))
		return
	}

	result, err := h.lifecycleService.Archive(c.Request.Context(), scaleCode)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewScaleResponse(result))
}

// Delete 删除量表
// @Summary 删除量表
// @Description 删除草稿状态的量表
// @Tags Scale-Lifecycle
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "量表编码"
// @Success 200 {object} core.Response
// @Router /api/v1/scales/{code} [delete]
func (h *ScaleHandler) Delete(c *gin.Context) {
	scaleCode := c.Param("code")
	if scaleCode == "" {
		h.ErrorResponse(c, errors.WithCode(code.ErrInvalidArgument, "量表编码不能为空"))
		return
	}

	if err := h.lifecycleService.Delete(c.Request.Context(), scaleCode); err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "删除成功", nil)
}

// ============= Factor API (因子管理 - 仅批量操作) =============

// ReplaceFactors 批量替换因子
// @Summary 批量替换因子
// @Description 批量替换量表中的所有因子
// @Tags Scale-Factor
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "量表编码"
// @Param request body request.ReplaceFactorsRequest true "替换因子请求"
// @Success 200 {object} core.Response{data=response.ScaleResponse}
// @Router /api/v1/scales/{code}/factors [put]
func (h *ScaleHandler) ReplaceFactors(c *gin.Context) {
	scaleCode := c.Param("code")
	if scaleCode == "" {
		h.ErrorResponse(c, errors.WithCode(code.ErrInvalidArgument, "量表编码不能为空"))
		return
	}

	var req request.ReplaceFactorsRequest
	if err := h.BindJSON(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}
	if ok, err := govalidator.ValidateStruct(req); !ok {
		h.ErrorResponse(c, err)
		return
	}

	factorDTOs := make([]scale.FactorDTO, 0, len(req.Factors))
	for _, f := range req.Factors {
		factorDTOs = append(factorDTOs, scale.FactorDTO{
			Code:            f.Code,
			Title:           f.Title,
			FactorType:      f.FactorType,
			IsTotalScore:    f.IsTotalScore,
			QuestionCodes:   f.QuestionCodes,
			ScoringStrategy: f.ScoringStrategy,
			ScoringParams:   f.ScoringParams,
			InterpretRules:  toInterpretRuleDTOs(f.InterpretRules),
		})
	}

	result, err := h.factorService.ReplaceFactors(c.Request.Context(), scaleCode, factorDTOs)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewScaleResponse(result))
}

// ReplaceInterpretRules 批量设置解读规则
// @Summary 批量设置解读规则
// @Description 批量设置量表所有因子的解读规则
// @Tags Scale-Factor
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "量表编码"
// @Param request body request.ReplaceInterpretRulesRequest true "设置解读规则请求"
// @Success 200 {object} core.Response{data=response.ScaleResponse}
// @Router /api/v1/scales/{code}/interpret-rules [put]
func (h *ScaleHandler) ReplaceInterpretRules(c *gin.Context) {
	scaleCode := c.Param("code")
	if scaleCode == "" {
		h.ErrorResponse(c, errors.WithCode(code.ErrInvalidArgument, "量表编码不能为空"))
		return
	}

	var req request.ReplaceInterpretRulesRequest
	if err := h.BindJSON(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}
	if ok, err := govalidator.ValidateStruct(req); !ok {
		h.ErrorResponse(c, err)
		return
	}

	// 构建批量更新 DTO
	dtos := make([]scale.UpdateFactorInterpretRulesDTO, 0, len(req.FactorRules))
	for _, fr := range req.FactorRules {
		dtos = append(dtos, scale.UpdateFactorInterpretRulesDTO{
			ScaleCode:      scaleCode,
			FactorCode:     fr.FactorCode,
			InterpretRules: toInterpretRuleDTOs(fr.InterpretRules),
		})
	}

	result, err := h.factorService.ReplaceInterpretRules(c.Request.Context(), scaleCode, dtos)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewScaleResponse(result))
}

// ============= Query API (查询) =============

// GetByCode 根据编码获取量表
// @Summary 获取量表详情
// @Description 根据编码获取量表详情
// @Tags Scale-Query
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "量表编码"
// @Success 200 {object} core.Response{data=response.ScaleResponse}
// @Router /api/v1/scales/{code} [get]
func (h *ScaleHandler) GetByCode(c *gin.Context) {
	scaleCode := c.Param("code")
	if scaleCode == "" {
		h.ErrorResponse(c, errors.WithCode(code.ErrInvalidArgument, "量表编码不能为空"))
		return
	}

	result, err := h.queryService.GetByCode(c.Request.Context(), scaleCode)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewScaleResponse(result))
}

// GetByQuestionnaireCode 根据问卷编码获取量表
// @Summary 根据问卷编码获取量表
// @Description 根据关联的问卷编码获取量表
// @Tags Scale-Query
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param questionnaireCode query string true "问卷编码"
// @Success 200 {object} core.Response{data=response.ScaleResponse}
// @Router /api/v1/scales/by-questionnaire [get]
func (h *ScaleHandler) GetByQuestionnaireCode(c *gin.Context) {
	questionnaireCode := c.Query("questionnaire_code")
	if questionnaireCode == "" {
		h.ErrorResponse(c, errors.WithCode(code.ErrInvalidArgument, "问卷编码不能为空"))
		return
	}

	result, err := h.queryService.GetByQuestionnaireCode(c.Request.Context(), questionnaireCode)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewScaleResponse(result))
}

// List 获取量表列表
// @Summary 获取量表列表
// @Description 分页获取量表列表
// @Tags Scale-Query
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param page query int true "页码"
// @Param page_size query int true "每页数量"
// @Success 200 {object} core.Response{data=response.ScaleListResponse}
// @Router /api/v1/scales [get]
func (h *ScaleHandler) List(c *gin.Context) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page <= 0 {
		h.ErrorResponse(c, errors.WithCode(code.ErrInvalidArgument, "页码无效"))
		return
	}

	pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if err != nil || pageSize <= 0 {
		h.ErrorResponse(c, errors.WithCode(code.ErrInvalidArgument, "每页数量无效"))
		return
	}

	dto := scale.ListScalesDTO{
		Page:       page,
		PageSize:   pageSize,
		Conditions: make(map[string]string),
	}

	// 解析查询条件
	if status := c.Query("status"); status != "" {
		dto.Conditions["status"] = status
	}
	if title := c.Query("title"); title != "" {
		dto.Conditions["title"] = title
	}

	result, err := h.queryService.List(c.Request.Context(), dto)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewScaleListResponse(result, page, pageSize))
}

// GetPublishedByCode 获取已发布的量表
// @Summary 获取已发布的量表
// @Description 根据编码获取已发布的量表
// @Tags Scale-Query
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "量表编码"
// @Success 200 {object} core.Response{data=response.ScaleResponse}
// @Router /api/v1/scales/published/{code} [get]
func (h *ScaleHandler) GetPublishedByCode(c *gin.Context) {
	scaleCode := c.Param("code")
	if scaleCode == "" {
		h.ErrorResponse(c, errors.WithCode(code.ErrInvalidArgument, "量表编码不能为空"))
		return
	}

	result, err := h.queryService.GetPublishedByCode(c.Request.Context(), scaleCode)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewScaleResponse(result))
}

// ListPublished 获取已发布量表列表
// @Summary 获取已发布量表列表
// @Description 分页获取已发布的量表列表
// @Tags Scale-Query
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param page query int true "页码"
// @Param page_size query int true "每页数量"
// @Success 200 {object} core.Response{data=response.ScaleListResponse}
// @Router /api/v1/scales/published [get]
func (h *ScaleHandler) ListPublished(c *gin.Context) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page <= 0 {
		h.ErrorResponse(c, errors.WithCode(code.ErrInvalidArgument, "页码无效"))
		return
	}

	pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if err != nil || pageSize <= 0 {
		h.ErrorResponse(c, errors.WithCode(code.ErrInvalidArgument, "每页数量无效"))
		return
	}

	dto := scale.ListScalesDTO{
		Page:       page,
		PageSize:   pageSize,
		Conditions: make(map[string]string),
	}

	result, err := h.queryService.ListPublished(c.Request.Context(), dto)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewScaleListResponse(result, page, pageSize))
}

// ============= Helper Functions =============

// toInterpretRuleDTOs 转换解读规则请求为 DTO
func toInterpretRuleDTOs(rules []request.InterpretRuleModel) []scale.InterpretRuleDTO {
	result := make([]scale.InterpretRuleDTO, 0, len(rules))
	for _, r := range rules {
		result = append(result, scale.InterpretRuleDTO{
			MinScore:   r.MinScore,
			MaxScore:   r.MaxScore,
			RiskLevel:  r.RiskLevel,
			Conclusion: r.Conclusion,
			Suggestion: r.Suggestion,
		})
	}
	return result
}
