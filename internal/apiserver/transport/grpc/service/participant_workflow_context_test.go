package service

import (
	"context"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	bridge "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	evaluationtestee "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/testee"
	source "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/source"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/delegatedsubject"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/serviceidentity"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type workflowParticipantFixture struct {
	bridge.Store
	allowed bool
	reads   int
}

func (f *workflowParticipantFixture) GetTestee(context.Context, uint64) (*actorreadmodel.TesteeRow, error) {
	profile := uint64(9)
	return &actorreadmodel.TesteeRow{ID: 7, OrgID: 1, ProfileID: &profile}, nil
}
func (f *workflowParticipantFixture) HasActiveProfileLink(_ context.Context, user, profile string) (bool, error) {
	return f.allowed && user == "42" && profile == "9", nil
}
func (f *workflowParticipantFixture) AuthorizeAssessment(_ context.Context, actor evaluationtestee.Actor, id uint64) error {
	if actor.TesteeID != 7 || id != 42 {
		return bridge.ErrAccessDenied
	}
	return nil
}
func (f *workflowParticipantFixture) AuthorizeOwnAssessment(ctx context.Context, testee, assessment uint64) error {
	return f.AuthorizeAssessment(ctx, evaluationtestee.Actor{TesteeID: testee}, assessment)
}
func (f *workflowParticipantFixture) ResolveCurrent(context.Context, meta.ID) (*source.Current, error) {
	f.reads++
	return nil, source.ErrNotReady
}
func (f *workflowParticipantFixture) Original(_ context.Context, id string) (*bridge.Start, error) {
	f.reads++
	return &bridge.Start{RequestID: id, Actor: bridge.Actor{OrgID: "1", SubjectID: "42"}, TesteeID: "7", AssessmentIDs: []string{"42"}, Evidence: []bridge.EvidenceItem{{ReportID: "99"}}}, nil
}
func (f *workflowParticipantFixture) Projection(context.Context, string) (*bridge.Event, error) {
	return nil, nil
}

func TestParticipantWorkflowWithoutLoginOrganization(t *testing.T) {
	for _, operation := range []string{"source", "request", "read"} {
		for _, scenario := range []string{"allowed", "revoked", "different_org", "wrong_purpose", "unconfigured"} {
			t.Run(operation+"/"+scenario, func(t *testing.T) {
				options := &delegatedsubject.Options{Enabled: true, CurrentKey: "test-current-key", TTL: time.Minute}
				signer, _ := delegatedsubject.NewSignerFromOptions(options)
				verifier, _ := delegatedsubject.NewVerifierFromOptions(options)
				f := &workflowParticipantFixture{allowed: scenario != "revoked"}
				s := NewParticipantAIExplanationService(nil, nil, verifier)
				s.CurrentAccess = &bridge.CurrentAccess{Testees: f, Links: f, Assessments: f}
				s.Workflow = &bridge.Participant{Access: f, Sources: f, Bridge: &bridge.Service{Store: f}}
				purpose := map[string]string{"source": delegatedsubject.PurposeAIExplanationCapability, "request": delegatedsubject.PurposeAIExplanationRequest, "read": delegatedsubject.PurposeAIExplanationGet}[operation]
				org := uint64(0)
				want := codes.OK
				switch scenario {
				case "revoked":
					want = codes.PermissionDenied
				case "different_org":
					org = 2
					want = codes.PermissionDenied
				case "wrong_purpose":
					purpose = delegatedsubject.PurposeAIExplanationExport
					want = codes.PermissionDenied
				case "unconfigured":
					s.CurrentAccess = nil
					want = codes.Unavailable
				}
				raw, err := signer.Sign(delegatedsubject.SignInput{UserID: "42", TesteeID: 7, OrgID: org, Purpose: purpose, TTL: time.Minute})
				if err != nil {
					t.Fatal(err)
				}
				ctx := withMTLSWorkload(metadata.NewIncomingContext(context.Background(), metadata.Pairs(delegatedsubject.MetadataKey, raw)), serviceidentity.CollectionServerCertificateCommonName)
				const id = "00000000-0000-4000-8000-000000000001"
				switch operation {
				case "source":
					result, e := s.GetAIWorkflowSource(ctx, &pb.GetAIWorkflowSourceRequest{TesteeId: 7, AssessmentId: 42})
					err = e
					if err == nil && result.Status != "not_ready" {
						t.Fatal(result)
					}
				case "request":
					result, e := s.RequestAIWorkflow(ctx, &pb.RequestAIWorkflowRequest{TesteeId: 7, AssessmentId: 42, ReportId: 99, RequestId: id})
					err = e
					if err == nil && result.Status != "accepted" {
						t.Fatal(result)
					}
				case "read":
					result, e := s.GetAIWorkflow(ctx, &pb.GetAIWorkflowRequest{TesteeId: 7, AssessmentId: 42, RequestId: id})
					err = e
					if err == nil && result.Status != "accepted" {
						t.Fatal(result)
					}
				}
				if status.Code(err) != want {
					t.Fatalf("got %v want %v", err, want)
				}
				if want != codes.OK && f.reads != 0 {
					t.Fatal("denied actor reached workflow data")
				}
			})
		}
	}
}
