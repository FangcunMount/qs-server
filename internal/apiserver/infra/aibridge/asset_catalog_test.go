package aibridge

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type catalogRPCStub struct {
	pb.AssetCatalogClient
	response *pb.AssetCatalogResponse
	err      error
	calls    int
	scope    *pb.PublicationScope
	deadline bool
}

func (s *catalogRPCStub) mark(ctx context.Context, scope *pb.PublicationScope) {
	s.calls++
	s.scope = scope
	at, ok := ctx.Deadline()
	s.deadline = ok && time.Until(at) > 0 && time.Until(at) <= 5*time.Second
}
func (s *catalogRPCStub) List(ctx context.Context, query *pb.AssetCatalogQuery, _ ...grpc.CallOption) (*pb.AssetCatalogResponse, error) {
	s.mark(ctx, query.Scope)
	return s.response, s.err
}
func (s *catalogRPCStub) Get(ctx context.Context, query *pb.AssetCatalogGetQuery, _ ...grpc.CallOption) (*pb.AssetCatalogResponse, error) {
	s.mark(ctx, query.Scope)
	return s.response, s.err
}
func catalogFixture(kind string) app.AssetCatalogDetail {
	raw := `{"profile_id":"中文/配置","version":"v1","text":"<样例>&"}`
	digest := sha256.Sum256([]byte(raw))
	checksum := hex.EncodeToString(digest[:])
	return app.AssetCatalogDetail{Item: app.AssetCatalogItem{Kind: kind, Reference: app.PromptDraftSource{Identity: "中文/配置", Version: "v1", Fingerprint: "sha256:" + checksum, ContentSHA256: checksum}}, DefinitionJSON: raw}
}
func catalogCursor(kind, filter, id, version string) string {
	raw, _ := json.Marshal([]any{1, kind, filter, id, version})
	return base64.URLEncoding.EncodeToString(raw)
}
func TestCatalogPageChecksKindFilterOrderAndBoundContinuation(t *testing.T) {
	for _, damage := range []string{"", "kind", "identity", "fingerprint", "checksum", "duplicate", "order", "over-limit", "cursor-kind", "cursor-position", "cursor-short-page", "null", "schema", "oversize", "nil", "timeout"} {
		t.Run(damage, func(t *testing.T) {
			item := catalogFixture("profile").Item
			query := app.AssetCatalogQuery{Kind: "profile", Identity: item.Reference.Identity, Limit: 2}
			next := item
			next.Reference.Version = "v2"
			page := app.AssetCatalogPage{Items: []app.AssetCatalogItem{item, next}, NextCursor: catalogCursor(query.Kind, query.Identity, item.Reference.Identity, "v2")}
			switch damage {
			case "kind":
				page.Items[0].Kind = "prompt"
			case "identity":
				page.Items[0].Reference.Identity = "other"
			case "fingerprint":
				page.Items[0].Reference.Fingerprint = "bad"
			case "checksum":
				page.Items[0].Reference.ContentSHA256 = "bad"
			case "duplicate":
				page.Items[1] = item
			case "order":
				page.Items[0], page.Items[1] = page.Items[1], page.Items[0]
			case "over-limit":
				page.Items = append(page.Items, next)
			case "cursor-kind":
				page.NextCursor = catalogCursor("prompt", query.Identity, item.Reference.Identity, "v2")
			case "cursor-position":
				page.NextCursor = catalogCursor(query.Kind, query.Identity, item.Reference.Identity, "v3")
			case "cursor-short-page":
				page.Items = page.Items[:1]
			case "null":
				page.Items = nil
			}
			raw, _ := json.Marshal(page)
			response := &pb.AssetCatalogResponse{SchemaVersion: "qs-ai-asset-page/v1", PayloadJson: string(raw)}
			switch damage {
			case "schema":
				response.SchemaVersion = "unknown"
			case "oversize":
				response.PayloadJson = strings.Repeat(" ", 128*1024+1)
			case "nil":
				response = nil
			}
			stub := &catalogRPCStub{response: response}
			if damage == "timeout" {
				stub.err = status.Error(codes.DeadlineExceeded, "private")
			}
			scope := app.DraftScope{OrganizationID: 7, OperatorUserID: 42}
			_, err := (&AssetCatalogClient{RPC: stub}).ListAssets(context.Background(), scope, query)
			if damage == "" && err != nil {
				t.Fatal(err)
			}
			if damage != "" && damage != "timeout" && !errors.Is(err, app.ErrConflict) {
				t.Fatal(damage, err)
			}
			if damage == "timeout" && status.Code(err) != codes.DeadlineExceeded {
				t.Fatal(err)
			}
			if stub.calls != 1 || !stub.deadline || stub.scope.OrganizationId != 7 || stub.scope.OperatorUserId != 42 {
				t.Fatal(stub)
			}
		})
	}
	// Empty terminal pages and pages continuing strictly after the requested cursor are valid.
	q := app.AssetCatalogQuery{Kind: "profile", Limit: 1, Cursor: catalogCursor("profile", "", "中文/配置", "v1")}
	item := catalogFixture("profile").Item
	raw, _ := json.Marshal(app.AssetCatalogPage{Items: []app.AssetCatalogItem{item}})
	if _, err := catalogPage(&pb.AssetCatalogResponse{SchemaVersion: "qs-ai-asset-page/v1", PayloadJson: string(raw)}, q); !errors.Is(err, app.ErrConflict) {
		t.Fatal(err)
	}
	if _, err := catalogPage(&pb.AssetCatalogResponse{SchemaVersion: "qs-ai-asset-page/v1", PayloadJson: `{"items":[],"next_cursor":""}`}, q); err != nil {
		t.Fatal(err)
	}
}
func TestCatalogDetailBindsOriginalBytesAndPromptSourceFingerprint(t *testing.T) {
	for _, kind := range []string{"profile", "prompt", "route", "schema", "suite"} {
		for _, damage := range []string{"", "kind", "identity", "version", "content", "checksum", "fingerprint", "schema", "nil", "oversize"} {
			t.Run(kind+"/"+damage, func(t *testing.T) {
				value := catalogFixture(kind)
				query := app.AssetCatalogGet{Kind: kind, Identity: value.Item.Reference.Identity, Version: "v1"}
				if kind == "prompt" {
					value.Item.Reference.Fingerprint = "sha256:" + strings.Repeat("b", 64)
				}
				switch damage {
				case "kind":
					value.Item.Kind = "wrong"
				case "identity":
					value.Item.Reference.Identity = "wrong"
				case "version":
					value.Item.Reference.Version = "v2"
				case "content":
					value.DefinitionJSON += " "
				case "checksum":
					value.Item.Reference.ContentSHA256 = strings.Repeat("c", 64)
				case "fingerprint":
					value.Item.Reference.Fingerprint = "invalid"
				}
				raw, _ := json.Marshal(value)
				response := &pb.AssetCatalogResponse{SchemaVersion: "qs-ai-asset-detail/v1", PayloadJson: string(raw)}
				switch damage {
				case "schema":
					response.SchemaVersion = "unknown"
				case "nil":
					response = nil
				case "oversize":
					response.PayloadJson = strings.Repeat(" ", 4*1024*1024+1)
				}
				stub := &catalogRPCStub{response: response}
				_, err := (&AssetCatalogClient{RPC: stub}).GetAsset(context.Background(), app.DraftScope{OrganizationID: 7, OperatorUserID: 42}, query)
				if (damage == "") != (err == nil) {
					t.Fatal(damage, err)
				}
				if stub.calls != 1 || !stub.deadline {
					t.Fatal(stub)
				}
			})
		}
	}
}
func TestCatalogRejectsMalformedQueryBeforeRPCAndSupportsCharacterLimits(t *testing.T) {
	stub := &catalogRPCStub{}
	client := &AssetCatalogClient{RPC: stub}
	for _, q := range []app.AssetCatalogQuery{{Kind: "receipts", Limit: 1}, {Kind: "profile", Limit: 51}, {Kind: "profile", Limit: 1, Cursor: "bad"}, {Kind: "profile", Limit: 1, Identity: " x"}} {
		if _, err := client.ListAssets(context.Background(), app.DraftScope{}, q); !errors.Is(err, app.ErrInvalid) {
			t.Fatal(err)
		}
	}
	if _, err := client.GetAsset(context.Background(), app.DraftScope{}, app.AssetCatalogGet{Kind: "profile", Identity: "x", Version: " "}); !errors.Is(err, app.ErrInvalid) {
		t.Fatal(err)
	}
	if stub.calls != 0 {
		t.Fatal("invalid query made RPC")
	}
	ref := catalogFixture("profile").Item.Reference
	ref.Identity = strings.Repeat("中", 255)
	if !ref.Valid() || !(app.AssetCatalogGet{Kind: "profile", Identity: ref.Identity, Version: "v1"}).Valid() {
		t.Fatal("rejected valid 255-character identity")
	}
	ref.Identity += "文"
	if ref.Valid() {
		t.Fatal("accepted overlong identity")
	}
}
