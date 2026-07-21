package evaluation

import (
	"testing"
)

func TestNormalizeAssessmentListRequestDefault(t *testing.T) {
	t.Parallel()

	req := &ListAssessmentsRequest{}
	NormalizeAssessmentListRequest(req, AssessmentListPageDefault)
	if req.Page != 1 || req.PageSize != 10 {
		t.Fatalf("page=(%d,%d), want (1,10)", req.Page, req.PageSize)
	}
}
