package service

import (
	"context"
	"errors"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	source "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reportsource"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	report "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/pkg/delegatedsubject"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/serviceidentity"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type admissionWorkflowFixture struct {
	workflowParticipantFixture
	current   *source.Current
	err       error
	submitted int
}

func (f *admissionWorkflowFixture) Original(context.Context, string) (*app.Start, error) {
	return nil, app.ErrNotFound
}
func (f *admissionWorkflowFixture) StageStart(context.Context, app.Start) error {
	f.submitted++
	return f.err
}
func (f *admissionWorkflowFixture) ResolveCurrent(context.Context, meta.ID) (*source.Current, error) {
	return f.current, nil
}

func TestMQDelegatedMaintenanceKeepsAuthorizationAndUnknownFailureDistinct(t *testing.T) {
	r, e := report.RestoreInterpretReport(report.InterpretReportInput{
		ID: meta.FromUint64(99), GenerationID: meta.FromUint64(100), OutcomeID: meta.FromUint64(101), InterpretationRunID: meta.FromUint64(102),
		Association: report.Association{OrgID: 1, AssessmentID: meta.FromUint64(42), TesteeID: 7}, ReportType: policy.ReportTypeStandard, TemplateVersion: policy.TemplateVersionCurrent,
		ContentSchemaVersion: "standard-v1", BuilderIdentity: "isolated-test", GeneratedAt: time.Now(), Content: report.Content{Conclusion: "fixture"},
	})
	if e != nil {
		t.Fatal(e)
	}
	for _, scenario := range []struct {
		name      string
		err       error
		want      codes.Code
		submitted int
	}{
		{"maintenance", app.ErrRuntimeAdmissionClosed, codes.ResourceExhausted, 1},
		{"storage_unknown", errors.New("private driver detail"), codes.Internal, 1},
		{"revoked", app.ErrRuntimeAdmissionClosed, codes.PermissionDenied, 0},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f := &admissionWorkflowFixture{workflowParticipantFixture: workflowParticipantFixture{allowed: scenario.name != "revoked"}, err: scenario.err, current: &source.Current{Report: r, Outcome: evaluationfact.NewRecord(evaluationfact.NewRecordInput{ID: r.OutcomeID(), OrgID: 1, TesteeID: 7, AssessmentID: meta.FromUint64(42)})}}
			options := &delegatedsubject.Options{Enabled: true, CurrentKey: "isolated-test", TTL: time.Minute}
			signer, e := delegatedsubject.NewSignerFromOptions(options)
			if e != nil {
				t.Fatal(e)
			}
			verifier, e := delegatedsubject.NewVerifierFromOptions(options)
			if e != nil {
				t.Fatal(e)
			}
			s := NewParticipantAIExplanationService(verifier)
			s.CurrentAccess = &app.CurrentAccess{Testees: f, Links: f, Assessments: f}
			s.Workflow = &app.Participant{Access: f, Sources: f, Bridge: &app.Service{Store: f}}
			token, e := signer.Sign(delegatedsubject.SignInput{UserID: "42", TesteeID: 7, Purpose: delegatedsubject.PurposeAIExplanationRequest, TTL: time.Minute})
			if e != nil {
				t.Fatal(e)
			}
			ctx := withMTLSWorkload(metadata.NewIncomingContext(t.Context(), metadata.Pairs(delegatedsubject.MetadataKey, token)), serviceidentity.CollectionServerCertificateCommonName)
			result, e := s.RequestAIWorkflow(ctx, &pb.RequestAIWorkflowRequest{TesteeId: 7, AssessmentId: 42, ReportId: 99, RequestId: "00000000-0000-4000-8000-000000000001"})
			if result != nil || status.Code(e) != scenario.want || f.submitted != scenario.submitted {
				t.Fatalf("result=%v error=%v submits=%d", result, e, f.submitted)
			}
			if scenario.name == "maintenance" && status.Convert(e).Message() != app.RuntimeAdmissionClosedReason {
				t.Fatal("fixed maintenance reason lost", e)
			}
			if scenario.name == "storage_unknown" && status.Convert(e).Message() != "AI explanation request failed" {
				t.Fatal("unknown storage failure leaked or became definitive refusal", e)
			}
		})
	}
}
