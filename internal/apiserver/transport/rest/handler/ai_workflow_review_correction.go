package handler

import (
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
)

// CorrectReview preserves the original signature and appends a reviewer-owned correction.
// It never runs models, finalizes a Run or publishes a configuration.
func (h *AIWorkflowManagementHandler) CorrectReview(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	var command app.EvaluationReviewCorrection
	if err := h.BindJSON(c, &command); err != nil {
		return
	}
	value, err := h.service.CorrectReview(c.Request.Context(), scope, command)
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, value)
}
