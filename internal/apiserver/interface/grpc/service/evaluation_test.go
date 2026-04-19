package service

import (
	"errors"
	"testing"

	pkgerrors "github.com/FangcunMount/component-base/pkg/errors"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestToAssessmentQueryGRPCError(t *testing.T) {
	t.Run("maps assessment not found to grpc not found", func(t *testing.T) {
		err := pkgerrors.WithCode(errorCode.ErrAssessmentNotFound, "assessment not found")

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
	})
}
