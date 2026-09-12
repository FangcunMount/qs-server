package aibridge

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"unicode/utf8"

	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
)

type AssetCatalogQuery struct {
	Kind     string `json:"kind"`
	Identity string `json:"identity"`
	Limit    int    `json:"limit"`
	Cursor   string `json:"cursor"`
}
type AssetCatalogGet struct {
	Kind     string `json:"kind"`
	Identity string `json:"identity"`
	Version  string `json:"version"`
}
type AssetCatalogItem struct {
	Kind      string            `json:"kind"`
	Reference PromptDraftSource `json:"reference"`
}
type AssetCatalogPage struct {
	Items      []AssetCatalogItem `json:"items"`
	NextCursor string             `json:"next_cursor"`
}
type AssetCatalogDetail struct {
	Item           AssetCatalogItem `json:"item"`
	DefinitionJSON string           `json:"definition_json"`
}

func ValidAssetKind(kind string) bool {
	switch kind {
	case "profile", "prompt", "route", "schema", "suite":
		return true
	}
	return false
}
func validCatalogIdentity(identity string) bool {
	return identity != "" && strings.TrimSpace(identity) == identity && utf8.ValidString(identity) && utf8.RuneCountInString(identity) <= 255
}
func (q AssetCatalogGet) Valid() bool {
	return ValidAssetKind(q.Kind) && validCatalogIdentity(q.Identity) && frozenVersion.MatchString(q.Version)
}

// After validates a query-bound continuation position. The cursor is not authorization.
func (q AssetCatalogQuery) After() (string, string, bool) {
	if len(q.Cursor) > 4096 {
		return "", "", false
	}
	if q.Cursor == "" {
		return "", "", true
	}
	raw, err := base64.URLEncoding.Strict().DecodeString(q.Cursor)
	if err != nil {
		return "", "", false
	}
	var parts []json.RawMessage
	if json.Unmarshal(raw, &parts) != nil || len(parts) != 5 {
		return "", "", false
	}
	var format int
	var kind, filter, identity, version string
	if json.Unmarshal(parts[0], &format) != nil || format != 1 || json.Unmarshal(parts[1], &kind) != nil || json.Unmarshal(parts[2], &filter) != nil || json.Unmarshal(parts[3], &identity) != nil || json.Unmarshal(parts[4], &version) != nil {
		return "", "", false
	}
	return identity, version, kind == q.Kind && filter == q.Identity && (filter == "" || identity == filter) && validCatalogIdentity(identity) && frozenVersion.MatchString(version)
}
func (q AssetCatalogQuery) Valid() bool {
	_, _, cursorOK := q.After()
	return ValidAssetKind(q.Kind) && (q.Identity == "" || validCatalogIdentity(q.Identity)) && q.Limit >= 1 && q.Limit <= 50 && cursorOK
}

type AssetCatalogGateway interface {
	ListAssets(context.Context, DraftScope, AssetCatalogQuery) (AssetCatalogPage, error)
	GetAsset(context.Context, DraftScope, AssetCatalogGet) (AssetCatalogDetail, error)
}
type AssetCatalogAdministration struct{ Gateway AssetCatalogGateway }

func (s *AssetCatalogAdministration) authorize(ctx context.Context, scope DraftScope) error {
	if scope.OrganizationID <= 0 || scope.OperatorUserID <= 0 {
		return ErrInvalid
	}
	snapshot, ok := authz.FromContext(ctx)
	if !ok || !authz.DecideCapability(snapshot, authz.CapabilityAuditInterpretation).Allowed {
		return ErrGovernanceDenied
	}
	if s == nil || s.Gateway == nil {
		return ErrManagementUnavailable
	}
	return nil
}
func (s *AssetCatalogAdministration) List(ctx context.Context, scope DraftScope, query AssetCatalogQuery) (AssetCatalogPage, error) {
	if err := s.authorize(ctx, scope); err != nil {
		return AssetCatalogPage{}, err
	}
	if query.Limit == 0 {
		query.Limit = 20
	}
	if !query.Valid() {
		return AssetCatalogPage{}, ErrInvalid
	}
	return s.Gateway.ListAssets(ctx, scope, query)
}
func (s *AssetCatalogAdministration) Get(ctx context.Context, scope DraftScope, query AssetCatalogGet) (AssetCatalogDetail, error) {
	if err := s.authorize(ctx, scope); err != nil {
		return AssetCatalogDetail{}, err
	}
	if !query.Valid() {
		return AssetCatalogDetail{}, ErrInvalid
	}
	return s.Gateway.GetAsset(ctx, scope, query)
}
