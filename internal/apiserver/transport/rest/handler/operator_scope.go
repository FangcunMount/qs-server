package handler

import (
	"encoding/json"
	"github.com/FangcunMount/component-base/pkg/errors"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operator"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/request"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/response"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/gin-gonic/gin"
	"io"
	"strconv"
)

type OperatorScopeHandler struct {
	*BaseHandler
	service *app.ScopeService
}

func NewOperatorScopeHandler(s *app.ScopeService) *OperatorScopeHandler {
	return &OperatorScopeHandler{NewBaseHandler(), s}
}

func scopeFactResponse(f iambridge.OperatorAssignmentFact) response.OperatorAssignmentScopeResponse {
	result := response.OperatorAssignmentScopeResponse{ID: f.AssignmentID, RoleID: f.RoleID, RoleName: f.RoleName, Protection: f.ManagementProtection}
	if f.Scope != nil {
		result.Scope = &response.OperatorDataScopeResponse{OrgID: strconv.FormatInt(f.Scope.OrgID, 10), Kind: f.Scope.Kind, StoreIDs: []string{}}
		for _, id := range f.Scope.StoreIDs {
			result.Scope.StoreIDs = append(result.Scope.StoreIDs, strconv.FormatUint(id, 10))
		}
	}
	return result
}

// Get reads authoritative role assignment ranges for an operator.
// @Summary 查询运营人员角色数据范围
// @Tags Operator
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path string true "Operator ID"
// @Success 200 {object} core.Response{data=response.OperatorScopeConfigurationResponse}
// @Failure 403 {object} core.Response
// @Failure 404 {object} core.Response
// @Router /api/v1/operators/{id}/authorization-scope [get]
func (h *OperatorScopeHandler) Get(c *gin.Context) {
	if _, _, err := h.RequireProtectedScope(c); err != nil {
		h.Error(c, err)
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid operator ID"))
		return
	}
	result, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		h.Error(c, err)
		return
	}
	assignments, legacy := []response.OperatorAssignmentScopeResponse{}, []response.OperatorAssignmentScopeResponse{}
	for _, fact := range result.Assignments {
		assignments = append(assignments, scopeFactResponse(fact))
	}
	for _, fact := range result.Unconfigured {
		legacy = append(legacy, scopeFactResponse(fact))
	}
	h.Success(c, response.OperatorScopeConfigurationResponse{OperatorID: strconv.FormatUint(result.OperatorID, 10), PolicyVersion: strconv.FormatInt(result.PolicyVersion, 10), Assignments: assignments, UnconfiguredAssignments: legacy, ProtectedAccess: result.ProtectedAccess})
}

func scopeUpdateInput(id uint64, org int64, req request.ReplaceOperatorScopeRequest) (app.UpdateScopeConfiguration, error) {
	invalid := func() (app.UpdateScopeConfiguration, error) {
		return app.UpdateScopeConfiguration{}, errors.WithCode(code.ErrInvalidArgument, "valid policy version, explicit roles and company required")
	}
	version, err := strconv.ParseInt(req.ExpectedPolicyVersion, 10, 64)
	if err != nil || version <= 0 || strconv.FormatInt(version, 10) != req.ExpectedPolicyVersion || req.Roles == nil || org <= 0 {
		return invalid()
	}
	result := app.UpdateScopeConfiguration{OperatorID: id, ExpectedPolicyVersion: version, Reason: req.Reason, Roles: []iambridge.OperatorScopedRole{}}
	for _, role := range *req.Roles {
		item := iambridge.OperatorScopedRole{RoleName: role.RoleName, Scope: iambridge.OperatorAssignmentScope{OrgID: org, Kind: role.Kind, StoreIDs: []uint64{}}}
		for _, raw := range role.StoreIDs {
			value, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || value <= 0 || strconv.FormatInt(value, 10) != raw {
				return invalid()
			}
			item.Scope.StoreIDs = append(item.Scope.StoreIDs, uint64(value))
		}
		result.Roles = append(result.Roles, item)
	}
	return result, nil
}

// Replace updates independent business role ranges with optimistic concurrency.
// @Summary 更新运营人员角色数据范围
// @Tags Operator
// @Accept json
// @Produce json
// @Param id path string true "Operator ID"
// @Param request body request.ReplaceOperatorScopeRequest true "数据范围配置"
// @Param Authorization header string true "Bearer 用户令牌"
// @Success 200 {object} core.Response{data=response.OperatorScopeUpdateResponse}
// @Failure 400 {object} core.Response
// @Failure 403 {object} core.Response
// @Failure 409 {object} core.Response
// @Router /api/v1/operators/{id}/authorization-scope [put]
func (h *OperatorScopeHandler) Replace(c *gin.Context) {
	org, _, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid operator ID"))
		return
	}
	var req request.ReplaceOperatorScopeRequest
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid scope configuration"))
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "single scope configuration required"))
		return
	}
	input, err := scopeUpdateInput(id, org, req)
	if err != nil {
		h.Error(c, err)
		return
	}
	version, err := h.service.Replace(c.Request.Context(), input)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, response.OperatorScopeUpdateResponse{PolicyVersion: strconv.FormatInt(version, 10), ProjectionPending: true})
}
