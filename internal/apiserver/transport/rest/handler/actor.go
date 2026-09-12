package handler

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	actorAccessApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	clinicianApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/clinician"
	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	evaluationoperator "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/operator"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/request"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/response"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/gin-gonic/gin"
)

// TesteeHandler 负责 testee 相关的 HTTP 入口。
type TesteeHandler struct {
	*BaseHandler
	testeeManagementService      testeeApp.TesteeManagementService
	testeeQueryService           testeeApp.TesteeQueryService
	testeeBackendQueryService    testeeApp.TesteeBackendQueryService
	clinicianQueryService        clinicianApp.ClinicianQueryService
	clinicianRelationshipService clinicianApp.ClinicianRelationshipService
	testeeAccessService          actorAccessApp.TesteeAccessService
	scaleAnalysisQueryService    evaluationoperator.ScaleAnalysisService
}

type testeeListQuery struct {
	Request        request.ListTesteeRequest
	OrgID          int64
	Page           int
	PageSize       int
	CreatedAtStart *time.Time
	CreatedAtEnd   *time.Time
}

func NewTesteeHandler(
	testeeManagementService testeeApp.TesteeManagementService,
	testeeQueryService testeeApp.TesteeQueryService,
	testeeBackendQueryService testeeApp.TesteeBackendQueryService,
	clinicianQueryService clinicianApp.ClinicianQueryService,
	clinicianRelationshipService clinicianApp.ClinicianRelationshipService,
	testeeAccessService actorAccessApp.TesteeAccessService,
	scaleAnalysisQueryService evaluationoperator.ScaleAnalysisService,
) *TesteeHandler {
	return &TesteeHandler{
		BaseHandler:                  NewBaseHandler(),
		testeeManagementService:      testeeManagementService,
		testeeQueryService:           testeeQueryService,
		testeeBackendQueryService:    testeeBackendQueryService,
		clinicianQueryService:        clinicianQueryService,
		clinicianRelationshipService: clinicianRelationshipService,
		testeeAccessService:          testeeAccessService,
		scaleAnalysisQueryService:    scaleAnalysisQueryService,
	}
}

// GetTestee 获取受试者详情（后台管理接口，包含家长信息）。
// @Summary 获取受试者详情
// @Description 根据受试者 ID 获取受试者详情
// @Tags 受试者
// @Produce json
// @Param id path int true "受试者ID"
// @Success 200 {object} core.Response{data=response.TesteeResponse}
// @Failure 400 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security BearerAuth
// @Router /api/v1/testees/{id} [get]
func (h *TesteeHandler) GetTestee(c *gin.Context) {
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

	orgID, operatorUserID, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	if err := h.validateTesteeReadScope(c, orgID, operatorUserID, id); err != nil {
		h.Error(c, err)
		return
	}

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

	h.Success(c, toTesteeBackendResponse(backendResult))
}

// GetTesteeByProfileID 根据 profile_id 获取受试者详情。
// @Summary 根据 profile_id 获取受试者
// @Tags 受试者
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param profile_id query string true "Profile ID"
// @Param org_id query int false "机构 ID"
// @Success 200 {object} core.Response{data=response.TesteeResponse}
// @Router /api/v1/testees/by-profile-id [get]
func (h *TesteeHandler) GetTesteeByProfileID(c *gin.Context) {
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

	profileIDStr := req.ProfileID
	if profileIDStr == "" {
		h.BadRequestResponse(c, "profile_id is required", nil)
		return
	}

	testeeResult, err := h.fetchTesteeByProfile(c, orgID, profileIDStr)
	if err != nil {
		h.Error(c, err)
		return
	}
	if err := h.validateTesteeReadScope(c, orgID, operatorUserID, testeeResult.ID); err != nil {
		h.Error(c, err)
		return
	}

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

// GetScaleAnalysis 获取受试者量表分析结果。
// @Summary 获取受试者量表分析
// @Tags 受试者
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path int true "受试者ID"
// @Success 200 {object} core.Response
// @Router /api/v1/testees/{id}/scale-analysis [get]
func (h *TesteeHandler) GetScaleAnalysis(c *gin.Context) {
	id, err := h.parseTesteeIDParam(c, "get_scale_analysis")
	if err != nil {
		h.Error(c, err)
		return
	}
	orgID, operatorUserID, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.scaleAnalysisQueryService.GetScaleAnalysis(c.Request.Context(), evaluationoperator.Actor{OrgID: orgID, OperatorUserID: operatorUserID}, id)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, toScaleAnalysisResponse(result))
}

