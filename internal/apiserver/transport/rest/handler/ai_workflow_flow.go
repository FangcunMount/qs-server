package handler

import (
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
)

type AIWorkflowFlowHandler struct {
	*AIWorkflowPromptDraftHandler
	flows *app.FlowAdministration
}

func NewAIWorkflowFlowHandler(service *app.FlowAdministration) *AIWorkflowFlowHandler {
	return &AIWorkflowFlowHandler{NewAIWorkflowPromptDraftHandler(nil), service}
}
func (h *AIWorkflowFlowHandler) Read(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		scope, ok := h.scope(c)
		if !ok {
			return
		}
		value, err := h.flows.Read(c.Request.Context(), scope, kind, c.Param(kind+"_id"))
		if err != nil {
			h.failure(c, err)
			return
		}
		h.Success(c, value)
	}
}
