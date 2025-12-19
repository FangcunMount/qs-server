package handler

import (
	"strconv"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/scale"
	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
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
// @Description 创建新量表，初始状态为草稿。支持设置主类、阶段、使用年龄、填报人和标签等分类信息。
// @Tags Scale-Lifecycle
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param request body request.CreateScaleRequest true "创建量表请求（包含主类、阶段、使用年龄、填报人、标签等字段）"
// @Success 200 {object} core.Response{data=response.ScaleResponse}
// @Router /api/v1/scales [post]
func (h *ScaleHandler) Create(c *gin.Context) {
	var req request.CreateScaleRequest
	if err := h.BindJSON(c, &req); err != nil {
		h.Error(c, err)
		return
	}
	if ok, err := govalidator.ValidateStruct(req); !ok {
		h.Error(c, err)
		return
	}

	dto := scale.CreateScaleDTO{
		Title:                req.Title,
		Description:          req.Description,
		Category:             req.Category,
		Stage:                req.Stage,
		ApplicableAge:        req.ApplicableAge,
		Reporter:             req.Reporter,
		Tags:                 req.Tags,
		QuestionnaireCode:    req.QuestionnaireCode,
		QuestionnaireVersion: req.QuestionnaireVersion,
	}

	result, err := h.lifecycleService.Create(c.Request.Context(), dto)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewScaleResponse(result))
}

// UpdateBasicInfo 更新量表基本信息
// @Summary 更新量表基本信息
// @Description 更新量表的标题、描述、主类、阶段、使用年龄、填报人和标签等分类信息
// @Tags Scale-Lifecycle
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "量表编码"
// @Param request body request.UpdateScaleBasicInfoRequest true "更新请求（包含标题、描述、主类、阶段、使用年龄、填报人、标签等字段）"
// @Success 200 {object} core.Response{data=response.ScaleResponse}
// @Router /api/v1/scales/{code}/basic-info [put]
func (h *ScaleHandler) UpdateBasicInfo(c *gin.Context) {
	scaleCode := c.Param("code")
	if scaleCode == "" {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "量表编码不能为空"))
		return
	}

	var req request.UpdateScaleBasicInfoRequest
	if err := h.BindJSON(c, &req); err != nil {
		h.Error(c, err)
		return
	}
	if ok, err := govalidator.ValidateStruct(req); !ok {
		h.Error(c, err)
		return
	}

	dto := scale.UpdateScaleBasicInfoDTO{
		Code:          scaleCode,
		Title:         req.Title,
		Description:   req.Description,
		Category:      req.Category,
		Stage:         req.Stage,
		ApplicableAge: req.ApplicableAge,
		Reporter:      req.Reporter,
		Tags:          req.Tags,
	}

	result, err := h.lifecycleService.UpdateBasicInfo(c.Request.Context(), dto)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewScaleResponse(result))
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
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "量表编码不能为空"))
		return
	}

	var req request.UpdateScaleQuestionnaireRequest
	if err := h.BindJSON(c, &req); err != nil {
		h.Error(c, err)
		return
	}
	if ok, err := govalidator.ValidateStruct(req); !ok {
		h.Error(c, err)
		return
	}

	dto := scale.UpdateScaleQuestionnaireDTO{
		Code:                 scaleCode,
		QuestionnaireCode:    req.QuestionnaireCode,
		QuestionnaireVersion: req.QuestionnaireVersion,
	}

	result, err := h.lifecycleService.UpdateQuestionnaire(c.Request.Context(), dto)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewScaleResponse(result))
}

