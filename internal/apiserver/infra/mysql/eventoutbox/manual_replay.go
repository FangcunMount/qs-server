package eventoutbox

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/outboxcore"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Store) AuthorizeManualReplay(ctx context.Context, orgID int64, requestID string, targets []outboxport.ManualReplayTarget, authorizedAt time.Time) ([]outboxport.ManualReplayResult, error) {
	results := make([]outboxport.ManualReplayResult, 0, len(targets))
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, target := range targets {
			result := outboxport.ManualReplayResult{EventID: target.EventID}
			var row OutboxPO
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("event_id = ?", target.EventID).First(&row).Error; err != nil {
				if err == gorm.ErrRecordNotFound {
					result.Reason = "not_found"
					results = append(results, result)
					continue
				}
				return err
			}
			switch {
			case row.OrgID == nil:
				result.Reason = "system_event_forbidden"
			case *row.OrgID != orgID:
				result.Reason = "organization_mismatch"
			case row.Status != outboxcore.StatusFailed || row.RetryDisposition == nil || *row.RetryDisposition != string(retrygovernance.DispositionManualRequired):
				result.Reason = "not_manual_required"
			case row.AttemptCount != target.ExpectedAttemptCount:
				result.Reason = "attempt_conflict"
			default:
				disposition := string(retrygovernance.DispositionAutomatic)
				updates := map[string]interface{}{
					"retry_disposition": disposition, "next_attempt_at": authorizedAt,
					"manual_replay_request_id": requestID, "updated_at": authorizedAt,
				}
				if err := tx.Model(&OutboxPO{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
					return err
				}
				result.Authorized = true
			}
			results = append(results, result)
		}
		return nil
	})
	return results, err
}

var _ outboxport.ManualReplayAuthorizer = (*Store)(nil)
