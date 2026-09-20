package handler

import (
	"encoding/json"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
)

type AIWorkflowQuotaHandler struct {
	*AIWorkflowPromptDraftHandler
	quotas *app.QuotaAdministration
}

func NewAIWorkflowQuotaHandler(s *app.QuotaAdministration) *AIWorkflowQuotaHandler {
	return &AIWorkflowQuotaHandler{NewAIWorkflowPromptDraftHandler(nil), s}
}
func (h *AIWorkflowQuotaHandler) Read(operation string) gin.HandlerFunc {
	return func(c *gin.Context) {
		scope, ok := h.scope(c)
		if !ok {
			return
		}
		id := c.Param("solution_id")
		if operation == "receipt" {
			id = c.Param("command_id")
		}
		value, err := h.quotas.Read(c.Request.Context(), scope, operation, id, c.Query("before_revision"))
		if err != nil {
			h.failure(c, err)
			return
		}
		h.Success(c, value)
	}
}
func (h *AIWorkflowQuotaHandler) Write(operation string) gin.HandlerFunc {
	return func(c *gin.Context) {
		scope, ok := h.scope(c)
		if !ok {
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 15*1024)
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			h.failure(c, app.ErrInvalid)
			return
		}
		value, err := h.quotas.Write(c.Request.Context(), scope, operation, json.RawMessage(body))
		if err != nil {
			h.failure(c, err)
			return
		}
		h.Success(c, value)
	}
}
