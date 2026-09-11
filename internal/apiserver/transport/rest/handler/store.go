package handler

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	clinicianApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/clinician"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/store"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/store"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/actorstore"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/request"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/response"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/gin-gonic/gin"
	"strconv"
)

type StoreHandler struct {
	*BaseHandler
	service    *app.Service
	clinicians clinicianApp.ClinicianQueryService
}

func NewStoreHandler(s *app.Service, c clinicianApp.ClinicianQueryService) *StoreHandler {
	return &StoreHandler{NewBaseHandler(), s, c}
}
func (h *StoreHandler) actor(c *gin.Context) (app.Actor, bool) {
	org, user, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return app.Actor{}, false
	}
	return app.Actor{OrgID: org, UserID: user}, true
}
func (h *StoreHandler) id(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid ID"))
		return 0, false
	}
	return id, true
}
func storeResponse(s *domain.Store, n int64) *response.StoreResponse {
	v := s.State()
	return &response.StoreResponse{ID: v.ID, OrgID: v.OrgID, Code: v.Code, Name: v.Name, Address: v.Address, IsActive: v.IsActive, Version: v.Version, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt, CreatedBy: v.CreatedBy, UpdatedBy: v.UpdatedBy, ClinicianCount: n}
}
func historyResponse(h port.History) response.ClinicianStoreHistoryResponse {
	return response.ClinicianStoreHistoryResponse{ID: h.ID, OrgID: h.OrgID, ClinicianID: h.ClinicianID, FromStoreID: h.FromStoreID, ToStoreID: h.ToStoreID, Kind: h.Kind, ActorID: h.ActorID, CreatedAt: h.CreatedAt, Reason: h.Reason, RequestID: h.RequestID, InvalidatedCount: h.InvalidatedCount, Version: h.Version}
}

// List 查询公司门店。
// @Summary 查询公司门店
// @Tags Actor Store
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param page query int false "页码"
// @Param page_size query int false "每页数量，最多 100"
// @Param search query string false "名称或编号"
// @Param is_active query boolean false "启用状态"
// @Success 200 {object} core.Response{data=response.StoreListResponse}
// @Failure 403 {object} core.Response
// @Failure 409 {object} core.Response
// @Router /api/v1/stores [get]
func (h *StoreHandler) List(c *gin.Context) {
	a, ok := h.actor(c)
	if !ok {
		return
	}
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid page"))
		return
	}
	size, err := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if err != nil {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid page size"))
		return
	}
	f := port.Filter{Search: c.Query("search"), Page: page, PageSize: size}
	if raw, exists := c.GetQuery("is_active"); exists {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid status"))
			return
		}
		f.Active = &v
	}
	p, err := h.service.List(c.Request.Context(), a, f)
	if err != nil {
		h.Error(c, err)
		return
	}
	items := make([]*response.StoreResponse, 0, len(p.Items))
	for _, v := range p.Items {
		items = append(items, storeResponse(v.Store, v.ClinicianCount))
	}
	h.Success(c, response.StoreListResponse{Items: items, Total: p.Total})
}

// Get 查询门店详情。
// @Summary 查询门店详情
// @Tags Actor Store
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path string true "对象 ID"
// @Success 200 {object} core.Response{data=response.StoreResponse}
// @Failure 403 {object} core.Response
// @Failure 409 {object} core.Response
// @Router /api/v1/stores/{id} [get]
func (h *StoreHandler) Get(c *gin.Context) {
	a, ok := h.actor(c)
	if !ok {
		return
	}
	id, ok := h.id(c)
	if !ok {
		return
	}
	v, err := h.service.Get(c.Request.Context(), a, id)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, storeResponse(v.Store, v.ClinicianCount))
}

// Create 创建门店。
// @Summary 创建门店
// @Tags Actor Store
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param request body request.CreateStoreRequest true "请求"
// @Success 200 {object} core.Response{data=response.StoreResponse}
// @Failure 403 {object} core.Response
// @Failure 409 {object} core.Response
// @Router /api/v1/stores [post]
func (h *StoreHandler) Create(c *gin.Context) {
	a, ok := h.actor(c)
	if !ok {
		return
	}
	var req request.CreateStoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid store request"))
		return
	}
	v, err := h.service.Create(c.Request.Context(), a, req.Code, req.Name, req.Address)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, storeResponse(v, 0))
}

// Update 更新门店资料。
// @Summary 更新门店资料
// @Tags Actor Store
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path string true "对象 ID"
// @Param request body request.UpdateStoreRequest true "请求"
// @Success 200 {object} core.Response{data=response.StoreResponse}
// @Failure 403 {object} core.Response
// @Failure 409 {object} core.Response
// @Router /api/v1/stores/{id} [put]
func (h *StoreHandler) Update(c *gin.Context) {
	a, ok := h.actor(c)
	if !ok {
		return
	}
	id, ok := h.id(c)
	if !ok {
		return
	}
	var req request.UpdateStoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid store update"))
		return
	}
	v, err := h.service.Update(c.Request.Context(), a, id, req.ExpectedVersion, req.Name, req.Address, nil)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.updatedStoreResponse(c, a, v)
}

