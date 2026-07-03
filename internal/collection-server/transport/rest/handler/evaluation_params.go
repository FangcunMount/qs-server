package handler

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

func (h *EvaluationHandler) parseRequiredTesteeID(c *gin.Context) (uint64, bool) {
	return h.parseRequiredUint64Query(c, "testee_id", "testee_id is required", "invalid testee_id format")
}

func (h *EvaluationHandler) parseRequiredAssessmentID(c *gin.Context) (uint64, bool) {
	return h.parseRequiredUint64Path(c, "id", "invalid assessment id")
}

func (h *EvaluationHandler) parseRequiredAnswerSheetID(c *gin.Context) (uint64, bool) {
	return h.parseRequiredUint64Path(c, "id", "invalid answer sheet id")
}

func (h *EvaluationHandler) parseRequiredUint64Query(c *gin.Context, key, emptyMsg, invalidMsg string) (uint64, bool) {
	value := h.GetQueryParam(c, key)
	if value == "" {
		h.BadRequestResponse(c, emptyMsg, nil)
		return 0, false
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		h.BadRequestResponse(c, invalidMsg, err)
		return 0, false
	}
	return parsed, true
}

func (h *EvaluationHandler) parseRequiredUint64Path(c *gin.Context, key, invalidMsg string) (uint64, bool) {
	parsed, err := strconv.ParseUint(h.GetPathParam(c, key), 10, 64)
	if err != nil {
		h.BadRequestResponse(c, invalidMsg, err)
		return 0, false
	}
	return parsed, true
}

func (h *EvaluationHandler) parseReportStatusRequest(c *gin.Context) (uint64, uint64, bool) {
	testeeID, ok := h.parseRequiredTesteeID(c)
	if !ok {
		return 0, 0, false
	}
	assessmentID, ok := h.parseRequiredAssessmentID(c)
	if !ok {
		return 0, 0, false
	}
	return testeeID, assessmentID, true
}

func (h *EvaluationHandler) parseWaitReportRequest(c *gin.Context) (uint64, uint64, time.Duration, bool) {
	testeeID, ok := h.parseRequiredTesteeID(c)
	if !ok {
		return 0, 0, 0, false
	}
	assessmentID, ok := h.parseRequiredAssessmentID(c)
	if !ok {
		return 0, 0, 0, false
	}
	return testeeID, assessmentID, h.waitReportService.NormalizeTimeout(c.DefaultQuery("timeout", "20")), true
}
