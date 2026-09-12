package aibridge

import (
	"context"
	"encoding/json"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"strings"
	"testing"
	"time"
)

type suiteRPCStub struct {
	pb.SuiteManagementClient
	response *pb.SuiteRegistrationReceipt
	err      error
	calls    int
	scope    *pb.PublicationScope
	deadline bool
}

func (s *suiteRPCStub) mark(ctx context.Context, scope *pb.PublicationScope) {
	s.calls++
	s.scope = scope
	at, ok := ctx.Deadline()
	s.deadline = ok && time.Until(at) > 0 && time.Until(at) <= 5*time.Second
}
func (s *suiteRPCStub) Register(ctx context.Context, v *pb.SuiteRegisterCommand, _ ...grpc.CallOption) (*pb.SuiteRegistrationReceipt, error) {
	s.mark(ctx, v.Scope)
	return s.response, s.err
}
func (s *suiteRPCStub) GetReceipt(ctx context.Context, v *pb.SuiteRegistrationQuery, _ ...grpc.CallOption) (*pb.SuiteRegistrationReceipt, error) {
	s.mark(ctx, v.Scope)
	return s.response, s.err
}
func suiteFixture() (app.RegisterSuite, app.SuiteRegistrationReceipt) {
	ref := app.PromptDraftSource{Identity: "asset", Version: "v1", Fingerprint: "sha256:" + strings.Repeat("a", 64), ContentSHA256: strings.Repeat("b", 64)}
	source := app.FrozenEvaluationRef{ID: "source", Version: "v1", Fingerprint: "sha256:" + strings.Repeat("c", 64)}
	command := app.RegisterSuite{PromptDraftCommand: app.PromptDraftCommand{CommandID: publicationTestID, Reason: "为新配置评测"}, Source: source, SuiteID: "new-suite", SuiteVersion: "v2", Profile: ref, Prompt: ref, GenerationRoute: ref}
	value := app.SuiteRegistrationReceipt{Scope: app.DraftScope{OrganizationID: 7, OperatorUserID: 42}, Command: command, Suite: app.FrozenEvaluationRef{ID: command.SuiteID, Version: command.SuiteVersion, Fingerprint: "sha256:" + strings.Repeat("d", 64)}, Manifest: app.RegisteredManifest{Profile: ref, Prompt: ref, GenerationRoute: ref, InputSchema: ref, OutputSchema: ref}, RegisteredAt: "2026-09-13T00:00:00Z"}
	return command, value
}
func TestSuiteReceiptBindsScopeCommandDefinitionAndReferences(t *testing.T) {
	for _, method := range []string{"register", "receipt"} {
		for _, damage := range []string{"", "org", "actor", "command", "reason", "source", "suite", "version", "fingerprint", "profile", "checksum", "prompt", "route", "input", "output", "time", "schema", "envelope", "oversize", "nil", "timeout"} {
			t.Run(method+"/"+damage, func(t *testing.T) {
				command, value := suiteFixture()
				scope := value.Scope
				switch damage {
				case "org":
					value.Scope.OrganizationID++
				case "actor":
					value.Scope.OperatorUserID++
				case "command":
					value.Command.CommandID = "wrong"
				case "reason":
					value.Command.Reason = "another"
				case "source":
					value.Command.Source.Fingerprint = "invalid"
				case "suite":
					value.Suite.ID = "other"
				case "version":
					value.Suite.Version = "other"
				case "fingerprint":
					value.Suite.Fingerprint = "invalid"
				case "profile":
					value.Manifest.Profile.Identity = "other"
				case "checksum":
					value.Manifest.Profile.ContentSHA256 = strings.Repeat("0", 64)
				case "prompt":
					value.Manifest.Prompt.Identity = "other"
				case "route":
					value.Manifest.GenerationRoute.Version = "v2"
				case "input":
					value.Manifest.InputSchema.Version = " "
				case "output":
					value.Manifest.OutputSchema.Version = " "
				case "time":
					value.RegisteredAt = "yesterday"
				}
				raw, _ := json.Marshal(value)
				response := &pb.SuiteRegistrationReceipt{SchemaVersion: "qs-ai-suite-registration/v1", CommandId: command.CommandID, ReceiptJson: string(raw)}
				switch damage {
				case "schema":
					response.SchemaVersion = "other"
				case "envelope":
					response.CommandId = "other"
				case "oversize":
					response.ReceiptJson = strings.Repeat(" ", 32*1024+1)
				case "nil":
					response = nil
				}
				stub := &suiteRPCStub{response: response}
				if damage == "timeout" {
					stub.err = status.Error(codes.DeadlineExceeded, "private")
				}
				client := &SuiteClient{RPC: stub}
				var err error
				if method == "register" {
					_, err = client.RegisterSuite(context.Background(), scope, command)
				} else {
					_, err = client.GetSuiteReceipt(context.Background(), scope, command.CommandID)
				}
				reject := damage != "" && (method != "receipt" || damage != "reason")
				if reject != (err != nil) {
					t.Fatal("unexpected confirmation", reject, err)
				}
				if stub.calls != 1 || !stub.deadline || stub.scope.OrganizationId != 7 || stub.scope.OperatorUserId != 42 {
					t.Fatal("scope/deadline changed or retried")
				}
			})
		}
	}
}