// Activate 启用门店。
// @Summary 启用门店
// @Tags Actor Store
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path string true "对象 ID"
// @Param request body request.StoreStatusRequest true "请求"
// @Success 200 {object} core.Response{data=response.StoreResponse}
// @Failure 403 {object} core.Response
// @Failure 409 {object} core.Response
// @Router /api/v1/stores/{id}/activate [post]
func (h *StoreHandler) Activate(c *gin.Context) { h.status(c, true) }

// Deactivate 停用门店。
// @Summary 停用门店
// @Tags Actor Store
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path string true "对象 ID"
// @Param request body request.StoreStatusRequest true "请求"
// @Success 200 {object} core.Response{data=response.StoreResponse}
// @Failure 403 {object} core.Response
// @Failure 409 {object} core.Response
// @Router /api/v1/stores/{id}/deactivate [post]
func (h *StoreHandler) Deactivate(c *gin.Context) { h.status(c, false) }
func (h *StoreHandler) status(c *gin.Context, active bool) {
	a, ok := h.actor(c)
	if !ok {
		return
	}
	id, ok := h.id(c)
	if !ok {
		return
	}
	var req request.StoreStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "expected_version required"))
		return
	}
	v, err := h.service.Update(c.Request.Context(), a, id, req.ExpectedVersion, "", "", &active)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.updatedStoreResponse(c, a, v)
}

// Assign 配置医生服务门店。
// @Summary 配置医生服务门店
// @Tags Actor Store
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path string true "对象 ID"
// @Param request body request.AssignClinicianStoreRequest true "请求"
// @Success 200 {object} core.Response{data=response.ClinicianStoreAssignmentResponse}
// @Failure 403 {object} core.Response
// @Failure 409 {object} core.Response
// @Router /api/v1/clinicians/{id}/store [put]
func (h *StoreHandler) Assign(c *gin.Context) {
	a, ok := h.actor(c)
	if !ok {
		return
	}
	id, ok := h.id(c)
	if !ok {
		return
	}
	var req request.AssignClinicianStoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid assignment request"))
		return
	}
	v, err := h.service.Assign(c.Request.Context(), a, id, app.Change{StoreID: req.StoreID, ExpectedVersion: req.ExpectedVersion, Reason: req.Reason, RequestID: req.RequestID})
	if err != nil {
		h.Error(c, err)
		return
	}
	cl, err := h.clinicians.GetByID(c.Request.Context(), id)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, response.ClinicianStoreAssignmentResponse{Clinician: toClinicianResponse(cl), Change: historyResponse(*v), InvalidatedCount: v.InvalidatedCount})
}

// History 查询医生门店配置历史。
// @Summary 查询医生门店配置历史
// @Tags Actor Store
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path string true "对象 ID"
// @Success 200 {object} core.Response{data=response.ClinicianStoreHistoryListResponse}
// @Failure 403 {object} core.Response
// @Failure 409 {object} core.Response
// @Router /api/v1/clinicians/{id}/store-history [get]
func (h *StoreHandler) History(c *gin.Context) {
	a, ok := h.actor(c)
	if !ok {
		return
	}
	id, ok := h.id(c)
	if !ok {
		return
	}
	rows, err := h.service.History(c.Request.Context(), a, id)
	if err != nil {
		h.Error(c, err)
		return
	}
	items := make([]response.ClinicianStoreHistoryResponse, 0, len(rows))
	for _, v := range rows {
		items = append(items, historyResponse(v))
	}
	h.Success(c, response.ClinicianStoreHistoryListResponse{Items: items})
}

// Progress 查询医生门店配置进度。
// @Summary 查询医生门店配置进度
// @Tags Actor Store
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Success 200 {object} core.Response{data=response.StoreProgressResponse}
// @Failure 403 {object} core.Response
// @Failure 409 {object} core.Response
// @Router /api/v1/stores/configuration-progress [get]
func (h *StoreHandler) Progress(c *gin.Context) {
	a, ok := h.actor(c)
	if !ok {
		return
	}
	p, err := h.service.Progress(c.Request.Context(), a)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, response.StoreProgressResponse{Total: p.Total, Configured: p.Configured, Unconfigured: p.Unconfigured, ActiveTotal: p.ActiveTotal, ActiveConfigured: p.ActiveConfigured, ActiveUnconfigured: p.ActiveUnconfigured})
}

func (h *StoreHandler) updatedStoreResponse(c *gin.Context, a app.Actor, v *domain.Store) {
	detail, err := h.service.Get(c.Request.Context(), a, v.ID())
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, storeResponse(detail.Store, detail.ClinicianCount))
}
