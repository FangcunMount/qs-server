package service

import (
	"context"
	"testing"
	"time"

	basegrpc "github.com/FangcunMount/component-base/pkg/grpc/interceptors"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	interpretationParticipant "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/participant"
	"github.com/FangcunMount/qs-server/internal/pkg/delegatedsubject"
	"github.com/FangcunMount/qs-server/internal/pkg/serviceidentity"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func testDelegatedVerifier(t *testing.T) *delegatedsubject.Verifier {
	t.Helper()
	verifier, err := delegatedsubject.NewVerifierFromOptions(&delegatedsubject.Options{
		Enabled:    true,
		CurrentKey: "test-current-key",
		TTL:        time.Minute,
	})
	if err != nil {
		t.Fatalf("NewVerifierFromOptions() error = %v", err)
	}
	return verifier
}

func testDelegatedContext(t *testing.T, testeeID uint64) context.Context {
	t.Helper()
	signer, err := delegatedsubject.NewSignerFromOptions(&delegatedsubject.Options{
		Enabled:    true,
		CurrentKey: "test-current-key",
		TTL:        time.Minute,
	})
	if err != nil {
		t.Fatalf("NewSignerFromOptions() error = %v", err)
	}
	raw, err := signer.Sign(delegatedsubject.SignInput{
		UserID:   "42",
		TesteeID: testeeID,
		Purpose:  delegatedsubject.PurposeGetAssessmentReport,
		TTL:      time.Minute,
	})
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(delegatedsubject.MetadataKey, raw))
}

func withMTLSWorkload(ctx context.Context, serviceName string) context.Context {
	return basegrpc.ContextWithServiceIdentity(ctx, &basegrpc.ServiceIdentity{
		ServiceName: serviceName,
		CommonName:  serviceName,
	})
}

func TestParticipantReportServiceRejectsMissingDelegatedSubject(t *testing.T) {
	svc := NewParticipantReportService(&fakeParticipantReportService{}, testDelegatedVerifier(t))
	_, err := svc.GetAssessmentReport(context.Background(), &pb.GetAssessmentReportRequest{TesteeId: 7, AssessmentId: 42})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("code = %v, want Unauthenticated", status.Code(err))
	}
}

func TestParticipantReportServiceRejectsTamperedTesteeInRequest(t *testing.T) {
	svc := NewParticipantReportService(&fakeParticipantReportService{}, testDelegatedVerifier(t))
	ctx := testDelegatedContext(t, 7)
	_, err := svc.GetAssessmentReport(ctx, &pb.GetAssessmentReportRequest{TesteeId: 8, AssessmentId: 42})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied", status.Code(err))
	}
}

func TestParticipantReportServiceRejectsExpiredDelegatedSubject(t *testing.T) {
	raw, err := delegatedsubject.SignWithExpiryForTest("test-current-key", delegatedsubject.SignInput{
		UserID:   "42",
		TesteeID: 7,
		Purpose:  delegatedsubject.PurposeGetAssessmentReport,
	}, time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatalf("SignWithExpiryForTest() error = %v", err)
	}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(delegatedsubject.MetadataKey, raw))
	svc := NewParticipantReportService(&fakeParticipantReportService{}, testDelegatedVerifier(t))
	_, err = svc.GetAssessmentReport(ctx, &pb.GetAssessmentReportRequest{TesteeId: 7, AssessmentId: 42})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("code = %v, want Unauthenticated", status.Code(err))
	}
}

func TestParticipantReportServiceRejectsBadDelegatedSignature(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(delegatedsubject.MetadataKey, "bad.token"))
	svc := NewParticipantReportService(&fakeParticipantReportService{}, testDelegatedVerifier(t))
	_, err := svc.GetAssessmentReport(ctx, &pb.GetAssessmentReportRequest{TesteeId: 7, AssessmentId: 42})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("code = %v, want Unauthenticated", status.Code(err))
	}
}

func TestParticipantReportServiceAllowsValidDelegationButRejectsWrongAssessment(t *testing.T) {
	svc := NewParticipantReportService(
		&fakeParticipantReportService{err: evalerrors.Forbidden("无权访问此测评")},
		testDelegatedVerifier(t),
	)
	ctx := testDelegatedContext(t, 7)
	_, err := svc.GetAssessmentReport(ctx, &pb.GetAssessmentReportRequest{TesteeId: 7, AssessmentId: 42})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied", status.Code(err))
	}
}

func TestParticipantReportServiceReturnsReportWithValidDelegation(t *testing.T) {
	reportSvc := &fakeParticipantReportService{report: &interpretationParticipant.Report{AssessmentID: 42}}
	svc := NewParticipantReportService(reportSvc, testDelegatedVerifier(t))
	ctx := withMTLSWorkload(testDelegatedContext(t, 7), serviceidentity.CollectionServerCertificateCommonName)
	resp, err := svc.GetAssessmentReport(ctx, &pb.GetAssessmentReportRequest{TesteeId: 7, AssessmentId: 42})
	if err != nil {
		t.Fatalf("GetAssessmentReport() error = %v", err)
	}
	if resp.GetReport().GetAssessmentId() != 42 {
		t.Fatalf("assessment_id = %d, want 42", resp.GetReport().GetAssessmentId())
	}
}

func TestParticipantReportServiceRejectsUntrustedWorkloadIdentity(t *testing.T) {
	svc := NewParticipantReportService(&fakeParticipantReportService{}, testDelegatedVerifier(t))
	ctx := testDelegatedContext(t, 7)
	ctx = withMTLSWorkload(ctx, "qs-worker.svc")
	_, err := svc.GetAssessmentReport(ctx, &pb.GetAssessmentReportRequest{TesteeId: 7, AssessmentId: 42})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied", status.Code(err))
	}
}
