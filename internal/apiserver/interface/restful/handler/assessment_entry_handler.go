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
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/request"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/response"
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

func (h *AssessmentEntryHandler) CreateMyAssessmentEntry(c *gin.Context) {
	clinicianItem, err := h.currentClinician(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	var req request.CreateAssessmentEntryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Error(c, err)
		return
	}

	result, err := h.assessmentEntryService.Create(c.Request.Context(), assessmentEntryApp.CreateAssessmentEntryDTO{
		OrgID:         clinicianItem.OrgID,
		ClinicianID:   clinicianItem.ID,
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

func (h *AssessmentEntryHandler) ListMyAssessmentEntries(c *gin.Context) {
	clinicianItem, err := h.currentClinician(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	page, pageSize := paginationFromContext(c)
	result, err := h.assessmentEntryService.ListByClinician(c.Request.Context(), assessmentEntryApp.ListAssessmentEntryDTO{
		OrgID:       clinicianItem.OrgID,
		ClinicianID: clinicianItem.ID,
		Offset:      (page - 1) * pageSize,
		Limit:       pageSize,
	})
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, toAssessmentEntryListResponse(result, page, pageSize))
}

func (h *AssessmentEntryHandler) GetMyAssessmentEntry(c *gin.Context) {
	clinicianItem, err := h.currentClinician(c)
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
	if result.ClinicianID != clinicianItem.ID {
		h.Error(c, errors.WithCode(code.ErrPermissionDenied, "assessment entry does not belong to current clinician"))
		return
	}

	h.Success(c, toAssessmentEntryResponse(result, h.generateAssessmentEntryQRCodeURL(c.Request.Context(), result.Token)))
}

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

func (h *AssessmentEntryHandler) DeactivateAssessmentEntry(c *gin.Context) {
	h.setAssessmentEntryActive(c, false)
}

func (h *AssessmentEntryHandler) ReactivateAssessmentEntry(c *gin.Context) {
	h.setAssessmentEntryActive(c, true)
}

func (h *AssessmentEntryHandler) DeactivateMyAssessmentEntry(c *gin.Context) {
	h.setMyAssessmentEntryActive(c, false)
}

func (h *AssessmentEntryHandler) ReactivateMyAssessmentEntry(c *gin.Context) {
	h.setMyAssessmentEntryActive(c, true)
}

func (h *AssessmentEntryHandler) ResolveAssessmentEntry(c *gin.Context) {
	result, err := h.assessmentEntryService.Resolve(c.Request.Context(), c.Param("token"))
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, toAssessmentEntryResolvedResponse(result))
}

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

func (h *AssessmentEntryHandler) SetQRCodeService(qrCodeService qrcodeApp.QRCodeService) {
	h.qrCodeService = qrCodeService
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

func (h *AssessmentEntryHandler) currentClinician(c *gin.Context) (*clinicianApp.ClinicianResult, error) {
	orgID, userID, err := h.RequireProtectedScope(c)
	if err != nil {
		return nil, err
	}

	operatorItem, err := h.operatorQueryService.GetByUser(c.Request.Context(), orgID, userID)
	if err != nil {
		logger.L(c.Request.Context()).Errorw("Failed to get operator for clinician",
			"action", "current_clinician",
			"org_id", orgID,
			"user_id", userID,
			"error", err.Error(),
		)
		return nil, err
	}
	if !operatorItem.IsActive {
		return nil, errors.WithCode(code.ErrPermissionDenied, "operator is inactive")
	}

	clinicianItem, err := h.clinicianQueryService.GetByOperator(c.Request.Context(), orgID, operatorItem.ID)
	if err != nil {
		return nil, err
	}
	if !clinicianItem.IsActive {
		return nil, errors.WithCode(code.ErrPermissionDenied, "clinician is inactive")
	}
	return clinicianItem, nil
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

func (h *AssessmentEntryHandler) setMyAssessmentEntryActive(c *gin.Context, active bool) {
	clinicianItem, err := h.currentClinician(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	_, operatorUserID, err := h.RequireProtectedScope(c)
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
	if result.ClinicianID != clinicianItem.ID {
		h.Error(c, errors.WithCode(code.ErrPermissionDenied, "assessment entry does not belong to current clinician"))
		return
	}
	logger.L(c.Request.Context()).Infow("Assessment entry lifecycle changed",
		"action", map[bool]string{true: "reactivate_my_assessment_entry", false: "deactivate_my_assessment_entry"}[active],
		"org_id", clinicianItem.OrgID,
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
		ID:              strconv.FormatUint(item.ID, 10),
		OrgID:           strconv.FormatInt(item.OrgID, 10),
		ClinicianID:     strconv.FormatUint(item.ClinicianID, 10),
		Token:           item.Token,
		TargetType:      item.TargetType,
		TargetTypeLabel: response.LabelForTargetType(item.TargetType),
		TargetCode:      item.TargetCode,
		TargetVersion:   item.TargetVersion,
		IsActive:        item.IsActive,
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

	var operatorID *string
	if item.OperatorID != nil {
		value := strconv.FormatUint(*item.OperatorID, 10)
		operatorID = &value
	}

	return &response.ClinicianSummaryResponse{
		ID:                 strconv.FormatUint(item.ID, 10),
		OperatorID:         operatorID,
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
		item.Tags,
		item.Source,
		item.IsKeyFocus,
	)
}
