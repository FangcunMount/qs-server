package handler

import (
	"errors"

	evaluationapp "github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (h *EvaluationHandler) parseTesteeAndAssessmentID(c *gin.Context) (testeeID, assessmentID uint64, ok bool) {
	testeeID, ok = h.parseRequiredTesteeID(c)
	if !ok {
		return 0, 0, false
	}
	assessmentID, ok = h.parseRequiredAssessmentID(c)
	if !ok {
		return 0, 0, false
	}
	return testeeID, assessmentID, true
}

func (h *EvaluationHandler) bindAssessmentListQuery(c *gin.Context) (testeeID uint64, req evaluationapp.ListAssessmentsRequest, ok bool) {
	testeeID, ok = h.parseRequiredTesteeID(c)
	if !ok {
		return 0, req, false
	}
	if err := h.BindQuery(c, &req); err != nil {
		return 0, req, false
	}
	return testeeID, req, true
}

func respondAssessmentListError(h *EvaluationHandler, c *gin.Context, err error) {
	if errors.Is(err, evaluationapp.ErrInvalidAssessmentKind) {
		h.BadRequestResponse(c, err.Error(), err)
		return
	}
	h.InternalErrorResponse(c, "list assessments failed", err)
}

func respondOutcomeAssessmentReportError(h *EvaluationHandler, c *gin.Context, err error) {
	if status.Code(err) == codes.PermissionDenied {
		h.NotFoundResponse(c, "report not found", nil)
		return
	}
	h.InternalErrorResponse(c, "get report failed", err)
}