// Publish 发布量表
// @Summary 发布量表
// @Description 发布量表使其可用。量表编码通过 URL 路径参数传递，不需要请求体。
// @Tags Scale-Lifecycle
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "量表编码"
// @Success 200 {object} core.Response{data=response.ScaleResponse}
// @Router /api/v1/scales/{code}/publish [post]
func (h *ScaleHandler) Publish(c *gin.Context) {
	scaleCode := c.Param("code")
	logger.L(c.Request.Context()).Infow("Publish: 发布量表", "scaleCode", scaleCode)
	if scaleCode == "" {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "量表编码不能为空，请通过 URL 路径参数传递，例如：POST /api/v1/scales/{code}/publish"))
		return
	}

	result, err := h.lifecycleService.Publish(c.Request.Context(), scaleCode)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewScaleResponse(result))
}

// Unpublish 下架量表
// @Summary 下架量表
// @Description 下架量表使其不可用。量表编码通过 URL 路径参数传递，不需要请求体。
// @Tags Scale-Lifecycle
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "量表编码"
// @Success 200 {object} core.Response{data=response.ScaleResponse}
// @Router /api/v1/scales/{code}/unpublish [post]
func (h *ScaleHandler) Unpublish(c *gin.Context) {
	scaleCode := c.Param("code")
	if scaleCode == "" {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "量表编码不能为空，请通过 URL 路径参数传递，例如：POST /api/v1/scales/{code}/unpublish"))
		return
	}

	result, err := h.lifecycleService.Unpublish(c.Request.Context(), scaleCode)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewScaleResponse(result))
}

// Archive 归档量表
// @Summary 归档量表
// @Description 归档量表。量表编码通过 URL 路径参数传递，不需要请求体。
// @Tags Scale-Lifecycle
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "量表编码"
// @Success 200 {object} core.Response{data=response.ScaleResponse}
// @Router /api/v1/scales/{code}/archive [post]
func (h *ScaleHandler) Archive(c *gin.Context) {
	scaleCode := c.Param("code")
	if scaleCode == "" {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "量表编码不能为空，请通过 URL 路径参数传递，例如：POST /api/v1/scales/{code}/archive"))
		return
	}

	result, err := h.lifecycleService.Archive(c.Request.Context(), scaleCode)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewScaleResponse(result))
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
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "量表编码不能为空"))
		return
	}

	if err := h.lifecycleService.Delete(c.Request.Context(), scaleCode); err != nil {
		h.Error(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "删除成功", nil)
}

// ============= Factor API (因子管理 - 仅批量操作) =============

// BatchUpdateFactors 批量更新因子
// @Summary 批量更新因子
// @Description 批量更新量表的所有因子（前端保存时使用）。计分参数根据策略类型使用不同字段：
// @Description - sum/avg 策略：scoring_params 可为空或省略
// @Description - cnt 策略：scoring_params 必须包含 cnt_option_contents（选项内容数组，字符串数组），且不能为空
// @Description - risk_level：因子级别的风险等级（可选），如果解读规则中未指定风险等级，则使用此值；有效值：none/low/medium/high/severe
// @Description 响应中的 scoring_params 为 map[string]interface{}，cnt 策略直接包含 cnt_option_contents 字段
// @Description 响应中的 risk_level 为因子级别的风险等级，从解读规则中提取（使用第一个规则的风险等级）
// @Tags Scale-Factor
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "量表编码"
// @Param request body request.BatchUpdateFactorsRequest true "批量更新因子请求"
// @Success 200 {object} core.Response{data=response.ScaleResponse}
// @Router /api/v1/scales/{code}/factors/batch [put]
func (h *ScaleHandler) BatchUpdateFactors(c *gin.Context) {
	scaleCode := c.Param("code")
	if scaleCode == "" {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "量表编码不能为空"))
		return
	}

	var req request.BatchUpdateFactorsRequest
	if err := h.BindJSON(c, &req); err != nil {
		h.Error(c, err)
		return
	}
	if ok, err := govalidator.ValidateStruct(req); !ok {
		h.Error(c, err)
		return
	}

	factorDTOs := make([]scale.FactorDTO, 0, len(req.Factors))
	for _, f := range req.Factors {
		// 转换 ScoringParamsModel 为 ScoringParamsDTO
		var scoringParamsDTO *scale.ScoringParamsDTO
		if f.ScoringParams != nil {
			scoringParamsDTO = &scale.ScoringParamsDTO{
				CntOptionContents: f.ScoringParams.CntOptionContents,
			}
		}

		// 转换解读规则，如果规则中没有指定风险等级，使用因子级别的风险等级
		interpretRules := toInterpretRuleDTOs(f.InterpretRules, f.RiskLevel)

		factorDTOs = append(factorDTOs, scale.FactorDTO{
			Code:            f.Code,
			Title:           f.Title,
			FactorType:      f.FactorType,
			IsTotalScore:    f.IsTotalScore,
			QuestionCodes:   f.QuestionCodes,
			ScoringStrategy: f.ScoringStrategy,
			ScoringParams:   scoringParamsDTO,
			RiskLevel:       f.RiskLevel,
			InterpretRules:  interpretRules,
		})
	}

	result, err := h.factorService.ReplaceFactors(c.Request.Context(), scaleCode, factorDTOs)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewScaleResponse(result))
}

