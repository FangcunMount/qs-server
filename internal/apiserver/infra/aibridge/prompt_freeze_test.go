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

type freezeRPCStub struct {
	pb.PromptDraftManagementClient
	response *pb.PromptDraftFreezeReceipt
	calls    int
	scope    *pb.PublicationScope
	deadline bool
	err      error
}

func (s *freezeRPCStub) track(ctx context.Context, scope *pb.PublicationScope) (*pb.PromptDraftFreezeReceipt, error) {
	s.calls++
	s.scope = scope
	end, ok := ctx.Deadline()
	s.deadline = ok && time.Until(end) <= 5*time.Second
	return s.response, s.err
}
func (s *freezeRPCStub) Freeze(ctx context.Context, q *pb.PromptDraftFreezeCommand, _ ...grpc.CallOption) (*pb.PromptDraftFreezeReceipt, error) {
	return s.track(ctx, q.Scope)
}
func (s *freezeRPCStub) GetFreezeReceipt(ctx context.Context, q *pb.PromptDraftReceiptQuery, _ ...grpc.CallOption) (*pb.PromptDraftFreezeReceipt, error) {
	return s.track(ctx, q.Scope)
}
func TestFreezeClientBindsOriginalConfirmationAndDoesNotRetry(t *testing.T) {
	scope := app.DraftScope{OrganizationID: 7, OperatorUserID: 42}
	original := app.FrozenPromptReceipt{Scope: scope, Command: app.FrozenPromptCommand{DraftID: publicationTestID, CommandID: publicationTestID, ExpectedRevision: 2, Reason: "冻结"}, Asset: draftTestState().Source, SnapshotSHA256: strings.Repeat("c", 64), ValidatorVersion: "qs-ai-prompt-syntax/v1", FrozenAt: "2026-09-13T01:00:00Z"}
	command := app.FreezePromptDraft{PromptDraftCommand: app.PromptDraftCommand{CommandID: publicationTestID, Reason: "冻结"}, ExpectedRevision: 2}
	for _, method := range []string{"freeze", "receipt"} {
		for _, bad := range []string{"", "org", "actor", "command", "draft", "revision", "reason", "asset", "snapshot", "validator", "date", "envelope", "schema", "oversize", "nil", "timeout"} {
			t.Run(method+"/"+bad, func(t *testing.T) {
				receipt := original
				switch bad {
				case "org":
					receipt.Scope.OrganizationID = 8
				case "actor":
					receipt.Scope.OperatorUserID = 43
				case "command":
					receipt.Command.CommandID = "00000000-0000-4000-8000-000000000002"
				case "draft":
					receipt.Command.DraftID = "00000000-0000-4000-8000-000000000002"
				case "revision":
					receipt.Command.ExpectedRevision = 3
				case "reason":
					receipt.Command.Reason = "changed"
				case "asset":
					receipt.Asset.ContentSHA256 = "bad"
				case "snapshot":
					receipt.SnapshotSHA256 = "bad"
				case "validator":
					receipt.ValidatorVersion = "unknown"
				case "date":
					receipt.FrozenAt = "yesterday"
				}
				raw, _ := json.Marshal(receipt)
				response := &pb.PromptDraftFreezeReceipt{SchemaVersion: "qs-ai-prompt-freeze/v1", CommandId: publicationTestID, ReceiptJson: string(raw)}
				switch bad {
				case "envelope":
					response.CommandId = "wrong"
				case "schema":
					response.SchemaVersion = "wrong"
				case "oversize":
					response.ReceiptJson = strings.Repeat(" ", 32769)
				case "nil":
					response = nil
				}
				stub := &freezeRPCStub{response: response}
				if bad == "timeout" {
					stub.err = status.Error(codes.DeadlineExceeded, "private")
				}
				client := &PromptDraftClient{RPC: stub}
				var err error
				if method == "freeze" {
					_, err = client.FreezePromptDraft(context.Background(), scope, publicationTestID, command)
				} else {
					_, err = client.GetPromptFreezeReceipt(context.Background(), scope, publicationTestID)
				}
				reject := bad != "" && (method != "receipt" || (bad != "draft" && bad != "revision" && bad != "reason"))
				if reject != (err != nil) {
					t.Fatal("bad confirmation", reject, err)
				}
				if stub.calls != 1 || !stub.deadline || stub.scope.OrganizationId != 7 || stub.scope.OperatorUserId != 42 {
					t.Fatal("scope/deadline changed or retried")
				}
			})
		}
	}
}
