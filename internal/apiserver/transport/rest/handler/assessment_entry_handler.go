package handler

import (
	"context"
	"strconv"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	assessmentEntryApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/assessmententry"
	clinicianApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/clinician"
	operatorApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operator"
	qrcodeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/request"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/response"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/gin-gonic/gin"
)

// AssessmentEntryHandler 负责 assessment-entry 相关 HTTP 入口。
type AssessmentEntryHandler struct {
	*BaseHandler
	operatorQueryService   operatorApp.OperatorQueryService
	clinicianQueryService  clinicianApp.ClinicianQueryService
	assessmentEntryService assessmentEntryApp.AssessmentEntryService
	qrCodeService          qrcodeApp.QRCodeService
}

func NewAssessmentEntryHandler(
	operatorQueryService operatorApp.OperatorQueryService,
	clinicianQueryService clinicianApp.ClinicianQueryService,
	assessmentEntryService assessmentEntryApp.AssessmentEntryService,
	qrCodeService qrcodeApp.QRCodeService,
) *AssessmentEntryHandler {
	return &AssessmentEntryHandler{
		BaseHandler:            NewBaseHandler(),
		operatorQueryService:   operatorQueryService,
		clinicianQueryService:  clinicianQueryService,
		assessmentEntryService: assessmentEntryService,
		qrCodeService:          qrCodeService,
	}
}

// CreateClinicianAssessmentEntry 为从业者创建测评入口。
// @Summary 为从业者创建测评入口
// @Tags AssessmentEntry
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path int true "从业者ID"
// @Param request body request.CreateAssessmentEntryRequest true "创建测评入口请求"
// @Success 200 {object} core.Response
// @Router /api/v1/clinicians/{id}/assessment-entries [post]
func (h *AssessmentEntryHandler) CreateClinicianAssessmentEntry(c *gin.Context) {
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	clinicianID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.Error(c, err)
		return
	}
	if _, err := requireClinicianInOrg(c.Request.Context(), h.clinicianQueryService, orgID, clinicianID); err != nil {
		h.Error(c, err)
		return
	}

	var req request.CreateAssessmentEntryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Error(c, err)
		return
	}

	result, err := h.assessmentEntryService.Create(c.Request.Context(), assessmentEntryApp.CreateAssessmentEntryDTO{
		OrgID:         orgID,
		ClinicianID:   clinicianID,
		TargetType:    req.TargetType,
		TargetCode:    req.TargetCode,
		TargetVersion: req.TargetVersion,
		ExpiresAt:     flexibleTimePtrToTimePtr(req.ExpiresAt),
	})
	if err != nil {
		h.Error(c, err)
		return
	}

	qrCodeURL := h.generateAssessmentEntryQRCodeURL(c.Request.Context(), result.Token)
	h.SuccessResponseWithMessage(c, "测评入口创建成功", toAssessmentEntryResponse(result, qrCodeURL))
}

// ListClinicianAssessmentEntries 查询从业者测评入口列表。
// @Summary 查询从业者测评入口列表
// @Tags AssessmentEntry
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path int true "从业者ID"
// @Success 200 {object} core.Response
// @Router /api/v1/clinicians/{id}/assessment-entries [get]
func (h *AssessmentEntryHandler) ListClinicianAssessmentEntries(c *gin.Context) {
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	clinicianID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.Error(c, err)
		return
	}
	if _, err := requireClinicianInOrg(c.Request.Context(), h.clinicianQueryService, orgID, clinicianID); err != nil {
		h.Error(c, err)
		return
	}

	page, pageSize := paginationFromContext(c)
	result, err := h.assessmentEntryService.ListByClinician(c.Request.Context(), assessmentEntryApp.ListAssessmentEntryDTO{
		OrgID:       orgID,
		ClinicianID: clinicianID,
		Offset:      (page - 1) * pageSize,
		Limit:       pageSize,
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, toAssessmentEntryListResponse(result, page, pageSize))
}

