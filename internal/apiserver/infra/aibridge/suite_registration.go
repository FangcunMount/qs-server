package aibridge

import (
	"context"
	"encoding/json"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
	"time"
)

type SuiteClient struct{ RPC pb.SuiteManagementClient }

func NewSuiteClient(conn grpc.ClientConnInterface) *SuiteClient {
	return &SuiteClient{RPC: pb.NewSuiteManagementClient(conn)}
}
func suiteReceipt(raw *pb.SuiteRegistrationReceipt, scope app.DraftScope, id string) (app.SuiteRegistrationReceipt, error) {
	var v app.SuiteRegistrationReceipt
	if raw == nil || raw.SchemaVersion != "qs-ai-suite-registration/v1" || raw.CommandId != id || len(raw.ReceiptJson) > 32*1024 || json.Unmarshal([]byte(raw.ReceiptJson), &v) != nil {
		return v, app.ErrConflict
	}
	_, err := time.Parse(time.RFC3339Nano, v.RegisteredAt)
	if v.Scope != scope || scope.OrganizationID <= 0 || scope.OperatorUserID <= 0 || v.Command.CommandID != id || !v.Command.Valid() || err != nil {
		return app.SuiteRegistrationReceipt{}, app.ErrConflict
	}
	for _, ref := range []app.PromptDraftSource{v.Manifest.Profile, v.Manifest.Prompt, v.Manifest.GenerationRoute, v.Manifest.InputSchema, v.Manifest.OutputSchema} {
		if !ref.Valid() {
			return app.SuiteRegistrationReceipt{}, app.ErrConflict
		}
	}
	if !v.Suite.Valid() || v.Suite.ID != v.Command.SuiteID || v.Suite.Version != v.Command.SuiteVersion || v.Manifest.Profile != v.Command.Profile || v.Manifest.Prompt != v.Command.Prompt || v.Manifest.GenerationRoute != v.Command.GenerationRoute {
		return app.SuiteRegistrationReceipt{}, app.ErrConflict
	}
	return v, nil
}
func (c *SuiteClient) RegisterSuite(ctx context.Context, s app.DraftScope, command app.RegisterSuite) (app.SuiteRegistrationReceipt, error) {
	if !command.Valid() {
		return app.SuiteRegistrationReceipt{}, app.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	raw, err := c.RPC.Register(ctx, &pb.SuiteRegisterCommand{Scope: draftScope(s), CommandId: command.CommandID, Source: frozenRef(command.Source), SuiteId: command.SuiteID, SuiteVersion: command.SuiteVersion, Profile: profileReference(command.Profile), Prompt: profileReference(command.Prompt), GenerationRoute: profileReference(command.GenerationRoute), Reason: command.Reason}, grpc.MaxCallRecvMsgSize(32*1024))
	if err != nil {
		return app.SuiteRegistrationReceipt{}, err
	}
	receipt, err := suiteReceipt(raw, s, command.CommandID)
	if err != nil || receipt.Command != command {
		return app.SuiteRegistrationReceipt{}, app.ErrConflict
	}
	return receipt, nil
}
func (c *SuiteClient) GetSuiteReceipt(ctx context.Context, s app.DraftScope, id string) (app.SuiteRegistrationReceipt, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	raw, err := c.RPC.GetReceipt(ctx, &pb.SuiteRegistrationQuery{Scope: draftScope(s), CommandId: id}, grpc.MaxCallRecvMsgSize(32*1024))
	if err != nil {
		return app.SuiteRegistrationReceipt{}, err
	}
	return suiteReceipt(raw, s, id)
}
