package aibridge

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"time"
	"unicode/utf8"

	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/google/uuid"
)

type EvaluationCatalogQuery struct {
	Status string `json:"status"`
	Limit  int    `json:"limit"`
	Cursor string `json:"cursor"`
}

type EvaluationSummary struct {
	RunID                        string `json:"run_id"`
	OrganizationID               int64  `json:"organization_id"`
	Version                      int64  `json:"version"`
	Status                       string `json:"status"`
	CreatedAt                    string `json:"created_at"`
	RequestedBy                  string `json:"requested_by"`
	ProfileID                    string `json:"profile_id"`
	ProfileVersion               string `json:"profile_version"`
	PromptID                     string `json:"prompt_id"`
	PromptVersion                string `json:"prompt_version"`
	ReleaseFingerprint           string `json:"release_fingerprint"`
	UnresolvedResultUnknownCount int64  `json:"unresolved_result_unknown_count"`
	ReviewCount                  int64  `json:"review_count"`
	RequiredCandidates           int64  `json:"required_candidates"`
	AcceptedCandidates           int64  `json:"accepted_candidates"`
	ReviewReadyCandidates        int64  `json:"review_ready_candidates"`
	LastCause                    string `json:"last_cause"`
	LastReason                   string `json:"last_reason"`
}

type EvaluationCatalogPage struct {
	Items      []EvaluationSummary `json:"items"`
	NextCursor string              `json:"next_cursor"`
}

func ValidEvaluationCatalogStatus(value string) bool {
	switch value {
	case "requested", "collecting", "blocked", "awaiting_review", "approved", "rejected", "canceled":
		return true
	}
	return false
}

func (s EvaluationSummary) Valid(org int64) bool {
	id, err := uuid.Parse(s.RunID)
	_, dateErr := time.Parse(time.RFC3339Nano, s.CreatedAt)
	return err == nil && id != uuid.Nil && id.String() == s.RunID && org > 0 && s.OrganizationID == org && s.Version > 0 && ValidEvaluationCatalogStatus(s.Status) && dateErr == nil &&
		frozenID.MatchString(s.RequestedBy) && frozenID.MatchString(s.ProfileID) && frozenVersion.MatchString(s.ProfileVersion) && frozenID.MatchString(s.PromptID) && frozenVersion.MatchString(s.PromptVersion) && frozenFingerprint.MatchString(s.ReleaseFingerprint) &&
		s.UnresolvedResultUnknownCount >= 0 && s.ReviewCount >= 0 && s.RequiredCandidates > 0 && s.AcceptedCandidates >= 0 && s.AcceptedCandidates <= s.RequiredCandidates && s.ReviewReadyCandidates >= 0 && s.ReviewReadyCandidates <= s.AcceptedCandidates &&
		len(s.LastCause) <= 128 && len(s.LastReason) <= 1000 && utf8.ValidString(s.LastCause) && utf8.ValidString(s.LastReason)
}

// After checks transport continuity, not authorization or AI review policy.
func (q EvaluationCatalogQuery) After(org int64) (time.Time, string, bool) {
	if len(q.Cursor) > 1024 {
		return time.Time{}, "", false
	}
	if q.Cursor == "" {
		return time.Time{}, "", true
	}
	raw, err := base64.URLEncoding.Strict().DecodeString(q.Cursor)
	if err != nil {
		return time.Time{}, "", false
	}
	var parts []json.RawMessage
	if json.Unmarshal(raw, &parts) != nil || len(parts) != 6 {
		return time.Time{}, "", false
	}
	var version int
	var kind, status, created, runID string
	var organization int64
	if json.Unmarshal(parts[0], &version) != nil || version != 1 || json.Unmarshal(parts[1], &kind) != nil || kind != "evaluation" || json.Unmarshal(parts[2], &organization) != nil || organization != org || json.Unmarshal(parts[3], &status) != nil || status != q.Status || json.Unmarshal(parts[4], &created) != nil || json.Unmarshal(parts[5], &runID) != nil {
		return time.Time{}, "", false
	}
	at, timeErr := time.Parse(time.RFC3339Nano, created)
	id, idErr := uuid.Parse(runID)
	return at, runID, timeErr == nil && idErr == nil && id != uuid.Nil && id.String() == runID
}

func (q EvaluationCatalogQuery) Valid(org int64) bool {
	_, _, cursorOK := q.After(org)
	return org > 0 && q.Limit >= 1 && q.Limit <= 100 && (q.Status == "" || ValidEvaluationCatalogStatus(q.Status)) && cursorOK
}

func (s *EvaluationAdministration) List(ctx context.Context, scope DraftScope, query EvaluationCatalogQuery) (EvaluationCatalogPage, error) {
	if scope.OrganizationID <= 0 || scope.OperatorUserID <= 0 {
		return EvaluationCatalogPage{}, ErrInvalid
	}
	snapshot, ok := authz.FromContext(ctx)
	if !ok || !authz.DecideCapability(snapshot, authz.CapabilityAuditInterpretation).Allowed {
		return EvaluationCatalogPage{}, ErrGovernanceDenied
	}
	if s == nil || s.Gateway == nil {
		return EvaluationCatalogPage{}, ErrManagementUnavailable
	}
	if query.Limit == 0 {
		query.Limit = 20
	}
	if !query.Valid(scope.OrganizationID) {
		return EvaluationCatalogPage{}, ErrInvalid
	}
	return s.Gateway.ListEvaluations(ctx, scope, query)
}