// UpdateTestee 更新受试者。
// @Summary 更新受试者
// @Tags 受试者
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path int true "受试者ID"
// @Param request body request.UpdateTesteeRequest true "更新受试者请求"
// @Success 200 {object} core.Response{data=response.TesteeUpdateResponse}
// @Router /api/v1/testees/{id} [put]
func (h *TesteeHandler) UpdateTestee(c *gin.Context) {
	id, err := h.parseTesteeIDParam(c, "update_testee")
	if err != nil {
		h.Error(c, err)
		return
	}
	if _, _, err := h.RequireProtectedScope(c); err != nil {
		h.Error(c, err)
		return
	}
	var req request.UpdateTesteeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Error(c, err)
		return
	}
	updater, ok := h.testeeManagementService.(testeeApp.ProfileUpdater)
	if !ok {
		h.Error(c, errors.WithCode(code.ErrInternalServerError, "atomic profile update unavailable"))
		return
	}
	dto := testeeApp.UpdateProfileDTO{TesteeID: id, Name: req.Name, Birthday: req.Birthday, IsKeyFocus: req.IsKeyFocus}
	if req.Gender != nil {
		value := toUpdateTesteeProfileDTO(id, &req).Gender
		dto.Gender = &value
	}
	if err := updater.UpdateProfile(c.Request.Context(), dto); err != nil {
		h.Error(c, err)
		return
	}
	h.SuccessResponseWithMessage(c, "受试者更新成功", &response.TesteeUpdateResponse{ID: strconv.FormatUint(id, 10), Updated: true})
}

// ListTestees 查询受试者列表。
// @Summary 查询受试者列表
// @Param store_id query string false "当前服务门店ID，与 unassigned_store=true 互斥"
// @Param unassigned_store query boolean false "仅查询未归属门店的受试者"
// @Tags 受试者
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Success 200 {object} core.Response{data=response.TesteeListResponse}
// @Router /api/v1/testees [get]
func (h *TesteeHandler) ListTestees(c *gin.Context) {
	_, _, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	query, err := h.parseTesteeListQuery(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	dto, err := h.buildTesteeListDTO(c, query)
	if err != nil {
		h.Error(c, err)
		return
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

	h.Success(c, toTesteeListResponse(listResult.Items, listResult.TotalCount, query.Page, query.PageSize))
}

func (h *TesteeHandler) fetchTesteeByProfile(c *gin.Context, orgID int64, profileIDStr string) (*testeeApp.TesteeResult, error) {
	profileID, err := strconv.ParseUint(profileIDStr, 10, 64)
	if err != nil {
		logger.L(c.Request.Context()).Warnw("Invalid profile_id format",
			"action", "fetch_testee_by_profile",
			"org_id", orgID,
			"profile_id", profileIDStr,
			"error", err.Error(),
		)
		return nil, err
	}

	result, err := h.testeeQueryService.FindByProfile(c.Request.Context(), orgID, profileID)
	if err != nil {
		logger.L(c.Request.Context()).Errorw("Failed to find testee by profile_id",
			"action", "fetch_testee_by_profile",
			"org_id", orgID,
			"profile_id", profileID,
			"error", err.Error(),
		)
		return nil, err
	}

	return result, nil
}

func (h *TesteeHandler) parseTesteeIDParam(c *gin.Context, action string) (uint64, error) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		logger.L(c.Request.Context()).Warnw("Invalid testee ID",
			"action", action,
			"testee_id", idStr,
			"error", err.Error(),
		)
		return 0, err
	}
	return id, nil
}

