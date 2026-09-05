package evaluation

import (
	"context"
	"time"

	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// EvidenceV2Catalog is a bounded read projection, never a Provider-output list.
type EvidenceV2Catalog interface {
	ListEvidenceV2(context.Context, int64, *domainevaluation.EvidenceStatus, string, int) ([]EvidenceV2Summary, string, error)
}
type EvidenceV2Summary struct {
	RunID                        string                          `json:"run_id"`
	OrganizationID               int64                           `json:"organization_id"`
	Version                      int64                           `json:"version"`
	Status                       domainevaluation.EvidenceStatus `json:"status"`
	CreatedAt                    time.Time                       `json:"created_at"`
	PromptVersion                string                          `json:"prompt_version"`
	ProfileVersion               string                          `json:"profile_version"`
	ReleaseFingerprint           string                          `json:"release_fingerprint"`
	RequiredCandidates           int                             `json:"required_candidates"`
	AcceptedCandidates           int                             `json:"accepted_candidates"`
	ReviewReadyCandidates        int                             `json:"review_ready_candidates"`
	ReviewCount                  int                             `json:"review_count"`
	UnresolvedResultUnknownCount int                             `json:"unresolved_result_unknown_count"`
	LastCause                    string                          `json:"last_cause"`
	LastReason                   string                          `json:"last_reason,omitempty"`
	CanCancel                    bool                            `json:"can_cancel"`
	CanDiscard                   bool                            `json:"can_discard"`
}
type EvidenceV2Page struct {
	Items      []EvidenceV2Summary `json:"items"`
	NextCursor string              `json:"next_cursor,omitempty"`
}

func (s *EvidenceV2Service) Cancel(ctx context.Context, runID meta.ID, expectedVersion int64, actor, reason string, discard bool, at time.Time) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	return s.mutate(ctx, runID, func(value *domainevaluation.PromptEvaluationEvidenceV2) error {
		if value.Version() != expectedVersion {
			return domainevaluation.ErrConflict
		}
		return value.Cancel(actor, reason, discard, at)
	})
}
