package aibridge

import (
	"context"

	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
)

type PublicationHistoryQuery struct {
	Selector      PublicationSelector `json:"selector"`
	BeforeVersion int64               `json:"before_version"`
	Limit         int32               `json:"limit"`
}

func (q PublicationHistoryQuery) Valid() bool {
	return q.Selector.Valid() && q.BeforeVersion >= 0 && q.Limit >= 1 && q.Limit <= 20
}

type PublicationHistoryEntry struct {
	Version               int64   `json:"version"`
	CommandID             string  `json:"command_id"`
	Action                string  `json:"action"`
	Actor                 string  `json:"actor"`
	Reason                string  `json:"reason"`
	ChangedAt             string  `json:"changed_at"`
	PreviousPublicationID *string `json:"previous_publication_id"`
	PublicationID         *string `json:"publication_id"`
	RunID                 *string `json:"run_id"`
	RunVersion            *int64  `json:"run_version"`
	ProfileID             *string `json:"profile_id"`
	ProfileVersion        *string `json:"profile_version"`
}
type PublicationHistoryPage struct {
	SchemaVersion     string                    `json:"schema_version"`
	Selector          PublicationSelector       `json:"selector"`
	Entries           []PublicationHistoryEntry `json:"entries"`
	NextBeforeVersion int64                     `json:"next_before_version"`
}

func (s *PublicationAdministration) ListHistory(ctx context.Context, scope PublicationScope, query PublicationHistoryQuery) (PublicationHistoryPage, error) {
	if err := s.authorize(ctx, scope, authz.CapabilityAuditInterpretation); err != nil {
		return PublicationHistoryPage{}, err
	}
	if !query.Valid() {
		return PublicationHistoryPage{}, ErrInvalid
	}
	return s.Gateway.ListPublicationHistory(ctx, scope, query)
}
func (s *PublicationAdministration) GetHistory(ctx context.Context, scope PublicationScope, selector PublicationSelector, version int64) (PublicationReceipt, error) {
	if err := s.authorize(ctx, scope, authz.CapabilityAuditInterpretation); err != nil {
		return PublicationReceipt{}, err
	}
	if !selector.Valid() || version < 1 {
		return PublicationReceipt{}, ErrInvalid
	}
	return s.Gateway.GetPublicationHistory(ctx, scope, selector, version)
}
