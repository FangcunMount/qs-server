package service

import (
	"context"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/FangcunMount/qs-server/internal/pkg/delegatedsubject"
	"github.com/FangcunMount/qs-server/internal/pkg/serviceidentity"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"testing"
	"time"
)

type operationWorkflowFixture struct {
	workflowParticipantFixture
	operationReads int
}

func (f *operationWorkflowFixture) ReadRequestOperation(_ context.Context, scope app.OperationScope, request, id string) (app.MessagingOperation, error) {
	f.operationReads++
	return app.MessagingOperation{OperationID: id, CommandID: id, Status: "submitted", TransportStatus: "awaiting_receipt"}, nil
}
func TestMQDelegatedOperationUsesCurrentParticipantAuthorizationWithoutNewACL(t *testing.T) {
	for _, scenario := range []string{"allowed", "revoked", "wrong_purpose", "wrong_workload", "different_org"} {
		t.Run(scenario, func(t *testing.T) {
			o := &delegatedsubject.Options{Enabled: true, CurrentKey: "local-test", TTL: time.Minute}
			signer, _ := delegatedsubject.NewSignerFromOptions(o)
			verifier, _ := delegatedsubject.NewVerifierFromOptions(o)
			f := &operationWorkflowFixture{workflowParticipantFixture: workflowParticipantFixture{allowed: scenario != "revoked"}}
			s := NewParticipantAIExplanationService(verifier)
			s.CurrentAccess = &app.CurrentAccess{Testees: f, Links: f, Assessments: f}
			s.Workflow = &app.Participant{Access: f, Bridge: &app.Service{Store: f}}
			purpose := delegatedsubject.PurposeAIExplanationGet
			cn := serviceidentity.CollectionServerCertificateCommonName
			org := uint64(0)
			if scenario == "wrong_purpose" {
				purpose = delegatedsubject.PurposeAIExplanationRequest
			}
			if scenario == "wrong_workload" {
				cn = "qs-ai.svc"
			}
			if scenario == "different_org" {
				org = 2
			}
			token, e := signer.Sign(delegatedsubject.SignInput{UserID: "42", TesteeID: 7, OrgID: org, Purpose: purpose, TTL: time.Minute})
			if e != nil {
				t.Fatal(e)
			}
			ctx := withMTLSWorkload(metadata.NewIncomingContext(context.Background(), metadata.Pairs(delegatedsubject.MetadataKey, token)), cn)
			const id = "00000000-0000-4000-8000-000000000001"
			r, e := s.GetAIWorkflow(ctx, &pb.GetAIWorkflowRequest{TesteeId: 7, AssessmentId: 42, RequestId: id, CommandId: id})
			if scenario == "allowed" {
				if e != nil || r.Operation == nil || r.Operation.Status != "submitted" || r.Operation.TransportStatus != "awaiting_receipt" || f.operationReads != 1 {
					t.Fatalf("operation=%v err=%v", r, e)
				}
			} else if status.Code(e) != codes.PermissionDenied || f.operationReads != 0 || f.reads != 0 {
				t.Fatalf("authorization=%v reads=%d", e, f.reads)
			}
		})
	}
}