// ReplaceInterpretRules 批量设置解读规则
// @Summary 批量设置解读规则
// @Description 批量设置量表所有因子的解读规则
// @Description 响应中的 risk_level 为因子级别的风险等级，从解读规则中提取（使用第一个规则的风险等级），有效值：none/low/medium/high/severe
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
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "量表编码不能为空"))
		return
	}

	var req request.ReplaceInterpretRulesRequest
	if err := h.BindJSON(c, &req); err != nil {
		h.Error(c, err)
		return
	}
	if ok, err := govalidator.ValidateStruct(req); !ok {
		h.Error(c, err)
		return
	}

	// 构建批量更新 DTO
	dtos := make([]scale.UpdateFactorInterpretRulesDTO, 0, len(req.FactorRules))
	for _, fr := range req.FactorRules {
		dtos = append(dtos, scale.UpdateFactorInterpretRulesDTO{
			ScaleCode:      scaleCode,
			FactorCode:     fr.FactorCode,
			InterpretRules: toInterpretRuleDTOs(fr.InterpretRules, ""), // 批量设置解读规则接口不使用因子级别的风险等级
		})
	}

	result, err := h.factorService.ReplaceInterpretRules(c.Request.Context(), scaleCode, dtos)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewScaleResponse(result))
}

// ============= Query API (查询) =============

// GetByCode 根据编码获取量表
// @Summary 获取量表详情
// @Description 根据编码获取量表详情。响应中的 scoring_params 为 map[string]interface{}，cnt 策略直接包含 cnt_option_contents 字段
// @Description 响应中的 risk_level 为因子级别的风险等级，从解读规则中提取（使用第一个规则的风险等级），有效值：none/low/medium/high/severe
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
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "量表编码不能为空"))
		return
	}

	result, err := h.queryService.GetByCode(c.Request.Context(), scaleCode)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewScaleResponse(result))
}

// GetByQuestionnaireCode 根据问卷编码获取量表
// @Summary 根据问卷编码获取量表
// @Description 根据关联的问卷编码获取量表。响应中的 scoring_params 为 map[string]interface{}，cnt 策略直接包含 cnt_option_contents 字段
// @Description 响应中的 risk_level 为因子级别的风险等级，从解读规则中提取（使用第一个规则的风险等级），有效值：none/low/medium/high/severe
// @Tags Scale-Query
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param questionnaire_code query string true "问卷编码"
// @Success 200 {object} core.Response{data=response.ScaleResponse}
// @Router /api/v1/scales/by-questionnaire [get]
func (h *ScaleHandler) GetByQuestionnaireCode(c *gin.Context) {
	questionnaireCode := c.Query("questionnaire_code")
	if questionnaireCode == "" {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "问卷编码不能为空"))
		return
	}

	result, err := h.queryService.GetByQuestionnaireCode(c.Request.Context(), questionnaireCode)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewScaleResponse(result))
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
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "页码无效"))
		return
	}

	pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if err != nil || pageSize <= 0 {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "每页数量无效"))
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
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewScaleSummaryListResponse(result, page, pageSize))
}

