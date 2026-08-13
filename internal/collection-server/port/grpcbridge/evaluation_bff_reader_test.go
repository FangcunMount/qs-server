package grpcbridge

import (
	"context"
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestAuthorizeAssessmentFallsBackOnlyForUnimplemented(t *testing.T) {
	tests := []struct {
		name              string
		authorizeErr      error
		legacyErr         error
		wantErr           error
		wantLegacyCalls   int
		wantFallbackDelta float64
	}{
		{
			name:              "new rpc succeeds",
			wantFallbackDelta: 0,
		},
		{
			name:              "old apiserver uses legacy detail rpc",
			authorizeErr:      status.Error(codes.Unimplemented, "method is not implemented"),
			wantLegacyCalls:   1,
			wantFallbackDelta: 1,
		},
		{
			name:              "legacy detail error is returned",
			authorizeErr:      status.Error(codes.Unimplemented, "method is not implemented"),
			legacyErr:         errors.New("legacy authorization failed"),
			wantErr:           errors.New("legacy authorization failed"),
			wantLegacyCalls:   1,
			wantFallbackDelta: 1,
		},
		{
			name:              "permission denial is not masked",
			authorizeErr:      status.Error(codes.PermissionDenied, "denied"),
			wantErr:           status.Error(codes.PermissionDenied, "denied"),
			wantFallbackDelta: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &evaluationReaderStub{
				authorizeErr: test.authorizeErr,
				legacyErr:    test.legacyErr,
			}
			reader := NewEvaluationBFFReader(client, nil, nil)
			before := testutil.ToFloat64(assessmentAuthorizationFallbackTotal)

			err := reader.AuthorizeAssessment(context.Background(), 11, 22)

			if status.Code(err) != status.Code(test.wantErr) || (test.wantErr != nil && err.Error() != test.wantErr.Error()) {
				t.Fatalf("AuthorizeAssessment() error = %v, want %v", err, test.wantErr)
			}
			if client.legacyCalls != test.wantLegacyCalls {
				t.Fatalf("legacy calls = %d, want %d", client.legacyCalls, test.wantLegacyCalls)
			}
			if delta := testutil.ToFloat64(assessmentAuthorizationFallbackTotal) - before; delta != test.wantFallbackDelta {
				t.Fatalf("fallback metric delta = %v, want %v", delta, test.wantFallbackDelta)
			}
		})
	}
}

type evaluationReaderStub struct {
	EvaluationReader
	authorizeErr error
	legacyErr    error
	legacyCalls  int
}

func (s *evaluationReaderStub) AuthorizeAssessment(context.Context, uint64, uint64) error {
	return s.authorizeErr
}

func (s *evaluationReaderStub) GetMyAssessment(context.Context, uint64, uint64) (*AssessmentDetailOutput, error) {
	s.legacyCalls++
	return &AssessmentDetailOutput{}, s.legacyErr
}
