package service

import (
	"context"
	"testing"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestInternalServiceSyncAssessmentAttention(t *testing.T) {
	fake := &fakeAssessmentAttentionService{
		result: &testeeApp.AssessmentAttentionResult{KeyFocusMarked: true},
	}
	svc := &InternalService{assessmentAttentionService: fake}

	resp, err := svc.SyncAssessmentAttention(context.Background(), &pb.SyncAssessmentAttentionRequest{
		TesteeId:     10,
		RiskLevel:    "severe",
		MarkKeyFocus: true,
	})
	if err != nil {
		t.Fatalf("SyncAssessmentAttention returned error: %v", err)
	}

	if !resp.Success || !resp.KeyFocusMarked {
		t.Fatalf("unexpected response: %#v", resp)
	}
	if fake.calls != 1 || fake.testeeID != 10 || fake.riskLevel != "severe" || !fake.markKeyFocus {
		t.Fatalf("unexpected service call: %#v", fake)
	}
}

func TestInternalServiceSyncAssessmentAttentionRejectsMissingTesteeID(t *testing.T) {
	svc := &InternalService{assessmentAttentionService: &fakeAssessmentAttentionService{}}

	_, err := svc.SyncAssessmentAttention(context.Background(), &pb.SyncAssessmentAttentionRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
}

type fakeAssessmentAttentionService struct {
	calls        int
	testeeID     uint64
	riskLevel    string
	markKeyFocus bool
	result       *testeeApp.AssessmentAttentionResult
	err          error
}

func (s *fakeAssessmentAttentionService) SyncAssessmentAttention(
	_ context.Context,
	testeeID uint64,
	riskLevel string,
	markKeyFocus bool,
) (*testeeApp.AssessmentAttentionResult, error) {
	s.calls++
	s.testeeID = testeeID
	s.riskLevel = riskLevel
	s.markKeyFocus = markKeyFocus
	if s.err != nil {
		return nil, s.err
	}
	if s.result != nil {
		return s.result, nil
	}
	return &testeeApp.AssessmentAttentionResult{}, nil
}
