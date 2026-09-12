package aibridge

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type draftRPCStub struct {
	pb.PromptDraftManagementClient
	response *pb.PromptDraftState
	err      error
	calls    int
	scope    *pb.PublicationScope
	revision *int64
	deadline bool
}

func (s *draftRPCStub) track(ctx context.Context, scope *pb.PublicationScope) (*pb.PromptDraftState, error) {
	s.calls++
	s.scope = scope
	end, ok := ctx.Deadline()
	s.deadline = ok && time.Until(end) <= 5*time.Second
	return s.response, s.err
}
func (s *draftRPCStub) Create(ctx context.Context, q *pb.PromptDraftCreateCommand, _ ...grpc.CallOption) (*pb.PromptDraftState, error) {
	return s.track(ctx, q.Scope)
}
func (s *draftRPCStub) Revise(ctx context.Context, q *pb.PromptDraftReviseCommand, _ ...grpc.CallOption) (*pb.PromptDraftState, error) {
	return s.track(ctx, q.Scope)
}
func (s *draftRPCStub) Get(ctx context.Context, q *pb.PromptDraftQuery, _ ...grpc.CallOption) (*pb.PromptDraftState, error) {
	s.revision = q.Revision
	return s.track(ctx, q.Scope)
}
func (s *draftRPCStub) GetReceipt(ctx context.Context, q *pb.PromptDraftReceiptQuery, _ ...grpc.CallOption) (*pb.PromptDraftState, error) {
	return s.track(ctx, q.Scope)
}
func draftTestState() app.PromptDraftState {
	return app.PromptDraftState{DraftID: publicationTestID, OrganizationID: 7, OperatorUserID: 42, TemplateID: "p", TargetVersion: "v2", Source: app.PromptDraftSource{Identity: "p", Version: "v1", Fingerprint: "sha256:" + strings.Repeat("a", 64), ContentSHA256: strings.Repeat("b", 64)}, Revision: 1, CommandID: publicationTestID, Reason: "修订", SavedAt: "2026-09-13T00:00:00Z", Content: app.PromptDraftContent{SystemMessage: "草稿 {{", AllowedPlaceholders: []string{}}}
}
func draftTestResponse(v app.PromptDraftState) *pb.PromptDraftState {
	raw, _ := json.Marshal(v)
	return &pb.PromptDraftState{SchemaVersion: "qs-ai-prompt-draft/v1", DraftId: v.DraftID, Revision: v.Revision, SnapshotJson: string(raw)}
}
func TestDraftClientBindsStateAndConfirmation(t *testing.T) {
	expected := draftTestState()
	scope := app.DraftScope{OrganizationID: 7, OperatorUserID: 42}
	for _, method := range []string{"create", "revise", "get", "receipt"} {
		for _, bad := range []string{"", "org", "id", "revision", "command", "actor", "reason", "source", "target", "content", "envelope", "schema", "oversize", "timeout"} {
			t.Run(method+"/"+bad, func(t *testing.T) {
				v := expected
				if method == "revise" {
					v.Revision = 2
				}
				switch bad {
				case "org":
					v.OrganizationID = 8
				case "id":
					v.DraftID = "00000000-0000-4000-8000-000000000002"
				case "revision":
					v.Revision = 4
				case "command":
					v.CommandID = "00000000-0000-4000-8000-000000000002"
				case "actor":
					v.OperatorUserID = 43
				case "reason":
					v.Reason = "changed"
				case "source":
					v.Source.ContentSHA256 = strings.Repeat("c", 64)
				case "target":
					v.TargetVersion = "v3"
				case "content":
					v.Content.SystemMessage = "changed"
				}
				response := draftTestResponse(v)
				switch bad {
				case "envelope":
					response.Revision = 9
				case "schema":
					response.SchemaVersion = "wrong"
				case "oversize":
					response.SnapshotJson = strings.Repeat(" ", 256*1024+1)
				}
				stub := &draftRPCStub{response: response}
				if bad == "timeout" {
					stub.err = status.Error(codes.DeadlineExceeded, "private")
				}
				client := &PromptDraftClient{RPC: stub}
				var err error
				command := app.PromptDraftCommand{CommandID: expected.CommandID, Reason: expected.Reason}
				switch method {
				case "create":
					_, err = client.CreatePromptDraft(context.Background(), scope, expected.DraftID, app.CreatePromptDraft{PromptDraftCommand: command, Source: expected.Source, TemplateID: expected.TemplateID, TargetVersion: expected.TargetVersion})
				case "revise":
					_, err = client.RevisePromptDraft(context.Background(), scope, expected.DraftID, app.RevisePromptDraft{PromptDraftCommand: command, ExpectedRevision: 1, Content: &expected.Content})
				case "get":
					rev := int64(1)
					_, err = client.GetPromptDraft(context.Background(), scope, expected.DraftID, &rev)
					if stub.revision == nil || *stub.revision != 1 {
						t.Fatal("lost revision")
					}
				case "receipt":
					_, err = client.GetPromptDraftReceipt(context.Background(), scope, expected.CommandID)
				}
				reject := bad == "org" || bad == "envelope" || bad == "schema" || bad == "oversize" || bad == "timeout" || bad == "id" && method != "receipt" || bad == "revision" && method != "receipt" || bad == "command" && method != "get" || bad == "actor" && method != "get" || bad == "reason" && (method == "create" || method == "revise") || bad == "source" && method == "create" || bad == "target" && method == "create" || bad == "content" && method == "revise"
				if reject != (err != nil) {
					t.Fatal("confirmation binding", reject, err)
				}
				if stub.calls != 1 || !stub.deadline || stub.scope.OrganizationId != 7 || stub.scope.OperatorUserId != 42 {
					t.Fatal("scope/deadline changed or retried")
				}
			})
		}
	}
}