// GetAssessmentEntry 获取测评入口详情。
// @Summary 获取测评入口详情
// @Tags AssessmentEntry
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path int true "测评入口ID"
// @Success 200 {object} core.Response
// @Router /api/v1/assessment-entries/{id} [get]
func (h *AssessmentEntryHandler) GetAssessmentEntry(c *gin.Context) {
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	entryID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.assessmentEntryService.GetByID(c.Request.Context(), entryID)
	if err != nil {
		h.Error(c, err)
		return
	}
	if result.OrgID != orgID {
		h.Error(c, errors.WithCode(code.ErrPermissionDenied, "assessment entry does not belong to current organization"))
		return
	}
	h.Success(c, toAssessmentEntryResponse(result, h.generateAssessmentEntryQRCodeURL(c.Request.Context(), result.Token)))
}

// DeactivateAssessmentEntry 停用测评入口。
// @Summary 停用测评入口
// @Tags AssessmentEntry
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path int true "测评入口ID"
// @Success 200 {object} core.Response
// @Router /api/v1/assessment-entries/{id}/deactivate [post]
func (h *AssessmentEntryHandler) DeactivateAssessmentEntry(c *gin.Context) {
	h.setAssessmentEntryActive(c, false)
}

// ReactivateAssessmentEntry 重新启用测评入口。
// @Summary 重新启用测评入口
// @Tags AssessmentEntry
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path int true "测评入口ID"
// @Success 200 {object} core.Response
// @Router /api/v1/assessment-entries/{id}/reactivate [post]
func (h *AssessmentEntryHandler) ReactivateAssessmentEntry(c *gin.Context) {
	h.setAssessmentEntryActive(c, true)
}

// ResolveAssessmentEntry 解析公开测评入口。
// @Summary 解析公开测评入口
// @Tags AssessmentEntry
// @Produce json
// @Param token path string true "入口令牌"
// @Success 200 {object} core.Response
// @Router /api/v1/public/assessment-entries/{token} [get]
func (h *AssessmentEntryHandler) ResolveAssessmentEntry(c *gin.Context) {
	result, err := h.assessmentEntryService.Resolve(c.Request.Context(), c.Param("token"))
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, toAssessmentEntryResolvedResponse(result))
}

// IntakeAssessmentEntry 公开测评入口 intake。
// @Summary 公开测评入口 intake
// @Tags AssessmentEntry
// @Accept json
// @Produce json
// @Param token path string true "入口令牌"
// @Param request body request.IntakeByAssessmentEntryRequest true "intake 请求"
// @Success 200 {object} core.Response
// @Router /api/v1/public/assessment-entries/{token}/intake [post]
func (h *AssessmentEntryHandler) IntakeAssessmentEntry(c *gin.Context) {
	var req request.IntakeByAssessmentEntryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.BadRequestResponse(c, "invalid request body", nil)
		return
	}

	result, err := h.assessmentEntryService.Intake(c.Request.Context(), c.Param("token"), assessmentEntryApp.IntakeByAssessmentEntryDTO{
		ProfileID: req.ProfileID,
		Name:      req.Name,
		Gender:    parseGender(req.Gender),
		Birthday:  req.Birthday,
	})
	if err != nil {
		h.Error(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "扫码建档成功", toAssessmentEntryIntakeResponse(result))
}

func (h *AssessmentEntryHandler) generateAssessmentEntryQRCodeURL(ctx context.Context, token string) string {
	if h.qrCodeService == nil {
		return ""
	}

	generated, err := h.qrCodeService.GenerateAssessmentEntryQRCode(ctx, token)
	if err != nil {
		return ""
	}

	return generated
}

func (h *AssessmentEntryHandler) setAssessmentEntryActive(c *gin.Context, active bool) {
	orgID, operatorUserID, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	entryID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.Error(c, err)
		return
	}

	var result *assessmentEntryApp.AssessmentEntryResult
	if active {
		result, err = h.assessmentEntryService.Reactivate(c.Request.Context(), entryID)
	} else {
		result, err = h.assessmentEntryService.Deactivate(c.Request.Context(), entryID)
	}
	if err != nil {
		h.Error(c, err)
		return
	}
	if result.OrgID != orgID {
		h.Error(c, errors.WithCode(code.ErrPermissionDenied, "assessment entry does not belong to current organization"))
		return
	}
	logger.L(c.Request.Context()).Infow("Assessment entry lifecycle changed",
		"action", map[bool]string{true: "reactivate_assessment_entry", false: "deactivate_assessment_entry"}[active],
		"org_id", orgID,
		"assessment_entry_id", entryID,
		"clinician_id", result.ClinicianID,
		"operator_user_id", operatorUserID,
		"is_active", result.IsActive,
	)
	if active {
		h.SuccessResponseWithMessage(c, "测评入口已启用", toAssessmentEntryResponse(result, ""))
		return
	}
	h.SuccessResponseWithMessage(c, "测评入口已停用", toAssessmentEntryResponse(result, ""))
}

