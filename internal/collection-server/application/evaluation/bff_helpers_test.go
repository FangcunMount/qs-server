package evaluation

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/answersheet"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/scale"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestNormalizeAssessmentListRequestDefault(t *testing.T) {
	t.Parallel()

	req := &ListAssessmentsRequest{}
	NormalizeAssessmentListRequest(req, AssessmentListPageDefault)
	if req.Page != 1 || req.PageSize != 10 {
		t.Fatalf("page=(%d,%d), want (1,10)", req.Page, req.PageSize)
	}
}

func TestNormalizeAssessmentListRequestLegacy(t *testing.T) {
	t.Parallel()

	req := &ListAssessmentsRequest{}
	NormalizeAssessmentListRequest(req, AssessmentListPageLegacy)
	if req.Page != 1 || req.PageSize != 50 {
		t.Fatalf("page=(%d,%d), want (1,50)", req.Page, req.PageSize)
	}
}

func TestReportDimensionFilterKeepsVisibleFactors(t *testing.T) {
	t.Parallel()

	filter := NewReportDimensionFilter(stubScaleCatalog{factors: []string{"f1", "f2"}})
	report := &LegacyAssessmentReportResponse{
		ScaleCode: "scl-1",
		Dimensions: []DimensionInterpretResponse{
			{FactorCode: "f1"},
			{FactorCode: "hidden"},
			{FactorCode: "f2"},
		},
	}
	out, err := filter.Apply(context.Background(), report)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Dimensions) != 2 {
		t.Fatalf("dimensions = %d, want 2", len(out.Dimensions))
	}
}

func TestPendingAssessmentResolverAnswerSheetNotFound(t *testing.T) {
	t.Parallel()

	resolver := NewPendingAssessmentResolver(stubAnswerSheetLookup{
		err: status.Error(codes.NotFound, "missing"),
	})
	_, err := resolver.PendingStatus(context.Background(), 1)
	if err != ErrAnswerSheetNotFound {
		t.Fatalf("err = %v, want ErrAnswerSheetNotFound", err)
	}
}

type stubAnswerSheetLookup struct {
	resp *answersheet.AnswerSheetResponse
	err  error
}

func (s stubAnswerSheetLookup) Get(context.Context, uint64) (*answersheet.AnswerSheetResponse, error) {
	return s.resp, s.err
}

type stubScaleCatalog struct {
	factors []string
}

func (s stubScaleCatalog) GetScale(context.Context, string) (*scale.ScaleResponse, error) {
	factors := make([]scale.FactorResponse, len(s.factors))
	for i, code := range s.factors {
		factors[i] = scale.FactorResponse{Code: code}
	}
	return &scale.ScaleResponse{Factors: factors}, nil
}

func (s stubScaleCatalog) ListScales(context.Context, int32, int32, string, string, string, []string, []string, []string, []string) (*scale.ListScalesResponse, error) {
	return nil, nil
}

func (s stubScaleCatalog) ListHotScales(context.Context, int32, int32) (*scale.ListHotScalesResponse, error) {
	return nil, nil
}

func (s stubScaleCatalog) GetScaleCategories(context.Context) (*scale.ScaleCategoriesResponse, error) {
	return nil, nil
}
