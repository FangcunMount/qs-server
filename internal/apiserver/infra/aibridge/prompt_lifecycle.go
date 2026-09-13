package aibridge

import (
	"context"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

func (c *PromptDraftClient) GetPromptDraftLifecycle(ctx context.Context, scope app.DraftScope, id string) (app.PromptDraftLifecycle, error) {
	if !app.ValidPublicationID(id) || scope.OrganizationID <= 0 || scope.OperatorUserID <= 0 {
		return app.PromptDraftLifecycle{}, app.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	r, err := c.RPC.GetLifecycle(ctx, &pb.PromptDraftQuery{Scope: draftScope(scope), DraftId: id}, grpc.MaxCallRecvMsgSize(260*1024))
	if err != nil {
		return app.PromptDraftLifecycle{}, err
	}
	if r == nil || r.SchemaVersion != "qs-ai-prompt-lifecycle/v1" || proto.Size(r) > 260*1024 {
		return app.PromptDraftLifecycle{}, app.ErrConflict
	}
	draft, err := draftState(r.Draft, scope)
	if err != nil || draft.DraftID != id {
		return app.PromptDraftLifecycle{}, app.ErrConflict
	}
	value := app.PromptDraftLifecycle{SchemaVersion: r.SchemaVersion, Draft: draft, Status: r.Status}
	switch r.Status {
	case "editable":
		if r.Frozen != nil {
			return app.PromptDraftLifecycle{}, app.ErrConflict
		}
	case "frozen":
		f := r.Frozen
		if f == nil || f.Asset == nil || f.Revision != draft.Revision {
			return app.PromptDraftLifecycle{}, app.ErrConflict
		}
		ref := app.PromptDraftSource{Identity: f.Asset.Identity, Version: f.Asset.Version, Fingerprint: f.Asset.Fingerprint, ContentSHA256: f.Asset.ContentSha256}
		at, timeErr := time.Parse(time.RFC3339Nano, f.FrozenAt)
		savedAt, _ := time.Parse(time.RFC3339Nano, draft.SavedAt)
		if !ref.Valid() || ref.Identity != draft.TemplateID || ref.Version != draft.TargetVersion || timeErr != nil || at.Before(savedAt) {
			return app.PromptDraftLifecycle{}, app.ErrConflict
		}
		value.Frozen = &app.PromptFrozenVersion{Asset: ref, Revision: f.Revision, FrozenAt: f.FrozenAt}
	default:
		return app.PromptDraftLifecycle{}, app.ErrConflict
	}
	return value, nil
}
