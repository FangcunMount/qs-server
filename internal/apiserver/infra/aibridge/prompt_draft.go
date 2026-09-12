package aibridge

import (
	"context"
	"encoding/json"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
)

type PromptDraftClient struct {
	RPC pb.PromptDraftManagementClient
}

func NewPromptDraftClient(conn grpc.ClientConnInterface) *PromptDraftClient {
	return &PromptDraftClient{RPC: pb.NewPromptDraftManagementClient(conn)}
}
func draftScope(s app.DraftScope) *pb.PublicationScope {
	return &pb.PublicationScope{OrganizationId: s.OrganizationID, OperatorUserId: s.OperatorUserID}
}
func draftState(r *pb.PromptDraftState, s app.DraftScope) (app.PromptDraftState, error) {
	var v app.PromptDraftState
	if r == nil || r.SchemaVersion != "qs-ai-prompt-draft/v1" || len(r.SnapshotJson) > 256*1024 || json.Unmarshal([]byte(r.SnapshotJson), &v) != nil {
		return v, app.ErrConflict
	}
	command := app.PromptDraftCommand{CommandID: v.CommandID, Reason: v.Reason}
	create := app.CreatePromptDraft{PromptDraftCommand: command, Source: v.Source, TemplateID: v.TemplateID, TargetVersion: v.TargetVersion}
	_, err := time.Parse(time.RFC3339Nano, v.SavedAt)
	if !app.ValidPublicationID(v.DraftID) || v.DraftID != r.DraftId || v.Revision < 1 || v.Revision != r.Revision || v.OrganizationID != s.OrganizationID || v.OperatorUserID <= 0 || !create.Valid() || !v.Content.Valid() || err != nil {
		return app.PromptDraftState{}, app.ErrConflict
	}
	return v, nil
}
func (c *PromptDraftClient) CreatePromptDraft(ctx context.Context, s app.DraftScope, id string, cmd app.CreatePromptDraft) (app.PromptDraftState, error) {
	if !cmd.Valid() || !app.ValidPublicationID(id) {
		return app.PromptDraftState{}, app.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ref := cmd.Source
	r, err := c.RPC.Create(ctx, &pb.PromptDraftCreateCommand{Scope: draftScope(s), DraftId: id, CommandId: cmd.CommandID, Reason: cmd.Reason, TemplateId: cmd.TemplateID, TargetVersion: cmd.TargetVersion, Source: &pb.PromptDraftSource{Identity: ref.Identity, Version: ref.Version, Fingerprint: ref.Fingerprint, ContentSha256: ref.ContentSHA256}}, grpc.MaxCallRecvMsgSize(256*1024))
	if err != nil {
		return app.PromptDraftState{}, err
	}
	v, err := draftState(r, s)
	if err != nil || v.DraftID != id || v.CommandID != cmd.CommandID || v.OperatorUserID != s.OperatorUserID || v.Reason != cmd.Reason || v.Revision != 1 || v.Source != cmd.Source || v.TemplateID != cmd.TemplateID || v.TargetVersion != cmd.TargetVersion {
		return app.PromptDraftState{}, app.ErrConflict
	}
	return v, nil
}
func (c *PromptDraftClient) RevisePromptDraft(ctx context.Context, s app.DraftScope, id string, cmd app.RevisePromptDraft) (app.PromptDraftState, error) {
	if !cmd.Valid() || !app.ValidPublicationID(id) {
		return app.PromptDraftState{}, app.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	content := cmd.Content
	r, err := c.RPC.Revise(ctx, &pb.PromptDraftReviseCommand{Scope: draftScope(s), DraftId: id, CommandId: cmd.CommandID, Reason: cmd.Reason, ExpectedRevision: cmd.ExpectedRevision, Content: &pb.PromptDraftContent{SystemMessage: content.SystemMessage, TaskTemplate: content.TaskTemplate, DataPreamble: content.DataPreamble, AllowedPlaceholders: content.AllowedPlaceholders}}, grpc.MaxCallRecvMsgSize(256*1024))
	if err != nil {
		return app.PromptDraftState{}, err
	}
	v, err := draftState(r, s)
	if err != nil || v.DraftID != id || v.CommandID != cmd.CommandID || v.OperatorUserID != s.OperatorUserID || v.Reason != cmd.Reason || v.Revision != cmd.ExpectedRevision+1 || !v.Content.Equal(*cmd.Content) {
		return app.PromptDraftState{}, app.ErrConflict
	}
	return v, nil
}
func (c *PromptDraftClient) GetPromptDraft(ctx context.Context, s app.DraftScope, id string, rev *int64) (app.PromptDraftState, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	r, err := c.RPC.Get(ctx, &pb.PromptDraftQuery{Scope: draftScope(s), DraftId: id, Revision: rev}, grpc.MaxCallRecvMsgSize(256*1024))
	if err != nil {
		return app.PromptDraftState{}, err
	}
	v, err := draftState(r, s)
	if err != nil || v.DraftID != id || rev != nil && v.Revision != *rev {
		return app.PromptDraftState{}, app.ErrConflict
	}
	return v, nil
}
func (c *PromptDraftClient) GetPromptDraftReceipt(ctx context.Context, s app.DraftScope, id string) (app.PromptDraftState, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	r, err := c.RPC.GetReceipt(ctx, &pb.PromptDraftReceiptQuery{Scope: draftScope(s), CommandId: id}, grpc.MaxCallRecvMsgSize(256*1024))
	if err != nil {
		return app.PromptDraftState{}, err
	}
	v, err := draftState(r, s)
	if err != nil || v.CommandID != id || v.OperatorUserID != s.OperatorUserID {
		return app.PromptDraftState{}, app.ErrConflict
	}
	return v, nil
}
