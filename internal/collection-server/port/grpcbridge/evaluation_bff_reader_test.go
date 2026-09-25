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

func TestRuntimeStatusReadPreservesFreshAttemptPhase(t *testing.T) {
	client := &evaluationReaderStub{runtime: &AssessmentRuntimeStatusOutput{Attempt: 2, Status: "running"}}
	reader := NewEvaluationBFFReader(client, nil, nil)
	got, err := reader.GetMyAssessmentRunStatus(context.Background(), 11, 22)
	if err != nil || got == nil || got.Attempt != 2 || got.Status != "running" {
		t.Fatalf("runtime status = %+v, err = %v", got, err)
	}
	if client.runtimeCalls != 1 {
		t.Fatalf("runtime calls = %d, want 1", client.runtimeCalls)
	}
}

type evaluationReaderStub struct {
	EvaluationReader
	authorizeErr   error
	authorizeCalls int
	runtime        *AssessmentRuntimeStatusOutput
	runtimeCalls   int
}

func (s *evaluationReaderStub) AuthorizeAssessment(context.Context, uint64, uint64) error {
	s.authorizeCalls++
	return s.authorizeErr
}

func (s *evaluationReaderStub) GetMyAssessmentRunStatus(context.Context, uint64, uint64) (*AssessmentRuntimeStatusOutput, error) {
	s.runtimeCalls++
	return s.runtime, nil
}