func (h *TesteeHandler) parseTesteeListQuery(c *gin.Context) (*testeeListQuery, error) {
	var req request.ListTesteeRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		logger.L(c.Request.Context()).Warnw("Invalid list testees request",
			"action", "list_testees",
			"resource", "testee",
			"error", err.Error(),
		)
		return nil, err
	}

	if req.StoreID != nil && req.UnassignedStore {
		return nil, errors.WithCode(code.ErrInvalidArgument, "store_id and unassigned_store are mutually exclusive")
	}
	orgID, err := h.RequireProtectedOrgIDWithLegacy(c, req.OrgID)
	if err != nil {
		return nil, err
	}
	createdAtStart, createdAtEnd, err := parseInclusiveLocalDateRange(req.CreatedStartDate, req.CreatedEndDate)
	if err != nil {
		return nil, err
	}
	page, pageSize := normalizePageRequest(req.Page, req.PageSize, 1, 20)

	return &testeeListQuery{
		Request:        req,
		OrgID:          orgID,
		Page:           page,
		PageSize:       pageSize,
		CreatedAtStart: createdAtStart,
		CreatedAtEnd:   createdAtEnd,
	}, nil
}

func (h *TesteeHandler) buildTesteeListDTO(c *gin.Context, query *testeeListQuery) (testeeApp.ListTesteeDTO, error) {
	dto := testeeApp.ListTesteeDTO{
		StoreID: query.Request.StoreID, UnassignedStore: query.Request.UnassignedStore,
		OrgID:          query.OrgID,
		Name:           query.Request.Name,
		KeyFocus:       query.Request.IsKeyFocus,
		CreatedAtStart: query.CreatedAtStart,
		CreatedAtEnd:   query.CreatedAtEnd,
		Offset:         (query.Page - 1) * query.PageSize,
		Limit:          query.PageSize,
	}

	if query.Request.ProfileID != "" {
		id, err := strconv.ParseUint(query.Request.ProfileID, 10, 64)
		if err != nil || id == 0 {
			return testeeApp.ListTesteeDTO{}, errors.WithCode(code.ErrInvalidArgument, "invalid profile_id")
		}
		dto.ProfileID = &id
	}

	clinicianTesteeIDs, restrictToClinicianScope, err := h.resolveClinicianScopedTesteeIDs(c, query.OrgID, query.Request.ClinicianID)
	if err != nil {
		return testeeApp.ListTesteeDTO{}, err
	}
	dto.AccessibleTesteeIDs = clinicianTesteeIDs
	dto.RestrictToAccessScope = restrictToClinicianScope

	// The shared query service applies the action-specific store range before
	// count and pagination; clinician selection remains an intersecting filter.

	return dto, nil
}

