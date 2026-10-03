package handler

import (
	"net/http"
	"strings"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/FangcunMount/qs-server/pkg/core"
	"github.com/gin-gonic/gin"
)

// submittedCommand is a durable QS submission, not an AI admission receipt.
func submittedCommand(c *gin.Context, id string) {
	prefix := strings.SplitN(c.Request.URL.Path, "/interpretation/", 2)[0]
	c.JSON(http.StatusAccepted, core.Response{Code: 0, Message: "submitted", Data: struct {
		OperationID string `json:"operation_id"`
		CommandID   string `json:"command_id"`
		Status      string `json:"status"`
		StatusURL   string `json:"status_url"`
	}{id, id, "submitted", prefix + "/interpretation/ai-workflow/operations/" + id}})
}

type AIWorkflowOperationsHandler struct {
	*BaseHandler
	service *app.OperationAdministration
}

func NewAIWorkflowOperationsHandler(service *app.OperationAdministration) *AIWorkflowOperationsHandler {
	return &AIWorkflowOperationsHandler{NewBaseHandler(), service}
}
func (h *AIWorkflowOperationsHandler) Get(c *gin.Context) {
	org, user, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	value, err := h.service.Get(c.Request.Context(), app.DraftScope{OrganizationID: org, OperatorUserID: user}, c.Param("command_id"))
	if err != nil {
		NewAIWorkflowManagementHandler(nil).failure(c, err)
		return
	}
	h.Success(c, value)
}
