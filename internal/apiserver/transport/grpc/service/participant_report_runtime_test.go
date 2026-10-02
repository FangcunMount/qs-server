package service

import (
	"context"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	participant "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/participant"
	"github.com/FangcunMount/qs-server/internal/pkg/delegatedsubject"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/FangcunMount/qs-server/internal/pkg/serviceidentity"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type runtimeQueryRecorder struct {
	calls int
	actor participant.Actor
	id    uint64
}

func (r *runtimeQueryRecorder) Get(_ context.Context, a participant.Actor, id uint64) (*participant.RuntimeStatus, error) {
	r.calls++
	r.actor = a
	r.id = id
	return &participant.RuntimeStatus{Status: "failed", Attempt: 2, RetryDisposition: retrygovernance.DispositionManualRequired}, nil
}
func reportRuntimeDelegation(t *testing.T, purpose string, testeeID uint64) context.Context {
	t.Helper()
	signer, err := delegatedsubject.NewSignerFromOptions(&delegatedsubject.Options{Enabled: true, CurrentKey: "test-current-key", TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := signer.Sign(delegatedsubject.SignInput{UserID: "42", TesteeID: testeeID, Purpose: purpose, TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	return withMTLSWorkload(metadata.NewIncomingContext(t.Context(), metadata.Pairs(delegatedsubject.MetadataKey, raw)), serviceidentity.CollectionServerCertificateCommonName)
}
func TestReportRuntimeRPCRequiresOwnSignedPurpose(t *testing.T) {
	r := &runtimeQueryRecorder{}
	s := NewParticipantReportService(&fakeParticipantReportService{}, testDelegatedVerifier(t), r)
	req := &pb.GetAssessmentReportRequest{TesteeId: 7, AssessmentId: 42}
	for _, tc := range []struct {
		name string
		ctx  context.Context
		want codes.Code
	}{
		{"missing", t.Context(), codes.Unauthenticated},
		{"wrong_purpose", reportRuntimeDelegation(t, delegatedsubject.PurposeGetAssessmentReport, 7), codes.PermissionDenied},
		{"wrong_testee", reportRuntimeDelegation(t, delegatedsubject.PurposeGetAssessmentReportStatus, 8), codes.PermissionDenied},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := s.GetAssessmentReportStatus(tc.ctx, req)
			if status.Code(err) != tc.want || r.calls != 0 {
				t.Fatalf("code=%v calls=%d", status.Code(err), r.calls)
			}
		})
	}
	got, err := s.GetAssessmentReportStatus(reportRuntimeDelegation(t, delegatedsubject.PurposeGetAssessmentReportStatus, 7), req)
	if err != nil || !got.GetExists() || got.GetStatus() != "failed" || got.GetAttempt() != 2 || got.GetRetryDisposition() != "manual_required" || r.actor.TesteeID != 7 || r.id != 42 || r.calls != 1 {
		t.Fatalf("response=%v err=%v reader=%+v", got, err, r)
	}
}
