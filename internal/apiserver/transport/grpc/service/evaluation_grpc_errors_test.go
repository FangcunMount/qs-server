package service

import (
	"context"
	"errors"
	"testing"

	pkgerrors "github.com/FangcunMount/component-base/pkg/errors"
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestToEvaluationGRPCErrorClassification(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want codes.Code
	}{
		{"nil", nil, codes.OK},
		{"invalid argument", evalerrors.InvalidArgument("bad id"), codes.InvalidArgument},
		{"not found", evalerrors.AssessmentNotFound(errors.New("missing"), "missing"), codes.NotFound},
		{"invalid status", evalerrors.AssessmentInvalidStatus("not submitted"), codes.FailedPrecondition},
		{"module missing", evalerrors.ModuleNotConfigured("engine missing"), codes.FailedPrecondition},
		{"database", evalerrors.Database(errors.New("timeout"), "db"), codes.Unavailable},
		{"claim conflict", evalrun.ErrInvalidClaim, codes.Aborted},
		{"snapshot conflict", evalrun.ErrInputSnapshotConflict, codes.Aborted},
		{"canceled", context.Canceled, codes.Canceled},
		{"deadline", context.DeadlineExceeded, codes.DeadlineExceeded},
		{"unknown", errors.New("boom"), codes.Internal},
		{"unknown coded", pkgerrors.WithCode(errorCode.ErrAssessmentInterpretFailed, "scoring failed"), codes.Internal},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := toEvaluationGRPCError(tc.err)
			if tc.want == codes.OK {
				if got != nil {
					t.Fatalf("got %v, want nil", got)
				}
				return
			}
			if status.Code(got) != tc.want {
				t.Fatalf("code = %v, want %v (err=%v)", status.Code(got), tc.want, got)
			}
			if tc.want == codes.Internal || tc.want == codes.Unavailable {
				if status.Convert(got).Message() == "boom" || status.Convert(got).Message() == "timeout" {
					t.Fatalf("leaked internal detail: %q", status.Convert(got).Message())
				}
			}
		})
	}
}
