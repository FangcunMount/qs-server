package aibridge

import (
	"context"
	"encoding/json"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"time"
)

type QuotaClient struct{ RPC pb.QuotaManagementClient }

func NewQuotaClient(conn grpc.ClientConnInterface) *QuotaClient {
	return &QuotaClient{RPC: pb.NewQuotaManagementClient(conn)}
}
func quotaPayload(raw *pb.QuotaResponse, history bool) (json.RawMessage, error) {
	schema := "qs-ai-quota/v1"
	if history {
		schema = "qs-ai-quota-history/v1"
	}
	if raw == nil || raw.SchemaVersion != schema || len(raw.DataJson) > 131072 || !json.Valid([]byte(raw.DataJson)) {
		return nil, status.Error(codes.Unavailable, "Quota result requires reconciliation")
	}
	var value map[string]json.RawMessage
	if json.Unmarshal([]byte(raw.DataJson), &value) != nil || value == nil {
		return nil, status.Error(codes.Unavailable, "Invalid quota response")
	}
	if !history {
		var revision int64
		if len(value["revision"]) == 0 || json.Unmarshal(value["revision"], &revision) != nil || revision < 0 || len(value["effective"]) == 0 {
			return nil, status.Error(codes.Unavailable, "Invalid quota response")
		}
	}
	return json.RawMessage(raw.DataJson), nil
}
func (c *QuotaClient) ReadQuota(ctx context.Context, scope app.DraftScope, op, id string, before int64) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	q := &pb.QuotaQuery{Scope: draftScope(scope), CommandId: id, BeforeRevision: before}
	var raw *pb.QuotaResponse
	var err error
	switch op {
	case "status":
		raw, err = c.RPC.Status(ctx, q)
	case "get":
		raw, err = c.RPC.Get(ctx, q)
	case "history":
		raw, err = c.RPC.History(ctx, q)
	case "receipt":
		raw, err = c.RPC.GetReceipt(ctx, q)
	default:
		return nil, app.ErrInvalid
	}
	if err != nil {
		return nil, err
	}
	if op == "status" {
		var value struct {
			OrganizationID int64             `json:"organization_id"`
			Categories     []json.RawMessage `json:"categories"`
		}
		if raw == nil || raw.SchemaVersion != "qs-ai-configuration-status/v1" || len(raw.DataJson) > 131072 || json.Unmarshal([]byte(raw.DataJson), &value) != nil || value.OrganizationID != scope.OrganizationID || len(value.Categories) == 0 {
			return nil, app.ErrConflict
		}
		return json.RawMessage(raw.DataJson), nil
	}
	return quotaPayload(raw, op == "history")
}
func (c *QuotaClient) WriteQuota(ctx context.Context, scope app.DraftScope, op string, body json.RawMessage) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	q := &pb.QuotaWrite{Scope: draftScope(scope), CommandJson: string(body)}
	var raw *pb.QuotaResponse
	var err error
	switch op {
	case "update":
		raw, err = c.RPC.Update(ctx, q)
	case "rollback":
		raw, err = c.RPC.Rollback(ctx, q)
	default:
		return nil, app.ErrInvalid
	}
	if err != nil {
		return nil, err
	}
	return quotaPayload(raw, false)
}
