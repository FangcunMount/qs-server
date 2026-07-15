package behaviorassessment

import (
	"context"
	"errors"
	"testing"

	evaluationapp "github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
)

type fakeReader struct {
	listKinds []string
	list      *evaluationapp.ListAssessmentsResponse
	detail    *evaluationapp.AssessmentDetailResponse
	report    *evaluationapp.AssessmentReportResponse
}

func (f *fakeReader) ListMyAssessmentsByModelKinds(_ context.Context, _ uint64, _ string, kinds []string, _, _ int32) (*evaluationapp.ListAssessmentsResponse, error) {
	f.listKinds = append([]string(nil), kinds...)
	return f.list, nil
}

func (f *fakeReader) GetMyAssessment(context.Context, uint64, uint64) (*evaluationapp.AssessmentDetailResponse, error) {
	return f.detail, nil
}

func (f *fakeReader) GetAssessmentReport(context.Context, uint64, uint64) (*evaluationapp.AssessmentReportResponse, error) {
	return f.report, nil
}

func TestQueryServiceListUsesBothBehaviorModelKinds(t *testing.T) {
	t.Parallel()

	reader := &fakeReader{list: &evaluationapp.ListAssessmentsResponse{}}
	service := NewQueryService(reader, nil)
	if _, err := service.List(context.Background(), 1, &ListAssessmentsRequest{}); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(reader.listKinds) != 2 || reader.listKinds[0] != "behavioral_rating" || reader.listKinds[1] != "cognitive" {
		t.Fatalf("model kinds = %#v, want behavioral_rating/cognitive", reader.listKinds)
	}
}

func TestQueryServiceRejectsOtherAssessmentKinds(t *testing.T) {
	t.Parallel()

	for _, kind := range []string{"scale", "typology", "personality"} {
		kind := kind
		t.Run(kind, func(t *testing.T) {
			service := NewQueryService(&fakeReader{detail: &evaluationapp.AssessmentDetailResponse{Model: evaluationapp.ModelIdentityResponse{Kind: kind}}}, nil)
			_, err := service.Get(context.Background(), 1, 2)
			if !errors.Is(err, ErrNotBehaviorAssessment) {
				t.Fatalf("Get() error = %v, want ErrNotBehaviorAssessment", err)
			}
		})
	}
}

func TestQueryServiceAcceptsBothBehaviorAssessmentKinds(t *testing.T) {
	t.Parallel()

	for _, kind := range []string{"behavioral_rating", "cognitive"} {
		kind := kind
		t.Run(kind, func(t *testing.T) {
			service := NewQueryService(&fakeReader{report: &evaluationapp.AssessmentReportResponse{Model: evaluationapp.ModelIdentityResponse{Kind: kind}}}, nil)
			if _, err := service.GetReport(context.Background(), 1, 2); err != nil {
				t.Fatalf("GetReport() error = %v", err)
			}
		})
	}
}

func TestQueryServiceStatusRejectsOtherAssessmentBeforeWaiting(t *testing.T) {
	t.Parallel()

	service := NewQueryService(&fakeReader{detail: &evaluationapp.AssessmentDetailResponse{Model: evaluationapp.ModelIdentityResponse{Kind: "scale"}}}, nil)
	_, err := service.GetReportStatus(context.Background(), 1, 2)
	if !errors.Is(err, ErrNotBehaviorAssessment) {
		t.Fatalf("GetReportStatus() error = %v, want ErrNotBehaviorAssessment", err)
	}
}
