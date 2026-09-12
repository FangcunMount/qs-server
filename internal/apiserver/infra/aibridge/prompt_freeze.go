package aibridge

import (
	"context"
	"encoding/json"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
)

func freezeReceipt(r *pb.PromptDraftFreezeReceipt, s app.DraftScope, id string) (app.FrozenPromptReceipt, error) {
	var v app.FrozenPromptReceipt
	if r == nil || r.SchemaVersion != "qs-ai-prompt-freeze/v1" || r.CommandId != id || len(r.ReceiptJson) > 32*1024 || json.Unmarshal([]byte(r.ReceiptJson), &v) != nil {
		return v, app.ErrConflict
	}
	command := app.FreezePromptDraft{PromptDraftCommand: app.PromptDraftCommand{CommandID: v.Command.CommandID, Reason: v.Command.Reason}, ExpectedRevision: v.Command.ExpectedRevision}
	_, err := time.Parse(time.RFC3339Nano, v.FrozenAt)
	if v.Scope != s || v.Scope.OrganizationID <= 0 || v.Scope.OperatorUserID <= 0 || v.Command.CommandID != id || !app.ValidPublicationID(v.Command.DraftID) || !command.Valid() || !v.Asset.Valid() || !app.ValidPromptChecksum(v.SnapshotSHA256) || v.ValidatorVersion != "qs-ai-prompt-syntax/v1" || err != nil {
		return app.FrozenPromptReceipt{}, app.ErrConflict
	}
	return v, nil
}
func (c *PromptDraftClient) FreezePromptDraft(ctx context.Context, s app.DraftScope, id string, cmd app.FreezePromptDraft) (app.FrozenPromptReceipt, error) {
	if !app.ValidPublicationID(id) || !cmd.Valid() {
		return app.FrozenPromptReceipt{}, app.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	r, err := c.RPC.Freeze(ctx, &pb.PromptDraftFreezeCommand{Scope: draftScope(s), DraftId: id, CommandId: cmd.CommandID, ExpectedRevision: cmd.ExpectedRevision, Reason: cmd.Reason}, grpc.MaxCallRecvMsgSize(32*1024))
	if err != nil {
		return app.FrozenPromptReceipt{}, err
	}
	v, err := freezeReceipt(r, s, cmd.CommandID)
	if err != nil || v.Command.DraftID != id || v.Command.ExpectedRevision != cmd.ExpectedRevision || v.Command.Reason != cmd.Reason {
		return app.FrozenPromptReceipt{}, app.ErrConflict
	}
	return v, nil
}
func (c *PromptDraftClient) GetPromptFreezeReceipt(ctx context.Context, s app.DraftScope, id string) (app.FrozenPromptReceipt, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	r, err := c.RPC.GetFreezeReceipt(ctx, &pb.PromptDraftReceiptQuery{Scope: draftScope(s), CommandId: id}, grpc.MaxCallRecvMsgSize(32*1024))
	if err != nil {
		return app.FrozenPromptReceipt{}, err
	}
	return freezeReceipt(r, s, id)
}