func (h *TesteeHandler) resolveClinicianScopedTesteeIDs(c *gin.Context, orgID int64, clinicianID *uint64) ([]uint64, bool, error) {
	if clinicianID == nil {
		return nil, false, nil
	}
	// This is a Testee filter, not access to the headquarters clinician directory.
	// The backstage relationship use case enforces company and action/store scope.
	clinicianTesteeIDs, err := h.clinicianRelationshipService.ListAssignedTesteeIDs(c.Request.Context(), orgID, *clinicianID)
	if err != nil {
		return nil, false, err
	}
	return clinicianTesteeIDs, true, nil
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

func normalizePageRequest(page, pageSize, defaultPage, defaultPageSize int) (int, int) {
	if page == 0 {
		page = defaultPage
	}
	if pageSize == 0 {
		pageSize = defaultPageSize
	}
	return page, pageSize
}

func mergeAccessibleTesteeIDs(existing []uint64, restrictExisting bool, allowed []uint64) ([]uint64, bool) {
	if restrictExisting {
		return intersectUint64Slices(existing, allowed), true
	}
	return allowed, true
}

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

func toTesteeResponse(result *testeeApp.TesteeResult) *response.TesteeResponse {
	gender := response.GenderCodeFromValue(result.Gender)

	idStr := fmt.Sprintf("%d", result.ID)
	orgIDStr := fmt.Sprintf("%d", result.OrgID)
	var profileIDStr *string
	if result.ProfileID != nil {
		s := fmt.Sprintf("%d", *result.ProfileID)
		profileIDStr = &s
	}

	resp := &response.TesteeResponse{
		StoreID: uint64StringPtr(result.StoreID), StoreVersion: result.StoreVersion,
		ID:              idStr,
		OrgID:           orgIDStr,
		ProfileID:       profileIDStr,
		Name:            result.Name,
		Gender:          gender,
		GenderLabel:     response.LabelForGender(gender),
		Birthday:        response.FormatDatePtr(result.Birthday),
		Source:          result.Source,
		SourceLabel:     response.LabelForTesteeSource(result.Source),
		IsKeyFocus:      result.IsKeyFocus,
		IsKeyFocusLabel: response.LabelForKeyFocus(result.IsKeyFocus),
		CreatedAt:       response.FormatDateTimeValue(result.CreatedAt),
		UpdatedAt:       response.FormatDateTimeValue(result.UpdatedAt),
	}

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

func toTesteeBackendResponse(backendResult *testeeApp.TesteeBackendResult) *response.TesteeResponse {
	resp := toTesteeResponse(backendResult.TesteeResult)

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

func toScaleAnalysisResponse(result *evaluationoperator.ScaleAnalysis) *response.ScaleAnalysisResponse {
	resp := &response.ScaleAnalysisResponse{Scales: []response.ScaleTrendResponse{}}
	if result == nil {
		return resp
	}
	resp.Scales = make([]response.ScaleTrendResponse, 0, len(result.Scales))
	for _, scale := range result.Scales {
		tests := make([]response.ScaleTestResponse, 0, len(scale.Tests))
		for _, test := range scale.Tests {
			factors := make([]response.ScaleFactorResponse, 0, len(test.Factors))
			for _, factor := range test.Factors {
				factors = append(factors, response.ScaleFactorResponse{
					FactorCode:     factor.FactorCode,
					FactorName:     factor.FactorName,
					RawScore:       factor.RawScore,
					RiskLevel:      factor.RiskLevel,
					RiskLevelLabel: response.LabelForRiskLevel(factor.RiskLevel),
				})
			}
			tests = append(tests, response.ScaleTestResponse{
				AssessmentID:   strconv.FormatUint(test.AssessmentID, 10),
				TestDate:       response.FormatDateTimeValue(test.TestDate),
				TotalScore:     test.TotalScore,
				RiskLevel:      test.RiskLevel,
				RiskLevelLabel: response.LabelForRiskLevel(test.RiskLevel),
				Result:         test.Result,
				Factors:        factors,
			})
		}
		resp.Scales = append(resp.Scales, response.ScaleTrendResponse{
			ScaleID:   scale.ScaleID,
			ScaleCode: scale.ScaleCode,
			ScaleName: scale.ScaleName,
			Tests:     tests,
		})
	}
	return resp
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

// The application access service verifies active Operator, company and the
// exact read permission before backend guardian enrichment is invoked.
func (h *TesteeHandler) validateTesteeReadScope(c *gin.Context, orgID, userID int64, testeeID uint64) error {
	access, ok := h.testeeAccessService.(interface {
		ValidateTesteeStoreAccess(context.Context, int64, int64, uint64, string, string) error
	})
	if !ok {
		return errors.WithCode(code.ErrPermissionDenied, "scoped testee access is unavailable")
	}
	return access.ValidateTesteeStoreAccess(c.Request.Context(), orgID, userID, testeeID, "qs:actor:collection:testees", "read")
}
