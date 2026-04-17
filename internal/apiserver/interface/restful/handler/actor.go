package handler

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	actorAccessApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	assessmentEntryApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/assessmententry"
	clinicianApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/clinician"
	operatorApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operator"
	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	qrcodeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/request"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/response"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/gin-gonic/gin"
)

// ActorHandler Actor 模块的 HTTP Handler
type ActorHandler struct {
	*BaseHandler
	// Testee 服务按行为者组织
	testeeRegistrationService testeeApp.TesteeRegistrationService
	testeeManagementService   testeeApp.TesteeManagementService
	testeeQueryService        testeeApp.TesteeQueryService        // 通用查询服务（小程序、C端）
	testeeBackendQueryService testeeApp.TesteeBackendQueryService // 后台查询服务（包含家长信息）
	// Operator 服务按行为者组织
	operatorLifecycleService     operatorApp.OperatorLifecycleService
	operatorAuthorizationService operatorApp.OperatorAuthorizationService
	operatorQueryService         operatorApp.OperatorQueryService
	clinicianLifecycleService    clinicianApp.ClinicianLifecycleService
	clinicianQueryService        clinicianApp.ClinicianQueryService
	clinicianRelationshipService clinicianApp.ClinicianRelationshipService
	testeeAccessService          actorAccessApp.TesteeAccessService
	assessmentEntryService       assessmentEntryApp.AssessmentEntryService
	// IAM 服务（可选）
	guardianshipService *iam.GuardianshipService
	qrCodeService       qrcodeApp.QRCodeService
	// Evaluation 服务（用于查询测评记录）
	assessmentManagementService assessmentApp.AssessmentManagementService
	scoreQueryService           assessmentApp.ScoreQueryService
}

// NewActorHandler 创建 Actor Handler
func NewActorHandler(
	testeeRegistrationService testeeApp.TesteeRegistrationService,
	testeeManagementService testeeApp.TesteeManagementService,
	testeeQueryService testeeApp.TesteeQueryService,
	testeeBackendQueryService testeeApp.TesteeBackendQueryService,
	operatorLifecycleService operatorApp.OperatorLifecycleService,
	operatorAuthorizationService operatorApp.OperatorAuthorizationService,
	operatorQueryService operatorApp.OperatorQueryService,
	clinicianLifecycleService clinicianApp.ClinicianLifecycleService,
	clinicianQueryService clinicianApp.ClinicianQueryService,
	clinicianRelationshipService clinicianApp.ClinicianRelationshipService,
	testeeAccessService actorAccessApp.TesteeAccessService,
	assessmentEntryService assessmentEntryApp.AssessmentEntryService,
	guardianshipService *iam.GuardianshipService,
	qrCodeService qrcodeApp.QRCodeService,
	assessmentManagementService assessmentApp.AssessmentManagementService,
	scoreQueryService assessmentApp.ScoreQueryService,
) *ActorHandler {
	return &ActorHandler{
		BaseHandler:                  NewBaseHandler(),
		testeeRegistrationService:    testeeRegistrationService,
		testeeManagementService:      testeeManagementService,
		testeeQueryService:           testeeQueryService,
		testeeBackendQueryService:    testeeBackendQueryService,
		operatorLifecycleService:     operatorLifecycleService,
		operatorAuthorizationService: operatorAuthorizationService,
		operatorQueryService:         operatorQueryService,
		clinicianLifecycleService:    clinicianLifecycleService,
		clinicianQueryService:        clinicianQueryService,
		clinicianRelationshipService: clinicianRelationshipService,
		testeeAccessService:          testeeAccessService,
		assessmentEntryService:       assessmentEntryService,
		guardianshipService:          guardianshipService,
		qrCodeService:                qrCodeService,
		assessmentManagementService:  assessmentManagementService,
		scoreQueryService:            scoreQueryService,
	}
}

// ========== Testee API ==========

