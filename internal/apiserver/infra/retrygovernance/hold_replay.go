package retrygovernance

import (
	"context"
	"time"

	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	governance "github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type retryHoldReplayRow struct {
	ID                    uint64
	EventID               string
	OrgID                 *int64
	Status                string
	RetryDisposition      *string
	ReplayAttemptCount    int
	ManualReplayRequestID *string
}

func (retryHoldReplayRow) TableName() string { return "retry_event_hold" }

func (r *Reader) AuthorizeManualReplay(ctx context.Context, orgID int64, requestID string, targets []outboxport.ManualReplayTarget, authorizedAt time.Time) ([]outboxport.ManualReplayResult, error) {
	results := make([]outboxport.ManualReplayResult, 0, len(targets))
	err := r.mysql.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, target := range targets {
			result := outboxport.ManualReplayResult{EventID: target.EventID}
			var row retryHoldReplayRow
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("event_id = ?", target.EventID).First(&row).Error
			if err == gorm.ErrRecordNotFound {
				result.Reason = "not_found"
				results = append(results, result)
				continue
			}
			if err != nil {
				return err
			}
			switch {
			case row.OrgID == nil:
				result.Reason = "system_event_forbidden"
			case *row.OrgID != orgID:
				result.Reason = "organization_mismatch"
			case row.Status != "failed" || row.RetryDisposition == nil || *row.RetryDisposition != string(governance.DispositionManualRequired):
				result.Reason = "not_manual_required"
			case row.ReplayAttemptCount != target.ExpectedAttemptCount:
				result.Reason = "attempt_conflict"
			case row.ManualReplayRequestID != nil && *row.ManualReplayRequestID == requestID:
				result.Authorized = true
			default:
				disposition := string(governance.DispositionAutomatic)
				if err := tx.Model(&retryHoldReplayRow{}).Where("id = ?", row.ID).Updates(map[string]interface{}{
					"retry_disposition": disposition, "next_attempt_at": authorizedAt,
					"manual_replay_request_id": requestID, "updated_at": authorizedAt,
				}).Error; err != nil {
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

var _ outboxport.ManualReplayAuthorizer = (*Reader)(nil)
