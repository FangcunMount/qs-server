package handler

import (
	"strconv"

	"github.com/FangcunMount/component-base/pkg/errors"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testeestore"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/testeestore"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/request"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/response"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/gin-gonic/gin"
)

type TesteeStoreHandler struct {
	*BaseHandler
	service *app.Service
}

func NewTesteeStoreHandler(s *app.Service) *TesteeStoreHandler {
	return &TesteeStoreHandler{NewBaseHandler(), s}
}
func (h *TesteeStoreHandler) scope(c *gin.Context) (app.Actor, uint64, bool) {
	org, user, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return app.Actor{}, 0, false
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid testee ID"))
		return app.Actor{}, 0, false
	}
	return app.Actor{OrgID: org, UserID: user}, id, true
}
func ownershipResponse(v *port.History) response.TesteeStoreHistoryResponse {
	return response.TesteeStoreHistoryResponse{ID: v.ID, OrgID: v.OrgID, TesteeID: v.TesteeID, FromStoreID: v.FromStoreID, ToStoreID: v.ToStoreID, Kind: v.Kind, ActorID: v.ActorID, CreatedAt: v.CreatedAt, Reason: v.Reason, RequestID: v.RequestID, Version: v.Version}
}

// AssignInitial 首次配置受试者服务门店。
// @Summary 首次配置受试者服务门店
// @Tags Actor Testee
// @Accept json
// @Produce json
// @Param id path string true "受试者 ID"
// @Param body body request.AssignTesteeStoreRequest true "归属配置"
// @Success 200 {object} core.Response{data=response.TesteeStoreHistoryResponse}
// @Failure 403 {object} core.Response
// @Failure 409 {object} core.Response
// @Router /api/v1/testees/{id}/store [put]
func (h *TesteeStoreHandler) AssignInitial(c *gin.Context) { h.change(c, false) }

// Transfer 总部显式转店。
// @Summary 总部调整受试者服务门店
// @Tags Actor Testee
// @Accept json
// @Produce json
// @Param id path string true "受试者 ID"
// @Param body body request.AssignTesteeStoreRequest true "转店配置"
// @Success 200 {object} core.Response{data=response.TesteeStoreHistoryResponse}
// @Failure 403 {object} core.Response
// @Failure 409 {object} core.Response
// @Router /api/v1/testees/{id}/store-transfers [post]
func (h *TesteeStoreHandler) Transfer(c *gin.Context) { h.change(c, true) }
func (h *TesteeStoreHandler) change(c *gin.Context, transfer bool) {
	actor, id, ok := h.scope(c)
	if !ok {
		return
	}
	var req request.AssignTesteeStoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid ownership change"))
		return
	}
	change := app.Change{StoreID: req.StoreID, ExpectedVersion: req.ExpectedVersion, Reason: req.Reason, RequestID: req.RequestID}
	var result *port.History
	var err error
	if transfer {
		result, err = h.service.Transfer(c.Request.Context(), actor, id, change)
	} else {
		result, err = h.service.AssignInitial(c.Request.Context(), actor, id, change)
	}
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, ownershipResponse(result))
}

// History 查询归属历史。
// @Summary 查询受试者服务门店变更历史
// @Tags Actor Testee
// @Produce json
// @Param id path string true "受试者 ID"
// @Param before query string false "上一页末尾历史 ID"
// @Param limit query int false "每页数量，最多 100"
// @Success 200 {object} core.Response{data=[]response.TesteeStoreHistoryResponse}
// @Failure 403 {object} core.Response
// @Router /api/v1/testees/{id}/store-history [get]
func (h *TesteeStoreHandler) History(c *gin.Context) {
	actor, id, ok := h.scope(c)
	if !ok {
		return
	}
	before, err := strconv.ParseUint(c.DefaultQuery("before", "0"), 10, 64)
	if err != nil {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid history cursor"))
		return
	}
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if err != nil || limit < 1 {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid history limit"))
		return
	}
	rows, err := h.service.History(c.Request.Context(), actor, id, before, limit)
	if err != nil {
		h.Error(c, err)
		return
	}
	result := make([]response.TesteeStoreHistoryResponse, 0, len(rows))
	for _, v := range rows {
		result = append(result, ownershipResponse(&v))
	}
	h.Success(c, result)
}
