package aibridge

import (
	"context"
	"encoding/json"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
	"time"
)

func (c *AssetCatalogClient) PolicyReferences(ctx context.Context, scope app.DraftScope, q app.PolicyReferencesQuery) (json.RawMessage, error) {
	if !q.Valid() {
		return nil, app.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	raw, err := c.RPC.References(ctx, &pb.PolicyReferencesQuery{Scope: draftScope(scope), Kind: q.Kind, Reference: frozenRef(q.Reference), UsageKind: q.UsageKind, Limit: int32(q.Limit), Cursor: q.Cursor}, grpc.MaxCallRecvMsgSize(128*1024))
	if err != nil {
		return nil, err
	}
	var value struct {
		Kind       string                  `json:"kind"`
		Reference  app.FrozenEvaluationRef `json:"reference"`
		UsageKind  string                  `json:"usage_kind"`
		Items      []json.RawMessage       `json:"items"`
		NextCursor string                  `json:"next_cursor"`
	}
	if raw == nil || raw.SchemaVersion != "qs-ai-policy-references/v1" || len(raw.PayloadJson) > 128*1024 || json.Unmarshal([]byte(raw.PayloadJson), &value) != nil || value.Kind != q.Kind || value.Reference != q.Reference || value.UsageKind != q.UsageKind || value.Items == nil || len(value.Items) > q.Limit || len(value.NextCursor) > 4096 {
		return nil, app.ErrConflict
	}
	return json.RawMessage(raw.PayloadJson), nil
}
