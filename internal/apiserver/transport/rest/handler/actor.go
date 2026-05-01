package handler

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	actorAccessApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	clinicianApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/clinician"
	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
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
	scaleAnalysisQueryService    testeeApp.ScaleAnalysisQueryService
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
	scaleAnalysisQueryService testeeApp.ScaleAnalysisQueryService,
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

	orgID, _, err := h.validateProtectedTesteeAccess(c, id)
	if err != nil {
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
func (h *TesteeHandler) GetScaleAnalysis(c *gin.Context) {
	id, err := h.parseTesteeIDParam(c, "get_scale_analysis")
	if err != nil {
		h.Error(c, err)
		return
	}
	orgID, _, err := h.validateProtectedTesteeAccess(c, id)
	if err != nil {
		h.Error(c, err)
		return
	}
	if err := h.ensureTesteeExists(c, "get_scale_analysis", id); err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.scaleAnalysisQueryService.GetScaleAnalysis(c.Request.Context(), testeeApp.ScaleAnalysisQueryDTO{
		OrgID:    orgID,
		TesteeID: id,
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, toScaleAnalysisResponse(result))
}

// GetPeriodicStats 获取受试者周期统计。
func (h *TesteeHandler) GetPeriodicStats(c *gin.Context) {
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
	if _, err = h.testeeQueryService.GetByID(c.Request.Context(), id); err != nil {
		logger.L(c.Request.Context()).Errorw("Failed to get testee",
			"action", "get_periodic_stats",
			"testee_id", id,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	resp := &response.PeriodicStatsResponse{
		Projects:       []response.PeriodicProjectResponse{},
		TotalProjects:  0,
		ActiveProjects: 0,
	}
	h.Success(c, resp)
}

// UpdateTestee 更新受试者。
func (h *TesteeHandler) UpdateTestee(c *gin.Context) {
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

// ListTestees 查询受试者列表。
func (h *TesteeHandler) ListTestees(c *gin.Context) {
	_, operatorUserID, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	query, err := h.parseTesteeListQuery(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	if query.Request.ProfileID != "" {
		result, err := h.listTesteesByProfile(c, operatorUserID, query)
		if err != nil {
			h.Error(c, err)
			return
		}
		h.Success(c, result)
		return
	}

	dto, err := h.buildTesteeListDTO(c, operatorUserID, query)
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

func (h *TesteeHandler) ensureTesteeExists(c *gin.Context, action string, testeeID uint64) error {
	if _, err := h.testeeQueryService.GetByID(c.Request.Context(), testeeID); err != nil {
		logger.L(c.Request.Context()).Errorw("Failed to get testee",
			"action", action,
			"testee_id", testeeID,
			"error", err.Error(),
		)
		return err
	}
	return nil
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

func (h *TesteeHandler) listTesteesByProfile(c *gin.Context, operatorUserID int64, query *testeeListQuery) (*response.TesteeListResponse, error) {
	result, err := h.fetchTesteeByProfile(c, query.OrgID, query.Request.ProfileID)
	if err != nil {
		return nil, err
	}
	if err := h.testeeAccessService.ValidateTesteeAccess(c.Request.Context(), query.OrgID, operatorUserID, result.ID); err != nil {
		return nil, err
	}

	clinicianTesteeIDs, restrictToClinicianScope, err := h.resolveClinicianScopedTesteeIDs(c, query.OrgID, query.Request.ClinicianID)
	if err != nil {
		return nil, err
	}
	if restrictToClinicianScope && !containsUint64(clinicianTesteeIDs, result.ID) {
		return toTesteeListResponse([]*testeeApp.TesteeResult{}, 0, query.Page, query.PageSize), nil
	}
	if !testeeMatchesListFilter(result, query.Request, query.CreatedAtStart, query.CreatedAtEnd) {
		return toTesteeListResponse([]*testeeApp.TesteeResult{}, 0, query.Page, query.PageSize), nil
	}

	return toTesteeListResponse([]*testeeApp.TesteeResult{result}, 1, query.Page, query.PageSize), nil
}

func (h *TesteeHandler) buildTesteeListDTO(c *gin.Context, operatorUserID int64, query *testeeListQuery) (testeeApp.ListTesteeDTO, error) {
	dto := testeeApp.ListTesteeDTO{
		OrgID:          query.OrgID,
		Name:           query.Request.Name,
		Tags:           query.Request.Tags,
		KeyFocus:       query.Request.IsKeyFocus,
		CreatedAtStart: query.CreatedAtStart,
		CreatedAtEnd:   query.CreatedAtEnd,
		Offset:         (query.Page - 1) * query.PageSize,
		Limit:          query.PageSize,
	}

	clinicianTesteeIDs, restrictToClinicianScope, err := h.resolveClinicianScopedTesteeIDs(c, query.OrgID, query.Request.ClinicianID)
	if err != nil {
		return testeeApp.ListTesteeDTO{}, err
	}
	dto.AccessibleTesteeIDs = clinicianTesteeIDs
	dto.RestrictToAccessScope = restrictToClinicianScope

	scope, err := h.testeeAccessService.ResolveAccessScope(c.Request.Context(), query.OrgID, operatorUserID)
	if err != nil {
		return testeeApp.ListTesteeDTO{}, err
	}
	if scope.IsAdmin {
		return dto, nil
	}

	allowedTesteeIDs, err := h.testeeAccessService.ListAccessibleTesteeIDs(c.Request.Context(), query.OrgID, operatorUserID)
	if err != nil {
		return testeeApp.ListTesteeDTO{}, err
	}
	dto.AccessibleTesteeIDs, dto.RestrictToAccessScope = mergeAccessibleTesteeIDs(dto.AccessibleTesteeIDs, dto.RestrictToAccessScope, allowedTesteeIDs)
	return dto, nil
}

func (h *TesteeHandler) resolveClinicianScopedTesteeIDs(c *gin.Context, orgID int64, clinicianID *uint64) ([]uint64, bool, error) {
	if clinicianID == nil {
		return nil, false, nil
	}
	if _, err := requireClinicianInOrg(c.Request.Context(), h.clinicianQueryService, orgID, *clinicianID); err != nil {
		return nil, false, err
	}
	clinicianTesteeIDs, err := h.clinicianRelationshipService.ListAssignedTesteeIDs(c.Request.Context(), orgID, *clinicianID)
	if err != nil {
		return nil, false, err
	}
	return clinicianTesteeIDs, true, nil
}

func (h *TesteeHandler) validateProtectedTesteeAccess(c *gin.Context, testeeID uint64) (int64, int64, error) {
	orgID, operatorUserID, err := h.RequireProtectedScope(c)
	if err != nil {
		return 0, 0, err
	}
	if err := h.testeeAccessService.ValidateTesteeAccess(c.Request.Context(), orgID, operatorUserID, testeeID); err != nil {
		return 0, 0, err
	}
	return orgID, operatorUserID, nil
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

func toScaleAnalysisResponse(result *testeeApp.ScaleAnalysisQueryResult) *response.ScaleAnalysisResponse {
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
