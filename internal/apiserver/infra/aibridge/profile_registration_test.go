package aibridge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

type profileRPCStub struct {
	pb.ProfileManagementClient
	response *pb.ProfileRegistrationReceipt
	err      error
	calls    int
	scope    *pb.PublicationScope
	deadline bool
}

func (s *profileRPCStub) mark(ctx context.Context, scope *pb.PublicationScope) {
	s.calls++
	s.scope = scope
	at, ok := ctx.Deadline()
	s.deadline = ok && time.Until(at) > 0 && time.Until(at) <= 5*time.Second
}
func (s *profileRPCStub) Register(ctx context.Context, v *pb.ProfileRegisterCommand, _ ...grpc.CallOption) (*pb.ProfileRegistrationReceipt, error) {
	s.mark(ctx, v.Scope)
	return s.response, s.err
}
func (s *profileRPCStub) GetReceipt(ctx context.Context, v *pb.ProfileRegistrationQuery, _ ...grpc.CallOption) (*pb.ProfileRegistrationReceipt, error) {
	s.mark(ctx, v.Scope)
	return s.response, s.err
}
func profileFixture() (app.RegisterProfile, app.ProfileRegistrationReceipt) {
	ref := app.PromptDraftSource{Identity: "prompt", Version: "v1", Fingerprint: "sha256:" + strings.Repeat("a", 64), ContentSHA256: strings.Repeat("b", 64)}
	command := app.RegisterProfile{PromptDraftCommand: app.PromptDraftCommand{CommandID: publicationTestID, Reason: "配置改进"}, Source: ref, Prompt: ref, GenerationRoute: ref,
		DefinitionJSON: `{"version":"v2", "profile_id":"profile", "note":"中文<&>\u2028", "generation_policy":{"prompt_template_id":"prompt","prompt_version":"v1","provider_route":"prompt","input_schema_version":"prompt/v1","output_schema_version":"prompt/v1"}}`}
	var body map[string]any
	_ = json.Unmarshal([]byte(command.DefinitionJSON), &body)
	canonical, _ := json.Marshal(body)
	sum := sha256.Sum256(canonical)
	checksum := hex.EncodeToString(sum[:])
	profile := app.PromptDraftSource{Identity: "profile", Version: "v2", Fingerprint: "sha256:" + checksum, ContentSHA256: checksum}
	value := app.ProfileRegistrationReceipt{Scope: app.DraftScope{OrganizationID: 7, OperatorUserID: 42}, Command: command, Manifest: app.RegisteredManifest{Profile: profile, Prompt: ref, GenerationRoute: ref, InputSchema: ref, OutputSchema: ref}, RegisteredAt: "2026-09-13T00:00:00Z"}
	return command, value
}
func TestProfileReceiptBindsScopeCommandDefinitionAndReferences(t *testing.T) {
	for _, method := range []string{"register", "receipt"} {
		for _, damage := range []string{"", "org", "actor", "command", "reason", "definition", "profile", "checksum", "prompt", "route", "input", "output", "time", "schema", "envelope", "oversize", "nil", "timeout"} {
			t.Run(method+"/"+damage, func(t *testing.T) {
				command, value := profileFixture()
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
				case "definition":
					value.Command.DefinitionJSON = `{"profile_id":"other","version":"v2"}`
				case "profile":
					value.Manifest.Profile.Identity = "other"
				case "checksum":
					value.Manifest.Profile.ContentSHA256 = strings.Repeat("0", 64)
				case "prompt":
					value.Manifest.Prompt.Identity = "other"
				case "route":
					value.Manifest.GenerationRoute.Version = "v2"
				case "input":
					value.Manifest.InputSchema.Version = "v2"
				case "output":
					value.Manifest.OutputSchema.Version = "v2"
				case "time":
					value.RegisteredAt = "yesterday"
				}
				raw, _ := json.Marshal(value)
				response := &pb.ProfileRegistrationReceipt{SchemaVersion: "qs-ai-profile-registration/v1", CommandId: command.CommandID, ReceiptJson: string(raw)}
				switch damage {
				case "schema":
					response.SchemaVersion = "other"
				case "envelope":
					response.CommandId = "other"
				case "oversize":
					response.ReceiptJson = strings.Repeat(" ", 512*1024+1)
				case "nil":
					response = nil
				}
				stub := &profileRPCStub{response: response}
				if damage == "timeout" {
					stub.err = status.Error(codes.DeadlineExceeded, "private")
				}
				client := &ProfileClient{RPC: stub}
				var err error
				if method == "register" {
					_, err = client.RegisterProfile(context.Background(), scope, command)
				} else {
					_, err = client.GetProfileReceipt(context.Background(), scope, command.CommandID)
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
