package aibridge

import (
	"context"
	"encoding/base64"
	"encoding/json"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
)

type ProfileLifecycleQuery struct {
	Identity string `json:"identity"`
	Status   string `json:"status"`
	Limit    int    `json:"limit"`
	Cursor   string `json:"cursor"`
}
type ProfileLifecycle struct {
	Reference           PromptDraftSource `json:"reference"`
	Status              string            `json:"status"`
	SourceRef           string            `json:"source_ref"`
	ImportedAt          string            `json:"imported_at"`
	ActivePublicationID string            `json:"active_publication_id"`
	ActiveRunID         string            `json:"active_run_id"`
	SelectorVersion     int64             `json:"selector_version"`
	SelectorChangedAt   string            `json:"selector_changed_at"`
	InactiveReason      string            `json:"inactive_reason"`
}
type ProfileLifecyclePage struct {
	Items      []ProfileLifecycle `json:"items"`
	NextCursor string             `json:"next_cursor"`
}

// After binds pagination to the exact identity and lifecycle filter, not authority.
func (q ProfileLifecycleQuery) After() (string, string, bool) {
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
	var identityFilter, statusFilter, identity, version string
	if json.Unmarshal(parts[0], &format) != nil || format != 1 || json.Unmarshal(parts[1], &identityFilter) != nil || json.Unmarshal(parts[2], &statusFilter) != nil || json.Unmarshal(parts[3], &identity) != nil || json.Unmarshal(parts[4], &version) != nil {
		return "", "", false
	}
	return identity, version, identityFilter == q.Identity && statusFilter == q.Status && (q.Identity == "" || identity == q.Identity) && validCatalogIdentity(identity) && frozenVersion.MatchString(version)
}
func (q ProfileLifecycleQuery) Valid() bool {
	_, _, ok := q.After()
	return ok && (q.Identity == "" || validCatalogIdentity(q.Identity)) && (q.Status == "" || q.Status == "draft" || q.Status == "published" || q.Status == "disabled") && q.Limit >= 1 && q.Limit <= 50
}
func (s *ProfileAdministration) ListLifecycle(ctx context.Context, scope DraftScope, query ProfileLifecycleQuery) (ProfileLifecyclePage, error) {
	if err := s.authorize(ctx, scope, authz.CapabilityAuditInterpretation); err != nil {
		return ProfileLifecyclePage{}, err
	}
	if query.Limit == 0 {
		query.Limit = 20
	}
	if !query.Valid() {
		return ProfileLifecyclePage{}, ErrInvalid
	}
	return s.Gateway.ListProfileLifecycles(ctx, scope, query)
}
func (s *ProfileAdministration) GetLifecycle(ctx context.Context, scope DraftScope, identity, version string) (ProfileLifecycle, error) {
	if err := s.authorize(ctx, scope, authz.CapabilityAuditInterpretation); err != nil {
		return ProfileLifecycle{}, err
	}
	if !(AssetCatalogGet{Kind: "profile", Identity: identity, Version: version}).Valid() {
		return ProfileLifecycle{}, ErrInvalid
	}
	return s.Gateway.GetProfileLifecycle(ctx, scope, identity, version)
}
