package handler

import (
	"encoding/json"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
)

type AIWorkflowSolutionHandler struct {
	*AIWorkflowPromptDraftHandler
	solutions *app.SolutionAdministration
}

func NewAIWorkflowSolutionHandler(s *app.SolutionAdministration) *AIWorkflowSolutionHandler {
	return &AIWorkflowSolutionHandler{NewAIWorkflowPromptDraftHandler(nil), s}
}
func (h *AIWorkflowSolutionHandler) Read(operation string) gin.HandlerFunc {
	return func(c *gin.Context) {
		scope, ok := h.scope(c)
		if !ok {
			return
		}
		id := c.Param("solution_id")
		if operation == "receipt" {
			id = c.Param("command_id")
		}
		value, err := h.solutions.Read(c.Request.Context(), scope, operation, id, c.Query("cursor"))
		if err != nil {
			h.failure(c, err)
			return
		}
		h.Success(c, value)
	}
}
func (h *AIWorkflowSolutionHandler) Write(operation string) gin.HandlerFunc {
	return func(c *gin.Context) {
		scope, ok := h.scope(c)
		if !ok {
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 256*1024)
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			h.failure(c, app.ErrInvalid)
			return
		}
		value, err := h.solutions.Write(c.Request.Context(), scope, operation, c.Param("solution_id"), json.RawMessage(body))
		if err != nil {
			h.failure(c, err)
			return
		}
		h.Success(c, value)
	}
}