// GetPublishedByCode 获取已发布的量表
// @Summary 获取已发布的量表
// @Description 根据编码获取已发布的量表。响应中的 scoring_params 为 map[string]interface{}，cnt 策略直接包含 cnt_option_contents 字段
// @Description 响应中的 risk_level 为因子级别的风险等级，从解读规则中提取（使用第一个规则的风险等级），有效值：none/low/medium/high/severe
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
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "量表编码不能为空"))
		return
	}

	result, err := h.queryService.GetPublishedByCode(c.Request.Context(), scaleCode)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewScaleResponse(result))
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
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "页码无效"))
		return
	}

	pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if err != nil || pageSize <= 0 {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "每页数量无效"))
		return
	}

	dto := scale.ListScalesDTO{
		Page:       page,
		PageSize:   pageSize,
		Conditions: make(map[string]string),
	}

	result, err := h.queryService.ListPublished(c.Request.Context(), dto)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewScaleSummaryListResponse(result, page, pageSize))
}

// GetFactors 获取量表的因子列表
// @Summary 获取量表的因子列表
// @Description 根据量表编码获取该量表的所有因子。响应中的 scoring_params 为 map[string]interface{}，cnt 策略直接包含 cnt_option_contents 字段
// @Description 响应中的 risk_level 为因子级别的风险等级，从解读规则中提取（使用第一个规则的风险等级），有效值：none/low/medium/high/severe
// @Tags Scale-Query
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "量表编码"
// @Success 200 {object} core.Response{data=response.FactorListResponse}
// @Router /api/v1/scales/{code}/factors [get]
func (h *ScaleHandler) GetFactors(c *gin.Context) {
	scaleCode := c.Param("code")
	if scaleCode == "" {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "量表编码不能为空"))
		return
	}

	factors, err := h.queryService.GetFactors(c.Request.Context(), scaleCode)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewFactorListResponse(factors))
}

