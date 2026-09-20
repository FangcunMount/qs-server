package handler

import (
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
	"strconv"
	"time"
)

type AIWorkflowRuntimeHandler struct {
	*AIWorkflowPromptDraftHandler
	service *app.RuntimeAdministration
}

func NewAIWorkflowRuntimeHandler(s *app.RuntimeAdministration) *AIWorkflowRuntimeHandler {
	return &AIWorkflowRuntimeHandler{NewAIWorkflowPromptDraftHandler(nil), s}
}
func (h *AIWorkflowRuntimeHandler) List(c *gin.Context) {
	for _, values := range c.Request.URL.Query() {
		if len(values) != 1 {
			h.failure(c, app.ErrInvalid)
			return
		}
	}
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	q := app.RuntimeQuery{RequestID: c.Query("request_id"), SessionID: c.Query("session_id"), AssessmentID: c.Query("assessment_id"), TesteeID: c.Query("testee_id"), Status: c.Query("status"), Cursor: c.Query("cursor")}
	if v := c.Query("history"); v != "" {
		if v != "true" && v != "false" {
			h.failure(c, app.ErrInvalid)
			return
		}
		q.History = v == "true"
	}
	if v := c.Query("limit"); v != "" {
		n, e := strconv.Atoi(v)
		if e != nil {
			h.failure(c, app.ErrInvalid)
			return
		}
		q.Limit = n
	}
	for key, dst := range map[string]*time.Time{"since": &q.Since, "until": &q.Until} {
		if v := c.Query(key); v != "" {
			t, e := time.Parse(time.RFC3339, v)
			if e != nil {
				h.failure(c, app.ErrInvalid)
				return
			}
			*dst = t
		}
	}
	result, err := h.service.List(c.Request.Context(), scope, q)
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, result)
}
func (h *AIWorkflowRuntimeHandler) Get(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	result, err := h.service.Get(c.Request.Context(), scope, c.Param("request_id"))
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, result)
}

func (h *AIWorkflowRuntimeHandler) Health(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	result, err := h.service.Health(c.Request.Context(), scope)
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, result)
}
func (h *AIWorkflowRuntimeHandler) Timeline(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	result, err := h.service.Timeline(c.Request.Context(), scope, c.Param("request_id"))
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, result)
}
