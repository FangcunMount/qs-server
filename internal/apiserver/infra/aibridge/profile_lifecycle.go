package aibridge

import (
	"context"
	"encoding/json"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
	"time"
	"unicode/utf8"
)

// Validate the read contract; publication decisions remain owned by qs-ai.
func validProfileLifecycle(v app.ProfileLifecycle) bool {
	if !v.Reference.Valid() || v.SourceRef == "" || len(v.SourceRef) > 4096 || !utf8.ValidString(v.SourceRef) || v.SelectorVersion < 0 {
		return false
	}
	if _, err := time.Parse(time.RFC3339Nano, v.ImportedAt); err != nil {
		return false
	}
	if v.SelectorChangedAt != "" {
		if _, err := time.Parse(time.RFC3339Nano, v.SelectorChangedAt); err != nil {
			return false
		}
	}
	switch v.Status {
	case "draft":
		return v.ActivePublicationID == "" && v.ActiveRunID == "" && v.InactiveReason == ""
	case "published":
		return app.ValidPublicationID(v.ActivePublicationID) && app.ValidPublicationID(v.ActiveRunID) && v.SelectorVersion > 0 && v.SelectorChangedAt != "" && v.InactiveReason == ""
	case "disabled":
		return v.ActivePublicationID == "" && v.ActiveRunID == "" && v.SelectorVersion > 0 && v.SelectorChangedAt != "" && (v.InactiveReason == "replaced" || v.InactiveReason == "disabled")
	}
	return false
}
func profileLifecyclePage(raw *pb.AssetCatalogResponse, query app.ProfileLifecycleQuery) (app.ProfileLifecyclePage, error) {
	var page app.ProfileLifecyclePage
	if raw == nil || raw.SchemaVersion != "qs-ai-profile-lifecycle-page/v1" || len(raw.PayloadJson) > 128*1024 || json.Unmarshal([]byte(raw.PayloadJson), &page) != nil || page.Items == nil || len(page.Items) > query.Limit {
		return app.ProfileLifecyclePage{}, app.ErrConflict
	}
	id, version, _ := query.After()
	for _, item := range page.Items {
		ref := item.Reference
		if !validProfileLifecycle(item) || (query.Identity != "" && query.Identity != ref.Identity) || (query.Status != "" && query.Status != item.Status) || !catalogBefore(id, version, ref.Identity, ref.Version) {
			return app.ProfileLifecyclePage{}, app.ErrConflict
		}
		id, version = ref.Identity, ref.Version
	}
	if page.NextCursor != "" {
		next := query
		next.Cursor = page.NextCursor
		nextID, nextVersion, ok := next.After()
		if !ok || len(page.Items) != query.Limit || nextID != id || nextVersion != version {
			return app.ProfileLifecyclePage{}, app.ErrConflict
		}
	}
	return page, nil
}
func (c *ProfileClient) ListProfileLifecycles(ctx context.Context, scope app.DraftScope, query app.ProfileLifecycleQuery) (app.ProfileLifecyclePage, error) {
	if !query.Valid() {
		return app.ProfileLifecyclePage{}, app.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	raw, err := c.RPC.ListLifecycle(ctx, &pb.ProfileLifecycleQuery{Scope: draftScope(scope), Identity: query.Identity, Status: query.Status, Limit: int32(query.Limit), Cursor: query.Cursor}, grpc.MaxCallRecvMsgSize(128*1024+1024))
	if err != nil {
		return app.ProfileLifecyclePage{}, err
	}
	return profileLifecyclePage(raw, query)
}
func (c *ProfileClient) GetProfileLifecycle(ctx context.Context, scope app.DraftScope, identity, version string) (app.ProfileLifecycle, error) {
	if !(app.AssetCatalogGet{Kind: "profile", Identity: identity, Version: version}).Valid() {
		return app.ProfileLifecycle{}, app.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	raw, err := c.RPC.GetLifecycle(ctx, &pb.ProfileLifecycleQuery{Scope: draftScope(scope), Identity: identity, Version: version}, grpc.MaxCallRecvMsgSize(128*1024+1024))
	if err != nil {
		return app.ProfileLifecycle{}, err
	}
	var value app.ProfileLifecycle
	if raw == nil || raw.SchemaVersion != "qs-ai-profile-lifecycle/v1" || len(raw.PayloadJson) > 128*1024 || json.Unmarshal([]byte(raw.PayloadJson), &value) != nil || !validProfileLifecycle(value) || value.Reference.Identity != identity || value.Reference.Version != version {
		return app.ProfileLifecycle{}, app.ErrConflict
	}
	return value, nil
}