func toAssessmentEntryResponse(item *assessmentEntryApp.AssessmentEntryResult, qrCodeURL string) *response.AssessmentEntryResponse {
	if item == nil {
		return nil
	}

	return &response.AssessmentEntryResponse{
		InvalidatedAt: response.FormatDateTimePtr(item.InvalidatedAt), InvalidationReason: item.InvalidationReason, PermanentlyInvalidated: item.InvalidatedAt != nil,
		ID:              strconv.FormatUint(item.ID, 10),
		OrgID:           strconv.FormatInt(item.OrgID, 10),
		ClinicianID:     strconv.FormatUint(item.ClinicianID, 10),
		Token:           item.Token,
		TargetType:      item.TargetType,
		TargetTypeLabel: response.LabelForTargetType(item.TargetType),
		TargetCode:      item.TargetCode,
		TargetVersion:   item.TargetVersion,
		IsActive:        item.IsActive && item.InvalidatedAt == nil,
		IsActiveLabel:   map[bool]string{true: "启用", false: "停用"}[item.IsActive],
		ExpiresAt:       response.FormatDateTimePtr(item.ExpiresAt),
		QRCodeURL:       qrCodeURL,
	}
}

func toAssessmentEntryListResponse(result *assessmentEntryApp.AssessmentEntryListResult, page, pageSize int) *response.AssessmentEntryListResponse {
	items := make([]*response.AssessmentEntryResponse, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, toAssessmentEntryResponse(item, ""))
	}

	totalPages := 0
	if pageSize > 0 {
		totalPages = int((result.TotalCount + int64(pageSize) - 1) / int64(pageSize))
	}

	return &response.AssessmentEntryListResponse{
		Items:      items,
		Total:      result.TotalCount,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}
}

func toClinicianSummaryResponse(item *assessmentEntryApp.ClinicianSummaryResult) *response.ClinicianSummaryResponse {
	if item == nil {
		return nil
	}

	return &response.ClinicianSummaryResponse{
		ID:                 strconv.FormatUint(item.ID, 10),
		Name:               item.Name,
		Department:         item.Department,
		Title:              item.Title,
		ClinicianType:      item.ClinicianType,
		ClinicianTypeLabel: response.LabelForClinicianType(item.ClinicianType),
	}
}

func toAssessmentEntryResolvedResponse(item *assessmentEntryApp.ResolvedAssessmentEntryResult) *response.AssessmentEntryResolvedResponse {
	if item == nil {
		return nil
	}

	return &response.AssessmentEntryResolvedResponse{
		Entry:     toAssessmentEntryResponse(item.Entry, ""),
		Clinician: toClinicianSummaryResponse(item.Clinician),
	}
}

func toRelationResponse(item *assessmentEntryApp.RelationSummaryResult) *response.RelationResponse {
	if item == nil {
		return nil
	}
	return buildRelationResponse(
		item.ID,
		item.OrgID,
		item.ClinicianID,
		item.TesteeID,
		item.RelationType,
		item.SourceType,
		item.SourceID,
		item.IsActive,
		item.BoundAt,
		item.UnboundAt,
	)
}

func toAssessmentEntryIntakeResponse(item *assessmentEntryApp.AssessmentEntryIntakeResult) *response.AssessmentEntryIntakeResponse {
	if item == nil {
		return nil
	}

	return &response.AssessmentEntryIntakeResponse{
		Entry:      toAssessmentEntryResponse(item.Entry, ""),
		Clinician:  toClinicianSummaryResponse(item.Clinician),
		Testee:     toTesteeSummaryResponse(item.Testee),
		Relation:   toRelationResponse(item.Relation),
		Assignment: toRelationResponse(item.Assignment),
	}
}

func toTesteeSummaryResponse(item *assessmentEntryApp.TesteeSummaryResult) *response.TesteeResponse {
	if item == nil {
		return nil
	}
	return buildTesteeSummaryResponse(
		item.ID,
		item.OrgID,
		item.ProfileID,
		item.Name,
		item.Gender,
		item.Birthday,
		item.Source,
		item.IsKeyFocus,
	)
}
