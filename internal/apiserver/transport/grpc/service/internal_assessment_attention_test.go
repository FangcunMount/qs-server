package service

import (
	"context"
	"testing"

	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/internalapi"
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

func TestInternalServiceDeprecatedTagTesteeBridgesWithoutRiskTags(t *testing.T) {
	fake := &fakeAssessmentAttentionService{
		result: &testeeApp.AssessmentAttentionResult{KeyFocusMarked: true},
	}
	svc := &InternalService{assessmentAttentionService: fake}

	resp, err := svc.TagTestee(context.Background(), &pb.TagTesteeRequest{
		TesteeId:        20,
		RiskLevel:       "high",
		ScaleCode:       "scale-a",
		MarkKeyFocus:    true,
		HighRiskFactors: []string{"factor-a"},
	})
	if err != nil {
		t.Fatalf("TagTestee returned error: %v", err)
	}

	if !resp.Success || !resp.KeyFocusMarked {
		t.Fatalf("unexpected response: %#v", resp)
	}
	if len(resp.TagsAdded) != 0 {
		t.Fatalf("tags_added = %v, want empty", resp.TagsAdded)
	}
	if fake.calls != 1 || fake.testeeID != 20 || fake.riskLevel != "high" || !fake.markKeyFocus {
		t.Fatalf("unexpected service call: %#v", fake)
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
