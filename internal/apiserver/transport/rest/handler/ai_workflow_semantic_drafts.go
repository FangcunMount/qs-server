package handler

import (
	"encoding/json"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
)

type AIWorkflowSemanticDraftHandler struct {
	*AIWorkflowPromptDraftHandler
	drafts *app.SemanticDraftAdministration
}

func NewAIWorkflowSemanticDraftHandler(s *app.SemanticDraftAdministration) *AIWorkflowSemanticDraftHandler {
	return &AIWorkflowSemanticDraftHandler{NewAIWorkflowPromptDraftHandler(nil), s}
}
func (h *AIWorkflowSemanticDraftHandler) Read(operation string) gin.HandlerFunc {
	return func(c *gin.Context) {
		scope, ok := h.scope(c)
		if !ok {
			return
		}
		id := c.Param("draft_id")
		if operation == "receipt" {
			id = c.Param("command_id")
		}
		value, err := h.drafts.Read(c.Request.Context(), scope, operation, id, c.Query("revision"))
		if err != nil {
			h.failure(c, err)
			return
		}
		h.Success(c, value)
	}
}
func (h *AIWorkflowSemanticDraftHandler) Write(operation string) gin.HandlerFunc {
	return func(c *gin.Context) {
		scope, ok := h.scope(c)
		if !ok {
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 240*1024)
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			h.failure(c, app.ErrInvalid)
			return
		}
		value, err := h.drafts.Write(c.Request.Context(), scope, operation, c.Param("draft_id"), json.RawMessage(body))
		if err != nil {
			h.failure(c, err)
			return
		}
		h.Success(c, value)
	}
}
