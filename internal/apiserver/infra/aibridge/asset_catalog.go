package aibridge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
)

type AssetCatalogClient struct{ RPC pb.AssetCatalogClient }

func NewAssetCatalogClient(conn grpc.ClientConnInterface) *AssetCatalogClient {
	return &AssetCatalogClient{RPC: pb.NewAssetCatalogClient(conn)}
}
func catalogBefore(id, version, nextID, nextVersion string) bool {
	return id < nextID || (id == nextID && version < nextVersion)
}
func catalogPage(raw *pb.AssetCatalogResponse, query app.AssetCatalogQuery) (app.AssetCatalogPage, error) {
	var page app.AssetCatalogPage
	if raw == nil || raw.SchemaVersion != "qs-ai-asset-page/v1" || len(raw.PayloadJson) > 128*1024 || json.Unmarshal([]byte(raw.PayloadJson), &page) != nil || page.Items == nil || len(page.Items) > query.Limit {
		return page, app.ErrConflict
	}
	id, version, _ := query.After()
	for _, item := range page.Items {
		ref := item.Reference
		if item.Kind != query.Kind || !ref.Valid() || (query.Identity != "" && ref.Identity != query.Identity) || !catalogBefore(id, version, ref.Identity, ref.Version) {
			return app.AssetCatalogPage{}, app.ErrConflict
		}
		id, version = ref.Identity, ref.Version
	}
	if page.NextCursor != "" {
		next := query
		next.Cursor = page.NextCursor
		nextID, nextVersion, valid := next.After()
		if !valid || len(page.Items) != query.Limit || id != nextID || version != nextVersion {
			return app.AssetCatalogPage{}, app.ErrConflict
		}
	}
	return page, nil
}
func catalogDetail(raw *pb.AssetCatalogResponse, query app.AssetCatalogGet) (app.AssetCatalogDetail, error) {
	var value app.AssetCatalogDetail
	if raw == nil || raw.SchemaVersion != "qs-ai-asset-detail/v1" || len(raw.PayloadJson) > 4*1024*1024 || json.Unmarshal([]byte(raw.PayloadJson), &value) != nil {
		return value, app.ErrConflict
	}
	ref := value.Item.Reference
	digest := sha256.Sum256([]byte(value.DefinitionJSON))
	checksum := hex.EncodeToString(digest[:])
	if value.Item.Kind != query.Kind || ref.Identity != query.Identity || ref.Version != query.Version || !ref.Valid() || !json.Valid([]byte(value.DefinitionJSON)) || ref.ContentSHA256 != checksum {
		return app.AssetCatalogDetail{}, app.ErrConflict
	}
	// Imported Prompt fingerprint names its source; package checksum names these bytes.
	if query.Kind != "prompt" && ref.Fingerprint != "sha256:"+checksum {
		return app.AssetCatalogDetail{}, app.ErrConflict
	}
	return value, nil
}
func (c *AssetCatalogClient) ListAssets(ctx context.Context, scope app.DraftScope, query app.AssetCatalogQuery) (app.AssetCatalogPage, error) {
	if !query.Valid() {
		return app.AssetCatalogPage{}, app.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	raw, err := c.RPC.List(ctx, &pb.AssetCatalogQuery{Scope: draftScope(scope), Kind: query.Kind, Identity: query.Identity, Limit: int32(query.Limit), Cursor: query.Cursor}, grpc.MaxCallRecvMsgSize(128*1024+1024))
	if err != nil {
		return app.AssetCatalogPage{}, err
	}
	return catalogPage(raw, query)
}
func (c *AssetCatalogClient) GetAsset(ctx context.Context, scope app.DraftScope, query app.AssetCatalogGet) (app.AssetCatalogDetail, error) {
	if !query.Valid() {
		return app.AssetCatalogDetail{}, app.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	raw, err := c.RPC.Get(ctx, &pb.AssetCatalogGetQuery{Scope: draftScope(scope), Kind: query.Kind, Identity: query.Identity, Version: query.Version}, grpc.MaxCallRecvMsgSize(4*1024*1024+1024))
	if err != nil {
		return app.AssetCatalogDetail{}, err
	}
	return catalogDetail(raw, query)
}