// GetCategories 获取量表分类列表
// @Summary 获取量表分类列表
// @Description 获取量表的主类、阶段、使用年龄、填报人和标签等分类选项列表，用于前端渲染和配置量表字段
// @Tags Scale-Query
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Success 200 {object} core.Response{data=response.ScaleCategoriesResponse}
// @Router /api/v1/scales/categories [get]
func (h *ScaleHandler) GetCategories(c *gin.Context) {
	// 构建类别列表
	categories := []response.CategoryResponse{
		{Value: string(domainScale.CategoryADHD), Label: "ADHD"},
		{Value: string(domainScale.CategoryTicDisorder), Label: "抽动障碍"},
		{Value: string(domainScale.CategorySensoryIntegration), Label: "感统"},
		{Value: string(domainScale.CategoryExecutiveFunction), Label: "执行功能"},
		{Value: string(domainScale.CategoryMentalHealth), Label: "心理健康"},
		{Value: string(domainScale.CategoryNeurodevelopmentalScreening), Label: "神经发育筛查"},
		{Value: string(domainScale.CategoryChronicDiseaseManagement), Label: "慢性病管理"},
		{Value: string(domainScale.CategoryQualityOfLife), Label: "生活质量"},
	}

	// 构建阶段列表
	stages := []response.StageResponse{
		{Value: string(domainScale.StageScreening), Label: "筛查"},
		{Value: string(domainScale.StageDeepAssessment), Label: "深评"},
		{Value: string(domainScale.StageFollowUp), Label: "随访"},
		{Value: string(domainScale.StageOutcome), Label: "结局"},
	}

	// 构建使用年龄列表
	applicableAges := []response.ApplicableAgeResponse{
		{Value: string(domainScale.ApplicableAgeInfant), Label: "婴幼儿"},
		{Value: string(domainScale.ApplicableAgeSchoolAge), Label: "学龄"},
		{Value: string(domainScale.ApplicableAgeAdolescentAdult), Label: "青少年/成人"},
		{Value: string(domainScale.ApplicableAgeChildAdolescent), Label: "儿童/青少年"},
	}

	// 构建填报人列表
	reporters := []response.ReporterResponse{
		{Value: string(domainScale.ReporterParent), Label: "家长评"},
		{Value: string(domainScale.ReporterTeacher), Label: "教师评"},
		{Value: string(domainScale.ReporterSelf), Label: "自评"},
		{Value: string(domainScale.ReporterClinical), Label: "临床评定"},
	}

	// 构建标签列表
	tags := []response.TagResponse{
		// 阶段标签
		{Value: string(domainScale.TagScreening), Label: "筛查", Category: "stage"},
		{Value: string(domainScale.TagDeepAssessment), Label: "深评", Category: "stage"},
		{Value: string(domainScale.TagFollowUp), Label: "随访", Category: "stage"},
		{Value: string(domainScale.TagOutcome), Label: "功能结局", Category: "stage"},
		// 主题标签
		{Value: string(domainScale.TagBriefVersion), Label: "简版", Category: "theme"},
		{Value: string(domainScale.TagBroadSpectrum), Label: "广谱", Category: "theme"},
		{Value: string(domainScale.TagComorbidity), Label: "共病", Category: "theme"},
		{Value: string(domainScale.TagFunction), Label: "功能", Category: "theme"},
		{Value: string(domainScale.TagFamilySystem), Label: "家庭系统", Category: "theme"},
		{Value: string(domainScale.TagStress), Label: "压力", Category: "theme"},
		{Value: string(domainScale.TagInfant), Label: "婴幼儿", Category: "theme"},
		{Value: string(domainScale.TagSchoolAge), Label: "学龄", Category: "theme"},
		{Value: string(domainScale.TagAdolescent), Label: "青少年/成人", Category: "theme"},
		// 状态标签
		{Value: string(domainScale.TagNeedsVersioning), Label: "需定版", Category: "status"},
		{Value: string(domainScale.TagCustom), Label: "自定义", Category: "status"},
		// 填报人标签
		{Value: string(domainScale.TagParentRating), Label: "家长评", Category: "reporter"},
		{Value: string(domainScale.TagTeacherRating), Label: "教师评", Category: "reporter"},
		{Value: string(domainScale.TagSelfRating), Label: "自评", Category: "reporter"},
		{Value: string(domainScale.TagClinicalRating), Label: "临床评定", Category: "reporter"},
	}

	result := &response.ScaleCategoriesResponse{
		Categories:     categories,
		Stages:         stages,
		ApplicableAges: applicableAges,
		Reporters:      reporters,
		Tags:           tags,
	}

	h.Success(c, result)
}

// ============= Helper Functions =============

// toInterpretRuleDTOs 转换解读规则请求为 DTO
// defaultRiskLevel 为因子级别的默认风险等级，如果解读规则中没有指定风险等级，则使用此值
func toInterpretRuleDTOs(rules []request.InterpretRuleModel, defaultRiskLevel string) []scale.InterpretRuleDTO {
	result := make([]scale.InterpretRuleDTO, 0, len(rules))
	for _, r := range rules {
		// 如果解读规则中没有指定风险等级，使用因子级别的默认风险等级
		riskLevel := r.RiskLevel
		if riskLevel == "" && defaultRiskLevel != "" {
			riskLevel = defaultRiskLevel
		}
		// 如果都没有指定，使用默认值 "none"
		if riskLevel == "" {
			riskLevel = "none"
		}

		result = append(result, scale.InterpretRuleDTO{
			MinScore:   r.MinScore,
			MaxScore:   r.MaxScore,
			RiskLevel:  riskLevel,
			Conclusion: r.Conclusion,
			Suggestion: r.Suggestion,
		})
	}
	return result
}
