package aibridge

import (
	"context"
	"encoding/json"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type SemanticDraftClient struct{ RPC pb.SemanticPromptDraftsClient }

func NewSemanticDraftClient(conn grpc.ClientConnInterface) *SemanticDraftClient {
	return &SemanticDraftClient{RPC: pb.NewSemanticPromptDraftsClient(conn)}
}
func semanticDraftPayload(raw *pb.SemanticDraftResponse, scope app.DraftScope) (json.RawMessage, error) {
	invalid := status.Error(codes.Unavailable, "Semantic draft result requires reconciliation")
	if raw == nil || raw.SchemaVersion != "qs-ai-semantic-draft/v1" || len(raw.DataJson) > 262144 || !json.Valid([]byte(raw.DataJson)) {
		return nil, invalid
	}
	var value struct {
		Draft struct {
			DraftID        string `json:"draft_id"`
			OrganizationID int64  `json:"organization_id"`
			Revision       int64  `json:"revision"`
			State          string `json:"state"`
		} `json:"draft"`
	}
	if json.Unmarshal([]byte(raw.DataJson), &value) != nil || value.Draft.OrganizationID != scope.OrganizationID || !app.ValidPublicationID(value.Draft.DraftID) || value.Draft.Revision <= 0 || (value.Draft.State != "editing" && value.Draft.State != "frozen") {
		return nil, invalid
	}
	return json.RawMessage(raw.DataJson), nil
}
func (c *SemanticDraftClient) ReadSemanticDraft(ctx context.Context, scope app.DraftScope, operation, id string, revision int64) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	request := &pb.SemanticDraftQuery{Scope: draftScope(scope), DraftId: id, Revision: revision}
	var raw *pb.SemanticDraftResponse
	var err error
	switch operation {
	case "get":
		raw, err = c.RPC.Get(ctx, request)
	case "validate":
		raw, err = c.RPC.Validate(ctx, request)
	case "receipt":
		request.DraftId = ""
		request.CommandId = id
		raw, err = c.RPC.GetReceipt(ctx, request)
	default:
		return nil, app.ErrInvalid
	}
	if err != nil {
		return nil, err
	}
	return semanticDraftPayload(raw, scope)
}
func (c *SemanticDraftClient) WriteSemanticDraft(ctx context.Context, scope app.DraftScope, operation string, body json.RawMessage) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	request := &pb.SemanticDraftWrite{Scope: draftScope(scope), CommandJson: string(body)}
	var raw *pb.SemanticDraftResponse
	var err error
	switch operation {
	case "create":
		raw, err = c.RPC.Create(ctx, request)
	case "revise":
		raw, err = c.RPC.Revise(ctx, request)
	case "freeze":
		raw, err = c.RPC.Freeze(ctx, request)
	default:
		return nil, app.ErrInvalid
	}
	if err != nil {
		return nil, err
	}
	return semanticDraftPayload(raw, scope)
}
