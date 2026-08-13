package service

import (
	"context"
	"errors"
	"testing"

	pkgerrors "github.com/FangcunMount/component-base/pkg/errors"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	evaluationtestee "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/testee"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type testeeEvaluationServiceStub struct {
	getAssessmentErr error
	listResult       *evaluationtestee.AssessmentList
	listErr          error
	listActor        *evaluationtestee.Actor
	listQuery        *evaluationtestee.ListQuery
}

func (s testeeEvaluationServiceStub) AuthorizeAssessment(context.Context, evaluationtestee.Actor, uint64) error {
	return nil
}
func (s testeeEvaluationServiceStub) GetAssessment(context.Context, evaluationtestee.Actor, uint64) (*evaluationtestee.Assessment, error) {
	return nil, s.getAssessmentErr
}
func (s testeeEvaluationServiceStub) ListAssessments(_ context.Context, actor evaluationtestee.Actor, query evaluationtestee.ListQuery) (*evaluationtestee.AssessmentList, error) {
	if s.listActor != nil {
		*s.listActor = actor
	}
	if s.listQuery != nil {
		*s.listQuery = query
	}
	return s.listResult, s.listErr
}
func (s testeeEvaluationServiceStub) GetScore(context.Context, evaluationtestee.Actor, uint64) (*evaluationtestee.Score, error) {
	return nil, nil
}
func (s testeeEvaluationServiceStub) GetFactorTrend(context.Context, evaluationtestee.Actor, evaluationtestee.TrendQuery) (*evaluationtestee.FactorTrend, error) {
	return nil, nil
}
func (s testeeEvaluationServiceStub) GetHighRiskFactors(context.Context, evaluationtestee.Actor, uint64) (*evaluationtestee.HighRiskFactors, error) {
	return nil, nil
}

func TestTesteeEvaluationServiceScoreQueriesRequireTesteeSubject(t *testing.T) {
	t.Parallel()

	svc := &TesteeEvaluationService{}
	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "scores",
			call: func() error {
				_, err := svc.GetAssessmentScores(context.Background(), &pb.GetAssessmentScoresRequest{AssessmentId: 42})
				return err
			},
		},
		{
			name: "high risk factors",
			call: func() error {
				_, err := svc.GetHighRiskFactors(context.Background(), &pb.GetHighRiskFactorsRequest{AssessmentId: 42})
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := status.Code(tt.call()); got != codes.InvalidArgument {
				t.Fatalf("status = %s, want %s", got, codes.InvalidArgument)
			}
		})
	}
}

func TestToAssessmentQueryGRPCError(t *testing.T) {
	t.Run("maps assessment not found to grpc not found", func(t *testing.T) {
		err := pkgerrors.WithCode(errorCode.ErrAssessmentNotFound, "assessment not found")

		got := toAssessmentQueryGRPCError(err)
		if status.Code(got) != codes.NotFound {
			t.Fatalf("expected NotFound, got %s", status.Code(got))
		}
	})

	t.Run("maps wrapped assessment not found to grpc not found", func(t *testing.T) {
		err := pkgerrors.WrapC(errors.New("repo miss"), errorCode.ErrAssessmentNotFound, "assessment not found")

		got := toAssessmentQueryGRPCError(err)
		if status.Code(got) != codes.NotFound {
			t.Fatalf("expected NotFound, got %s", status.Code(got))
		}
	})

	t.Run("maps unknown error to grpc internal", func(t *testing.T) {
		got := toAssessmentQueryGRPCError(errors.New("boom"))
		if status.Code(got) != codes.Internal {
			t.Fatalf("expected Internal, got %s", status.Code(got))
		}
		if status.Convert(got).Message() != "internal error" {
			t.Fatalf("unknown internal error leaked: %q", status.Convert(got).Message())
		}
	})
}

