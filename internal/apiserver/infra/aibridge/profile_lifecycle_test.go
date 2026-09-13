package aibridge

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
	"strings"
	"testing"
)

type profileLifecycleRPCStub struct {
	*profileRPCStub
	raw   *pb.AssetCatalogResponse
	query *pb.ProfileLifecycleQuery
}

func (s *profileLifecycleRPCStub) ListLifecycle(ctx context.Context, q *pb.ProfileLifecycleQuery, _ ...grpc.CallOption) (*pb.AssetCatalogResponse, error) {
	s.mark(ctx, q.Scope)
	s.query = q
	return s.raw, s.err
}
func (s *profileLifecycleRPCStub) GetLifecycle(ctx context.Context, q *pb.ProfileLifecycleQuery, _ ...grpc.CallOption) (*pb.AssetCatalogResponse, error) {
	s.mark(ctx, q.Scope)
	s.query = q
	return s.raw, s.err
}
func TestProfileLifecycleContractBindsFilterCursorReferenceAndScope(t *testing.T) {
	scope := app.DraftScope{OrganizationID: 7, OperatorUserID: 42}
	_, receipt := profileFixture()
	base := app.ProfileLifecycle{Reference: receipt.Manifest.Profile, Status: "draft", SourceRef: "qs-server:baseline", ImportedAt: receipt.RegisteredAt}
	query := app.ProfileLifecycleQuery{Identity: base.Reference.Identity, Status: "draft", Limit: 1}
	for _, damage := range []string{"", "status", "identity", "source", "time", "pointer", "cursor-filter", "cursor-position", "nil-items", "duplicate", "schema"} {
		t.Run(damage, func(t *testing.T) {
			item := base
			rawCursor, _ := json.Marshal([]any{1, query.Identity, query.Status, item.Reference.Identity, item.Reference.Version})
			page := app.ProfileLifecyclePage{Items: []app.ProfileLifecycle{item}, NextCursor: base64.URLEncoding.EncodeToString(rawCursor)}
			switch damage {
			case "status":
				page.Items[0].Status = "published"
			case "identity":
				page.Items[0].Reference.Identity = "wrong"
			case "source":
				page.Items[0].SourceRef = ""
			case "time":
				page.Items[0].ImportedAt = "bad"
			case "pointer":
				page.Items[0].ActivePublicationID = publicationTestID
			case "cursor-filter":
				rawCursor, _ = json.Marshal([]any{1, query.Identity, "published", item.Reference.Identity, item.Reference.Version})
				page.NextCursor = base64.URLEncoding.EncodeToString(rawCursor)
			case "cursor-position":
				rawCursor, _ = json.Marshal([]any{1, query.Identity, query.Status, item.Reference.Identity, "v3"})
				page.NextCursor = base64.URLEncoding.EncodeToString(rawCursor)
			case "nil-items":
				page.Items = nil
			case "duplicate":
				page.Items = append(page.Items, item)
			}
			raw, _ := json.Marshal(page)
			rpc := &profileLifecycleRPCStub{profileRPCStub: &profileRPCStub{}, raw: &pb.AssetCatalogResponse{SchemaVersion: "qs-ai-profile-lifecycle-page/v1", PayloadJson: string(raw)}}
			if damage == "schema" {
				rpc.raw.SchemaVersion = "wrong"
			}
			result, err := (&ProfileClient{RPC: rpc}).ListProfileLifecycles(context.Background(), scope, query)
			if damage == "" {
				if err != nil || len(result.Items) != 1 {
					t.Fatal(err, result)
				}
			} else if !errors.Is(err, app.ErrConflict) {
				t.Fatal("accepted invalid response", err)
			}
			if rpc.scope.OrganizationId != 7 || rpc.scope.OperatorUserId != 42 || !rpc.deadline || rpc.calls != 1 || rpc.query.Status != "draft" {
				t.Fatal("lost protected scope/query/deadline")
			}
		})
	}
	raw, _ := json.Marshal(base)
	rpc := &profileLifecycleRPCStub{profileRPCStub: &profileRPCStub{}, raw: &pb.AssetCatalogResponse{SchemaVersion: "qs-ai-profile-lifecycle/v1", PayloadJson: string(raw)}}
	c := &ProfileClient{RPC: rpc}
	if _, err := c.GetProfileLifecycle(context.Background(), scope, base.Reference.Identity, base.Reference.Version); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetProfileLifecycle(context.Background(), scope, base.Reference.Identity, "v3"); !errors.Is(err, app.ErrConflict) {
		t.Fatal("accepted another version", err)
	}
	rpc.raw.PayloadJson = strings.Repeat("x", 128*1024+1)
	if _, err := c.GetProfileLifecycle(context.Background(), scope, base.Reference.Identity, base.Reference.Version); !errors.Is(err, app.ErrConflict) {
		t.Fatal(err)
	}
}
