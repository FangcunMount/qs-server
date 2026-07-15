package service

import (
	"context"
	"errors"
	"testing"

	pkgerrors "github.com/FangcunMount/component-base/pkg/errors"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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