func TestGetMyAssessmentUsesAssessmentQueryGRPCErrorContract(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code codes.Code
	}{
		{name: "not found", err: pkgerrors.WithCode(errorCode.ErrAssessmentNotFound, "missing detail"), code: codes.NotFound},
		{name: "permission denied", err: pkgerrors.WithCode(errorCode.ErrPermissionDenied, "foreign detail"), code: codes.PermissionDenied},
		{name: "dependency failure", err: errors.New("database endpoint"), code: codes.Internal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewTesteeEvaluationService(testeeEvaluationServiceStub{getAssessmentErr: tt.err})
			_, err := svc.GetMyAssessment(context.Background(), &pb.GetMyAssessmentRequest{TesteeId: 7, AssessmentId: 42})
			if status.Code(err) != tt.code {
				t.Fatalf("status = %s, want %s; err=%v", status.Code(err), tt.code, err)
			}
			if tt.code == codes.Internal && status.Convert(err).Message() != "internal error" {
				t.Fatalf("internal detail leaked: %q", status.Convert(err).Message())
			}
		})
	}
}

func TestListMyAssessmentsPreservesPaginationFiltersAndEmptyItems(t *testing.T) {
	t.Parallel()

	var actor evaluationtestee.Actor
	var query evaluationtestee.ListQuery
	svc := NewTesteeEvaluationService(testeeEvaluationServiceStub{
		listResult: &evaluationtestee.AssessmentList{Items: []*evaluationtestee.Assessment{}, Total: 0, Page: 1, PageSize: 10, TotalPages: 0},
		listActor:  &actor,
		listQuery:  &query,
	})
	result, err := svc.ListMyAssessments(context.Background(), &pb.ListMyAssessmentsRequest{
		TesteeId: 42, Status: "done", ScaleCode: "scale-a", RiskLevel: "high",
		ModelKinds: []string{"scale", "typology", "scale"}, ModelCode: "model-a",
		DateFrom: "2026-08-01", DateTo: "2026-08-02",
	})
	if err != nil {
		t.Fatalf("ListMyAssessments() error = %v", err)
	}
	if result == nil || result.Items == nil || len(result.Items) != 0 || result.Page != 1 || result.PageSize != 10 || result.Total != 0 || result.TotalPages != 0 {
		t.Fatalf("ListMyAssessments() result = %#v", result)
	}
	if actor.TesteeID != 42 || query.Page != 1 || query.PageSize != 10 || query.Status != "done" || query.ScaleCode != "scale-a" || query.RiskLevel != "high" ||
		len(query.ModelKinds) != 2 || query.ModelKinds[0] != "scale" || query.ModelKinds[1] != "typology" || query.ModelCode != "model-a" ||
		query.DateFrom != "2026-08-01" || query.DateTo != "2026-08-02" {
		t.Fatalf("ListMyAssessments() actor/query = %#v %#v", actor, query)
	}
}

func TestListMyAssessmentsPreservesErrorMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want codes.Code
	}{
		{name: "permission denied", err: pkgerrors.WithCode(errorCode.ErrPermissionDenied, "foreign list"), want: codes.PermissionDenied},
		{name: "dependency failure", err: errors.New("database endpoint"), want: codes.Internal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewTesteeEvaluationService(testeeEvaluationServiceStub{listErr: tt.err})
			_, err := svc.ListMyAssessments(context.Background(), &pb.ListMyAssessmentsRequest{TesteeId: 42})
			if status.Code(err) != tt.want {
				t.Fatalf("status = %s, want %s; err=%v", status.Code(err), tt.want, err)
			}
			if tt.want == codes.Internal && status.Convert(err).Message() != "internal error" {
				t.Fatalf("internal detail leaked: %q", status.Convert(err).Message())
			}
		})
	}
}

func TestNormalizeModelKinds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		modelKind  string
		modelKinds []string
		want       []string
		wantCode   codes.Code
	}{
		{name: "absent keeps legacy filter", modelKind: "typology"},
		{name: "deduplicates exact kinds", modelKinds: []string{"behavioral_rating", "cognitive", "behavioral_rating"}, want: []string{"behavioral_rating", "cognitive"}},
		{name: "rejects mixed filters", modelKind: "typology", modelKinds: []string{"cognitive"}, wantCode: codes.InvalidArgument},
		{name: "rejects empty kind", modelKinds: []string{"cognitive", ""}, wantCode: codes.InvalidArgument},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeModelKinds(tt.modelKind, tt.modelKinds)
			if tt.wantCode != codes.OK {
				if status.Code(err) != tt.wantCode {
					t.Fatalf("status = %s, want %s", status.Code(err), tt.wantCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeModelKinds() error = %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("model kinds = %#v, want %#v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("model kinds = %#v, want %#v", got, tt.want)
				}
			}
		})
	}
}
