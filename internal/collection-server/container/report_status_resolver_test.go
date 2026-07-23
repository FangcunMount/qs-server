package container

import (
	"errors"
	"testing"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportstatus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestNormalizeAssessmentAccessError(t *testing.T) {
	for _, code := range []codes.Code{codes.NotFound, codes.PermissionDenied} {
		t.Run(code.String(), func(t *testing.T) {
			err := normalizeAssessmentAccessError(status.Error(code, "sensitive detail"))
			if !errors.Is(err, reportstatus.ErrAssessmentAccess) {
				t.Fatalf("error = %v, want assessment access", err)
			}
		})
	}

	dependencyErr := status.Error(codes.Unavailable, "database endpoint")
	if got := normalizeAssessmentAccessError(dependencyErr); status.Code(got) != codes.Unavailable {
		t.Fatalf("dependency error = %v, want unavailable", got)
	}
}