// GetTestee 获取受试者详情
// @Summary 获取受试者详情
// @Tags Actor
// @Produce json
// @Param id path string true "受试者ID"
// @Success 200 {object} core.Response{data=response.TesteeResponse}
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/testees/{id} [get]
// GetTestee 获取受试者详情（后台管理接口，包含家长信息）
func (h *ActorHandler) GetTestee(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		logger.L(c.Request.Context()).Warnw("Invalid testee ID",
			"action", "get_testee",
			"testee_id", idStr,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	orgID, _, err := h.validateProtectedTesteeAccess(c, id)
	if err != nil {
		h.Error(c, err)
		return
	}

	// 使用后台查询服务（包含家长信息）
	backendResult, err := h.testeeBackendQueryService.GetByIDWithGuardians(c.Request.Context(), id)
	if err != nil {
		logger.L(c.Request.Context()).Errorw("Failed to get testee with guardians",
			"action", "get_testee",
			"testee_id", id,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}
	if backendResult.OrgID != orgID {
		h.Error(c, errors.WithCode(code.ErrPermissionDenied, "testee does not belong to current organization"))
		return
	}

	// 转换为响应对象（包含家长信息）
	resp := toTesteeBackendResponse(backendResult)

	h.Success(c, resp)
}

// GetTesteeByProfileID 根据 profile_id 获取受试者详情
// @Summary 根据 profile_id 获取受试者详情
// @Description 以 JWT org_id 为准，根据档案ID查询受试者；org_id 查询参数仅作兼容校验，若传入则必须与 JWT org_id 一致
// @Tags Actor
// @Produce json
// @Param org_id query string false "兼容字段：机构ID，若传入必须与 JWT org_id 一致"
// @Param profile_id query string true "用户档案ID（IAM Child ID/ProfileID）"
// @Param iam_child_id query string false "兼容字段：IAM儿童ID"
// @Success 200 {object} core.Response{data=response.TesteeResponse}
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/testees/by-profile-id [get]
func (h *ActorHandler) GetTesteeByProfileID(c *gin.Context) {
	_, operatorUserID, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	var req request.GetTesteeByProfileIDRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		logger.L(c.Request.Context()).Warnw("Invalid get testee by profile_id request",
			"action", "get_testee_by_profile_id",
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}
	orgID, err := h.RequireProtectedOrgIDWithLegacy(c, req.OrgID)
	if err != nil {
		h.Error(c, err)
		return
	}

	profileIDStr := req.CanonicalProfileID()
	if profileIDStr == "" {
		h.BadRequestResponse(c, "profile_id is required", nil)
		return
	}

	testeeResult, err := h.fetchTesteeByProfile(c, orgID, profileIDStr)
	if err != nil {
		h.Error(c, err)
		return
	}
	if err := h.testeeAccessService.ValidateTesteeAccess(c.Request.Context(), orgID, operatorUserID, testeeResult.ID); err != nil {
		h.Error(c, err)
		return
	}

	// 使用后台查询服务获取包含监护人的完整信息
	if h.testeeBackendQueryService != nil {
		backendResult, backendErr := h.testeeBackendQueryService.GetByIDWithGuardians(c.Request.Context(), testeeResult.ID)
		if backendErr != nil {
			h.Error(c, backendErr)
			return
		}
		h.Success(c, toTesteeBackendResponse(backendResult))
		return
	}

	h.Success(c, toTesteeResponse(testeeResult))
}

// GetScaleAnalysis
// @Summary 获取受试者量表分析结果
// @Tags Testee
// @Produce json
// @Param id path string true "Testee ID"
// @Success 200 {object} core.Response{data=response.ScaleAnalysisResponse}
// @Failure 429 {object} core.ErrResponse
// @Failure 400 {object} core.Response
// @Failure 404 {object} core.Response
// @Router /api/v1/testees/{id}/scale-analysis [get]
func (h *ActorHandler) GetScaleAnalysis(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		logger.L(c.Request.Context()).Warnw("Invalid testee ID",
			"action", "get_scale_analysis",
			"testee_id", idStr,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}
	orgID, _, err := h.validateProtectedTesteeAccess(c, id)
	if err != nil {
		h.Error(c, err)
		return
	}

	// 验证受试者是否存在
	_, err = h.testeeQueryService.GetByID(c.Request.Context(), id)
	if err != nil {
		logger.L(c.Request.Context()).Errorw("Failed to get testee",
			"action", "get_scale_analysis",
			"testee_id", id,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	// 查询该受试者的所有测评记录
	listDTO := assessmentApp.ListAssessmentsDTO{
		OrgID:    uint64(orgID),
		Page:     1,
		PageSize: 1000, // 获取所有记录
		Conditions: map[string]string{
			"testee_id": strconv.FormatUint(id, 10),
		},
	}

	assessmentList, err := h.assessmentManagementService.List(c.Request.Context(), listDTO)
	if err != nil {
		logger.L(c.Request.Context()).Errorw("Failed to list assessments",
			"action", "get_scale_analysis",
			"testee_id", id,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	// 按量表分组
	scaleMap := make(map[string]*response.ScaleTrendResponse)
	for _, assessment := range assessmentList.Items {
		// 只处理已解读的测评
		if assessment.Status != "interpreted" || assessment.MedicalScaleCode == nil {
			continue
		}

		scaleCode := *assessment.MedicalScaleCode
		scaleName := ""
		if assessment.MedicalScaleName != nil {
			scaleName = *assessment.MedicalScaleName
		}

		// 获取或创建量表趋势
		scaleTrend, exists := scaleMap[scaleCode]
		if !exists {
			scaleTrend = &response.ScaleTrendResponse{
				ScaleID:   "",
				ScaleCode: scaleCode,
				ScaleName: scaleName,
				Tests:     []response.ScaleTestResponse{},
			}
			if assessment.MedicalScaleID != nil {
				scaleTrend.ScaleID = strconv.FormatUint(*assessment.MedicalScaleID, 10)
			}
			scaleMap[scaleCode] = scaleTrend
		}

		// 获取测评得分
		var totalScore float64
		var riskLevel string
		var testDate time.Time
		if assessment.TotalScore != nil {
			totalScore = *assessment.TotalScore
		}
		if assessment.RiskLevel != nil {
			riskLevel = *assessment.RiskLevel
		}
		if assessment.InterpretedAt != nil {
			testDate = *assessment.InterpretedAt
		} else if assessment.SubmittedAt != nil {
			testDate = *assessment.SubmittedAt
		}

		// 查询因子得分
		factors := []response.ScaleFactorResponse{}
		scoreResult, err := h.scoreQueryService.GetByAssessmentID(c.Request.Context(), assessment.ID)
		if err == nil && scoreResult != nil {
			for _, factorScore := range scoreResult.FactorScores {
				factors = append(factors, response.ScaleFactorResponse{
					FactorCode:     factorScore.FactorCode,
					FactorName:     factorScore.FactorName,
					RawScore:       factorScore.RawScore,
					RiskLevel:      factorScore.RiskLevel,
					RiskLevelLabel: response.LabelForRiskLevel(factorScore.RiskLevel),
				})
			}
		}

		// 构建测评记录
		testRecord := response.ScaleTestResponse{
			AssessmentID:   strconv.FormatUint(assessment.ID, 10),
			TestDate:       response.FormatDateTimeValue(testDate),
			TotalScore:     totalScore,
			RiskLevel:      riskLevel,
			RiskLevelLabel: response.LabelForRiskLevel(riskLevel),
			Result:         "", // TODO: 从报告中获取结果描述
			Factors:        factors,
		}

		scaleTrend.Tests = append(scaleTrend.Tests, testRecord)
	}

	// 转换为列表并按时间排序
	scales := make([]response.ScaleTrendResponse, 0, len(scaleMap))
	for _, scaleTrend := range scaleMap {
		// 按测试日期升序排序
		for i := 0; i < len(scaleTrend.Tests)-1; i++ {
			for j := i + 1; j < len(scaleTrend.Tests); j++ {
				if scaleTrend.Tests[i].TestDate > scaleTrend.Tests[j].TestDate {
					scaleTrend.Tests[i], scaleTrend.Tests[j] = scaleTrend.Tests[j], scaleTrend.Tests[i]
				}
			}
		}
		scales = append(scales, *scaleTrend)
	}

	resp := &response.ScaleAnalysisResponse{
		Scales: scales,
	}

	h.Success(c, resp)
}

// GetPeriodicStats 获取受试者周期统计。
func (h *ActorHandler) GetPeriodicStats(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		logger.L(c.Request.Context()).Warnw("Invalid testee ID",
			"action", "get_periodic_stats",
			"testee_id", idStr,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}
	if _, _, err := h.validateProtectedTesteeAccess(c, id); err != nil {
		h.Error(c, err)
		return
	}

	// 验证受试者是否存在
	_, err = h.testeeQueryService.GetByID(c.Request.Context(), id)
	if err != nil {
		logger.L(c.Request.Context()).Errorw("Failed to get testee",
			"action", "get_periodic_stats",
			"testee_id", id,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	// TODO: 查询该受试者参与的周期性测评项目
	// 当前返回空数据结构
	resp := &response.PeriodicStatsResponse{
		Projects:       []response.PeriodicProjectResponse{},
		TotalProjects:  0,
		ActiveProjects: 0,
	}

	h.Success(c, resp)
}

// UpdateTestee 更新受试者
// @Summary 更新受试者
// @Tags Actor
// @Accept json
// @Produce json
// @Param id path string true "受试者ID"
// @Param body body request.UpdateTesteeRequest true "更新受试者请求"
// @Success 200 {object} core.Response{data=response.TesteeResponse}
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/testees/{id} [put]
// UpdateTestee 更新受试者
func (h *ActorHandler) UpdateTestee(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		logger.L(c.Request.Context()).Warnw("Invalid testee ID",
			"action", "update_testee",
			"testee_id", idStr,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}
	if _, _, err := h.validateProtectedTesteeAccess(c, id); err != nil {
		h.Error(c, err)
		return
	}

	var req request.UpdateTesteeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.L(c.Request.Context()).Warnw("Invalid request",
			"action", "update_testee",
			"testee_id", id,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	// 使用管理服务更新基本信息（B端员工操作）
	if (req.Name != nil && *req.Name != "") || req.Gender != nil || req.Birthday != nil {
		dto := toUpdateTesteeProfileDTO(id, &req)
		err = h.testeeManagementService.UpdateBasicInfo(c.Request.Context(), dto)
		if err != nil {
			logger.L(c.Request.Context()).Errorw("Failed to update testee profile",
				"action", "update_testee",
				"resource", "testee",
				"testee_id", id,
				"error", err.Error(),
			)
			h.Error(c, err)
			return
		}
	}

	// 更新标签（如果提供）
	// 注意：旧接口使用覆盖式更新，新服务使用增量操作
	// 这里简化处理，只在有标签时调用
	// TODO: 可能需要先获取现有标签进行比较，实现完整的覆盖逻辑

	// 更新重点关注状态
	if req.IsKeyFocus != nil {
		if *req.IsKeyFocus {
			err = h.testeeManagementService.MarkAsKeyFocus(c.Request.Context(), id)
		} else {
			err = h.testeeManagementService.UnmarkKeyFocus(c.Request.Context(), id)
		}
		if err != nil {
			logger.L(c.Request.Context()).Errorw("Failed to update key focus status",
				"action", "update_testee",
				"resource", "testee",
				"testee_id", id,
				"field", "is_key_focus",
				"error", err.Error(),
			)
			h.Error(c, err)
			return
		}
	}

	// 查询更新后的结果
	result, err := h.testeeQueryService.GetByID(c.Request.Context(), id)
	if err != nil {
		logger.L(c.Request.Context()).Errorw("Failed to get updated testee",
			"action", "update_testee",
			"resource", "testee",
			"testee_id", id,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "受试者更新成功", toTesteeResponse(result))
}

// ListTestees 查询受试者列表
// @Summary 查询受试者列表
// @Description 以 JWT org_id 为准查询后台可见受试者。qs:admin 可查看机构全量，其他后台用户按 ClinicianTesteeRelation 自动收口；org_id 查询参数仅作兼容校验
// @Tags Actor
// @Produce json
// @Param org_id query string false "兼容字段：机构ID，若传入必须与 JWT org_id 一致"
// @Param name query string false "姓名（模糊匹配）"
// @Param is_key_focus query bool false "是否重点关注"
// @Param profile_id query string false "档案ID（等同于IAM儿童ID）"
// @Param clinician_id query string false "按 Clinician 过滤受试者"
// @Param created_start_date query string false "报到开始日期（格式：YYYY-MM-DD，按 created_at 过滤）"
// @Param created_end_date query string false "报到结束日期（格式：YYYY-MM-DD，按 created_at 过滤）"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} core.Response{data=response.TesteeListResponse}
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/testees [get]
func (h *ActorHandler) ListTestees(c *gin.Context) {
	_, operatorUserID, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	var req request.ListTesteeRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		logger.L(c.Request.Context()).Warnw("Invalid list testees request",
			"action", "list_testees",
			"resource", "testee",
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}
	orgID, err := h.RequireProtectedOrgIDWithLegacy(c, req.OrgID)
	if err != nil {
		h.Error(c, err)
		return
	}
	createdAtStart, createdAtEnd, err := parseInclusiveLocalDateRange(req.CreatedStartDate, req.CreatedEndDate)
	if err != nil {
		h.Error(c, err)
		return
	}

	// 设置默认值
	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 20
	}

	// 按 profile_id 精确查询（等同于 IAM child_id）
	if req.ProfileID != "" {
		result, err := h.fetchTesteeByProfile(c, orgID, req.ProfileID)
		if err != nil {
			h.Error(c, err)
			return
		}
		if err := h.testeeAccessService.ValidateTesteeAccess(c.Request.Context(), orgID, operatorUserID, result.ID); err != nil {
			h.Error(c, err)
			return
		}
		if req.ClinicianID != nil {
			if _, err := h.requireClinicianInOrg(c, orgID, *req.ClinicianID); err != nil {
				h.Error(c, err)
				return
			}
			clinicianTesteeIDs, err := h.clinicianRelationshipService.ListAssignedTesteeIDs(c.Request.Context(), orgID, *req.ClinicianID)
			if err != nil {
				h.Error(c, err)
				return
			}
			if !containsUint64(clinicianTesteeIDs, result.ID) {
				h.Success(c, toTesteeListResponse([]*testeeApp.TesteeResult{}, 0, req.Page, req.PageSize))
				return
			}
		}
		if !testeeMatchesListFilter(result, req, createdAtStart, createdAtEnd) {
			h.Success(c, toTesteeListResponse([]*testeeApp.TesteeResult{}, 0, req.Page, req.PageSize))
			return
		}

		h.Success(c, toTesteeListResponse([]*testeeApp.TesteeResult{result}, 1, req.Page, req.PageSize))
		return
	}

	scope, err := h.testeeAccessService.ResolveAccessScope(c.Request.Context(), orgID, operatorUserID)
	if err != nil {
		h.Error(c, err)
		return
	}

	// 使用查询服务 - 统一通过 ListTestees 方法处理所有查询
	dto := testeeApp.ListTesteeDTO{
		OrgID:          orgID,
		Name:           req.Name,
		Tags:           req.Tags,
		KeyFocus:       req.IsKeyFocus,
		CreatedAtStart: createdAtStart,
		CreatedAtEnd:   createdAtEnd,
		Offset:         (req.Page - 1) * req.PageSize,
		Limit:          req.PageSize,
	}
	if req.ClinicianID != nil {
		if _, err := h.requireClinicianInOrg(c, orgID, *req.ClinicianID); err != nil {
			h.Error(c, err)
			return
		}
		clinicianTesteeIDs, err := h.clinicianRelationshipService.ListAssignedTesteeIDs(c.Request.Context(), orgID, *req.ClinicianID)
		if err != nil {
			h.Error(c, err)
			return
		}
		dto.AccessibleTesteeIDs = clinicianTesteeIDs
		dto.RestrictToAccessScope = true
	}
	if !scope.IsAdmin {
		allowedTesteeIDs, err := h.testeeAccessService.ListAccessibleTesteeIDs(c.Request.Context(), orgID, operatorUserID)
		if err != nil {
			h.Error(c, err)
			return
		}
		if dto.RestrictToAccessScope {
			dto.AccessibleTesteeIDs = intersectUint64Slices(dto.AccessibleTesteeIDs, allowedTesteeIDs)
		} else {
			dto.AccessibleTesteeIDs = allowedTesteeIDs
		}
		dto.RestrictToAccessScope = true
	}

	listResult, err := h.testeeQueryService.ListTestees(c.Request.Context(), dto)
	if err != nil {
		logger.L(c.Request.Context()).Errorw("Failed to list testees",
			"action", "list_testees",
			"resource", "testee",
			"org_id", dto.OrgID,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	h.Success(c, toTesteeListResponse(listResult.Items, listResult.TotalCount, req.Page, req.PageSize))
}

func parseInclusiveLocalDateRange(startRaw, endRaw string) (*time.Time, *time.Time, error) {
	var start, end *time.Time
	if strings.TrimSpace(startRaw) != "" {
		parsed, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(startRaw), time.Local)
		if err != nil {
			return nil, nil, errors.WithCode(code.ErrInvalidArgument, "created_start_date 格式无效，必须为 YYYY-MM-DD")
		}
		start = &parsed
	}
	if strings.TrimSpace(endRaw) != "" {
		parsed, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(endRaw), time.Local)
		if err != nil {
			return nil, nil, errors.WithCode(code.ErrInvalidArgument, "created_end_date 格式无效，必须为 YYYY-MM-DD")
		}
		nextDay := parsed.AddDate(0, 0, 1)
		end = &nextDay
	}
	if start != nil && end != nil && !start.Before(*end) {
		return nil, nil, errors.WithCode(code.ErrInvalidArgument, "created_start_date 不能晚于 created_end_date")
	}
	return start, end, nil
}

func createdAtInRange(createdAt time.Time, start, end *time.Time) bool {
	if start != nil && createdAt.Before(*start) {
		return false
	}
	if end != nil && !createdAt.Before(*end) {
		return false
	}
	return true
}

func testeeMatchesListFilter(
	result *testeeApp.TesteeResult,
	req request.ListTesteeRequest,
	createdAtStart, createdAtEnd *time.Time,
) bool {
	if result == nil {
		return false
	}
	if req.Name != "" && !strings.Contains(strings.ToLower(result.Name), strings.ToLower(req.Name)) {
		return false
	}
	if req.IsKeyFocus != nil && result.IsKeyFocus != *req.IsKeyFocus {
		return false
	}
	if len(req.Tags) > 0 {
		tagSet := make(map[string]struct{}, len(result.Tags))
		for _, tag := range result.Tags {
			tagSet[tag] = struct{}{}
		}
		for _, want := range req.Tags {
			if _, ok := tagSet[want]; !ok {
				return false
			}
		}
	}
	return createdAtInRange(result.CreatedAt, createdAtStart, createdAtEnd)
}

// ========== Staff API ==========

// CreateStaff 创建员工
// @Summary 创建员工
// @Tags Actor
// @Accept json
// @Produce json
// @Param body body request.CreateStaffRequest true "创建员工请求"
// @Success 200 {object} core.Response{data=response.StaffResponse}
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/staff [post]
func (h *ActorHandler) CreateStaff(c *gin.Context) {
	var req request.CreateStaffRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.L(c.Request.Context()).Warnw("Invalid create staff request",
			"action", "create_staff",
			"resource", "staff",
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}
	orgID, err := h.RequireProtectedOrgIDWithLegacy(c, req.OrgID)
	if err != nil {
		h.Error(c, err)
		return
	}

	dto := toRegisterStaffDTO(&req, orgID)
	// 使用生命周期服务 - 服务于人事/行放部门
	result, err := h.operatorLifecycleService.Register(c.Request.Context(), dto)
	if err != nil {
		logger.L(c.Request.Context()).Errorw("Failed to create staff",
			"action", "create_staff",
			"resource", "staff",
			"org_id", dto.OrgID,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "员工创建成功", toStaffResponse(result))
}

// GetStaff 获取员工详情
// @Summary 获取员工详情
// @Tags Actor
// @Produce json
// @Param id path string true "员工ID"
// @Success 200 {object} core.Response{data=response.StaffResponse}
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/staff/{id} [get]
func (h *ActorHandler) GetStaff(c *gin.Context) {
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		logger.L(c.Request.Context()).Warnw("Invalid staff ID",
			"action", "get_staff",
			"resource", "staff",
			"staff_id", idStr,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	// 使用查询服务
	result, err := h.operatorQueryService.GetByID(c.Request.Context(), id)
	if err != nil {
		logger.L(c.Request.Context()).Errorw("Failed to get staff",
			"action", "get_staff",
			"resource", "staff",
			"staff_id", id,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}
	if result.OrgID != orgID {
		h.Error(c, errors.WithCode(code.ErrPermissionDenied, "operator does not belong to current organization"))
		return
	}

	h.Success(c, toStaffResponse(result))
}

// UpdateStaff 更新员工
// @Summary 更新员工
// @Tags Actor
// @Accept json
// @Produce json
// @Param id path string true "员工ID"
// @Param body body request.UpdateStaffRequest true "更新员工请求"
// @Success 200 {object} core.Response{data=response.StaffResponse}
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/staff/{id} [put]
func (h *ActorHandler) UpdateStaff(c *gin.Context) {
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		logger.L(c.Request.Context()).Warnw("Invalid staff ID",
			"action", "update_staff",
			"resource", "staff",
			"staff_id", idStr,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	current, err := h.operatorQueryService.GetByID(c.Request.Context(), id)
	if err != nil {
		h.Error(c, err)
		return
	}
	if current.OrgID != orgID {
		h.Error(c, errors.WithCode(code.ErrPermissionDenied, "operator does not belong to current organization"))
		return
	}

	var req request.UpdateStaffRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.L(c.Request.Context()).Warnw("Invalid update staff request",
			"action", "update_staff",
			"resource", "staff",
			"staff_id", id,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	if _, err := h.operatorLifecycleService.UpdateProfile(c.Request.Context(), operatorApp.UpdateOperatorProfileDTO{
		OperatorID: id,
		Name:       req.Name,
		Email:      req.Email,
		Phone:      req.Phone,
	}); err != nil {
		h.Error(c, err)
		return
	}

	targetActive := current.IsActive
	if req.IsActive != nil {
		targetActive = *req.IsActive
	}

	if !targetActive {
		if current.IsActive {
			if err := h.operatorAuthorizationService.Deactivate(c.Request.Context(), id); err != nil {
				h.Error(c, err)
				return
			}
		}
	} else {
		if !current.IsActive {
			if err := h.operatorAuthorizationService.Activate(c.Request.Context(), id); err != nil {
				h.Error(c, err)
				return
			}
		}

		latest, err := h.operatorQueryService.GetByID(c.Request.Context(), id)
		if err != nil {
			h.Error(c, err)
			return
		}

		if req.Roles != nil {
			currentRoles := make(map[string]struct{}, len(latest.Roles))
			for _, role := range latest.Roles {
				currentRoles[role] = struct{}{}
			}
			targetRoles := make(map[string]struct{}, len(req.Roles))
			for _, role := range req.Roles {
				targetRoles[role] = struct{}{}
				if _, exists := currentRoles[role]; !exists {
					if err := h.operatorAuthorizationService.AssignRole(c.Request.Context(), id, role); err != nil {
						h.Error(c, err)
						return
					}
				}
			}
			for _, role := range latest.Roles {
				if _, exists := targetRoles[role]; !exists {
					if err := h.operatorAuthorizationService.RemoveRole(c.Request.Context(), id, role); err != nil {
						h.Error(c, err)
						return
					}
				}
			}
		}
	}

	result, err := h.operatorQueryService.GetByID(c.Request.Context(), id)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "员工更新成功", toStaffResponse(result))
}

// DeleteStaff 删除员工
// @Summary 删除员工
// @Tags Actor
// @Produce json
// @Param id path string true "员工ID"
// @Success 200 {object} core.Response
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/staff/{id} [delete]
func (h *ActorHandler) DeleteStaff(c *gin.Context) {
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		logger.L(c.Request.Context()).Warnw("Invalid staff ID",
			"action", "delete_staff",
			"resource", "staff",
			"staff_id", idStr,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}
	result, err := h.operatorQueryService.GetByID(c.Request.Context(), id)
	if err != nil {
		h.Error(c, err)
		return
	}
	if result.OrgID != orgID {
		h.Error(c, errors.WithCode(code.ErrPermissionDenied, "operator does not belong to current organization"))
		return
	}

	if clinicianItem, err := h.clinicianQueryService.GetByOperator(c.Request.Context(), orgID, id); err == nil && clinicianItem != nil {
		h.Error(c, errors.WithCode(code.ErrValidation, "员工已绑定临床人员，请先解绑"))
		return
	} else if err != nil && !errors.IsCode(err, code.ErrUserNotFound) {
		h.Error(c, err)
		return
	}

	// 使用生命周期服务 - 服务于人事/行政部门
	if err := h.operatorLifecycleService.Delete(c.Request.Context(), id); err != nil {
		logger.L(c.Request.Context()).Errorw("Failed to delete staff",
			"action", "delete_staff",
			"resource", "staff",
			"staff_id", id,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "员工删除成功", nil)
}

// ListStaff 查询员工列表
// @Summary 查询员工列表
// @Description 机构级后台账号列表，仅 qs:admin 可访问；org_id 查询参数仅作兼容校验
// @Tags Actor
// @Produce json
// @Param org_id query string false "兼容字段：机构ID，若传入必须与 JWT org_id 一致"
// @Param role query string false "角色筛选"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} core.Response{data=response.StaffListResponse}
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/staff [get]
func (h *ActorHandler) ListStaff(c *gin.Context) {
	var req request.ListStaffRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		logger.L(c.Request.Context()).Warnw("Invalid list staff request",
			"action", "list_staff",
			"resource", "staff",
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}
	orgID, err := h.RequireProtectedOrgIDWithLegacy(c, req.OrgID)
	if err != nil {
		h.Error(c, err)
		return
	}

	// 设置默认值
	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 20
	}

	offset := (req.Page - 1) * req.PageSize

	// 根据查询条件调用不同的服务方法
	var results []*operatorApp.OperatorResult
	var total int64

	// 使用查询服务
	listDTO := operatorApp.ListOperatorDTO{
		OrgID:  orgID,
		Role:   req.Role,
		Offset: offset,
		Limit:  req.PageSize,
	}

	listResult, err := h.operatorQueryService.ListOperators(c.Request.Context(), listDTO)
	if err != nil {
		logger.L(c.Request.Context()).Errorw("Failed to list staff",
			"action", "list_staff",
			"resource", "staff",
			"org_id", listDTO.OrgID,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	results = listResult.Items
	total = listResult.TotalCount

	h.Success(c, toStaffListResponse(results, total, req.Page, req.PageSize))
}

// fetchTesteeByProfile 提取的 profile_id 查询逻辑
func (h *ActorHandler) fetchTesteeByProfile(c *gin.Context, orgID int64, profileIDStr string) (*testeeApp.TesteeResult, error) {
	childID, err := strconv.ParseUint(profileIDStr, 10, 64)
	if err != nil {
		logger.L(c.Request.Context()).Warnw("Invalid profile_id format",
			"action", "fetch_testee_by_profile",
			"org_id", orgID,
			"profile_id", profileIDStr,
			"error", err.Error(),
		)
		return nil, err
	}

	result, err := h.testeeQueryService.FindByProfile(c.Request.Context(), orgID, childID)
	if err != nil {
		logger.L(c.Request.Context()).Errorw("Failed to find testee by profile_id",
			"action", "fetch_testee_by_profile",
			"org_id", orgID,
			"profile_id", childID,
			"error", err.Error(),
		)
		return nil, err
	}

	return result, nil
}

func (h *ActorHandler) validateProtectedTesteeAccess(c *gin.Context, testeeID uint64) (int64, int64, error) {
	orgID, operatorUserID, err := h.RequireProtectedScope(c)
	if err != nil {
		return 0, 0, err
	}
	if err := h.testeeAccessService.ValidateTesteeAccess(c.Request.Context(), orgID, operatorUserID, testeeID); err != nil {
		return 0, 0, err
	}
	return orgID, operatorUserID, nil
}

// ========== 映射辅助函数 ==========

// toUpdateTesteeProfileDTO 将更新请求转换为应用层 UpdateTesteeProfileDTO
func toUpdateTesteeProfileDTO(testeeID uint64, req *request.UpdateTesteeRequest) testeeApp.UpdateTesteeProfileDTO {
	var gender int8
	if req.Gender != nil {
		switch *req.Gender {
		case "male", "男":
			gender = 1
		case "female", "女":
			gender = 2
		default:
			gender = 0
		}
	}

	var name string
	if req.Name != nil {
		name = *req.Name
	}

	return testeeApp.UpdateTesteeProfileDTO{
		TesteeID: testeeID,
		Name:     name,
		Gender:   gender,
		Birthday: req.Birthday,
	}
}

// TODO: Create Testee API 已废弃，未来将通过 Registration Service 实现

// toTesteeResponse 将应用层结果转换为响应
func toTesteeResponse(result *testeeApp.TesteeResult) *response.TesteeResponse {
	gender := response.GenderCodeFromValue(result.Gender)

	// 转换 ID 字段为字符串
	idStr := fmt.Sprintf("%d", result.ID)
	orgIDStr := fmt.Sprintf("%d", result.OrgID)
	var profileIDStr *string
	if result.ProfileID != nil {
		s := fmt.Sprintf("%d", *result.ProfileID)
		profileIDStr = &s
	}

	resp := &response.TesteeResponse{
		ID:              idStr,
		OrgID:           orgIDStr,
		ProfileID:       profileIDStr,
		IAMChildID:      response.LegacyIAMChildIDAlias(profileIDStr),
		Name:            result.Name,
		Gender:          gender,
		GenderLabel:     response.LabelForGender(gender),
		Birthday:        response.FormatDatePtr(result.Birthday),
		Tags:            result.Tags,
		TagsLabel:       response.LabelTags(result.Tags),
		Source:          result.Source,
		SourceLabel:     response.LabelForTesteeSource(result.Source),
		IsKeyFocus:      result.IsKeyFocus,
		IsKeyFocusLabel: response.LabelForKeyFocus(result.IsKeyFocus),
		CreatedAt:       response.FormatDateTimeValue(result.CreatedAt),
		UpdatedAt:       response.FormatDateTimeValue(result.UpdatedAt),
	}

	// 测评统计信息
	if result.LastAssessmentAt != nil || result.TotalAssessments > 0 || result.LastRiskLevel != "" {
		resp.AssessmentStats = &response.AssessmentStatsResponse{
			TotalCount:         result.TotalAssessments,
			LastAssessmentAt:   response.FormatDateTimePtr(result.LastAssessmentAt),
			LastRiskLevel:      result.LastRiskLevel,
			LastRiskLevelLabel: response.LabelForRiskLevel(result.LastRiskLevel),
		}
	}

	return resp
}

// toTesteeBackendResponse 将后台查询结果转换为响应（包含家长信息）
func toTesteeBackendResponse(backendResult *testeeApp.TesteeBackendResult) *response.TesteeResponse {
	// 先转换基础信息
	resp := toTesteeResponse(backendResult.TesteeResult)

	// 添加家长信息
	if len(backendResult.Guardians) > 0 {
		resp.Guardians = make([]response.GuardianResponse, 0, len(backendResult.Guardians))
		for _, guardian := range backendResult.Guardians {
			resp.Guardians = append(resp.Guardians, response.GuardianResponse{
				Name:     guardian.Name,
				Relation: guardian.Relation,
				Phone:    guardian.Phone,
			})
		}
	}

	return resp
}

// toTesteeListResponse 将应用层列表结果转换为响应
func toTesteeListResponse(results []*testeeApp.TesteeResult, total int64, page, pageSize int) *response.TesteeListResponse {
	items := make([]*response.TesteeResponse, 0, len(results))
	for _, result := range results {
		items = append(items, toTesteeResponse(result))
	}

	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	return &response.TesteeListResponse{
		Items:      items,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}
}

// toRegisterStaffDTO 将创建请求转换为应用层 DTO
func toRegisterStaffDTO(req *request.CreateStaffRequest, orgID int64) operatorApp.RegisterOperatorDTO {
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	return operatorApp.RegisterOperatorDTO{
		OrgID:    orgID,
		UserID:   req.UserID.Int64(),
		Roles:    req.Roles,
		Name:     req.Name,
		Email:    req.Email,
		Phone:    req.Phone,
		Password: req.Password,
		IsActive: isActive,
	}
}

// toStaffResponse 将应用层结果转换为响应
func toStaffResponse(result *operatorApp.OperatorResult) *response.StaffResponse {
	return &response.StaffResponse{
		ID:       fmt.Sprintf("%d", result.ID),
		OrgID:    fmt.Sprintf("%d", result.OrgID),
		UserID:   fmt.Sprintf("%d", result.UserID),
		Roles:    result.Roles,
		Name:     result.Name,
		Email:    result.Email,
		Phone:    result.Phone,
		IsActive: result.IsActive,
	}
}

// toStaffListResponse 将应用层列表结果转换为响应
func toStaffListResponse(results []*operatorApp.OperatorResult, total int64, page, pageSize int) *response.StaffListResponse {
	items := make([]*response.StaffResponse, 0, len(results))
	for _, result := range results {
		items = append(items, toStaffResponse(result))
	}

	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	return &response.StaffListResponse{
		Items:      items,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}
}

// SetEvaluationServices 设置评估服务（用于延迟注入）
func (h *ActorHandler) SetEvaluationServices(
	assessmentManagementService assessmentApp.AssessmentManagementService,
	scoreQueryService assessmentApp.ScoreQueryService,
) {
	h.assessmentManagementService = assessmentManagementService
	h.scoreQueryService = scoreQueryService
}

// SetQRCodeService 设置二维码服务。
func (h *ActorHandler) SetQRCodeService(qrCodeService qrcodeApp.QRCodeService) {
	h.qrCodeService = qrCodeService
}

func containsUint64(items []uint64, target uint64) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func intersectUint64Slices(left, right []uint64) []uint64 {
	if len(left) == 0 || len(right) == 0 {
		return []uint64{}
	}

	set := make(map[uint64]struct{}, len(right))
	for _, item := range right {
		set[item] = struct{}{}
	}

	result := make([]uint64, 0, len(left))
	for _, item := range left {
		if _, ok := set[item]; ok {
			result = append(result, item)
		}
	}
	return result
}
