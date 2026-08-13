package grpcbridge

import (
	"context"
	"errors"
	"testing"
)

func TestAuthorizeAssessmentDelegatesToAuthorizationRPC(t *testing.T) {
	wantErr := errors.New("authorization failed")
	client := &evaluationReaderStub{authorizeErr: wantErr}
	reader := NewEvaluationBFFReader(client, nil, nil)

	err := reader.AuthorizeAssessment(context.Background(), 11, 22)

	if !errors.Is(err, wantErr) {
		t.Fatalf("AuthorizeAssessment() error = %v, want %v", err, wantErr)
	}
	if client.authorizeCalls != 1 {
		t.Fatalf("authorization calls = %d, want 1", client.authorizeCalls)
	}
}

type evaluationReaderStub struct {
	EvaluationReader
	authorizeErr   error
	authorizeCalls int
}

func (s *evaluationReaderStub) AuthorizeAssessment(context.Context, uint64, uint64) error {
	s.authorizeCalls++
	return s.authorizeErr
}
