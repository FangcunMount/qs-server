package systemgovernance

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
)

const deliveryReplayReviewAge = 5 * time.Minute

type deliveryReplayTargetInput struct {
	Targets []struct {
		ID uint64 `json:"id"`
	} `json:"targets"`
}

type deliveryReplayTargetRow struct {
	ID               uint64  `gorm:"column:id"`
	EventID          *string `gorm:"column:event_id"`
	EventType        *string `gorm:"column:event_type"`
	DeliveryAttempts int     `gorm:"column:delivery_attempts"`
	RetryDisposition string  `gorm:"column:retry_disposition"`
	ReplayRequestID  *string `gorm:"column:replay_request_id"`
}

// ListDeliveryReplayReviews exposes old, unfinished replay audits and failed
// audits that still own an uncertain dead letter. It never authorizes a send.
func (s *ActionAuditStore) ListDeliveryReplayReviews(ctx context.Context, orgID int64, cursor string, limit int) (app.DeliveryReplayReviewPage, error) {
	if s == nil || s.db == nil || orgID <= 0 || limit < 1 || limit > 100 {
		return app.DeliveryReplayReviewPage{}, fmt.Errorf("delivery replay review query requires a database, organization, and limit from 1 to 100")
	}
	var beforeID uint64
	if cursor != "" {
		var err error
		beforeID, err = strconv.ParseUint(cursor, 10, 64)
		if err != nil || beforeID == 0 {
			return app.DeliveryReplayReviewPage{}, fmt.Errorf("invalid delivery replay review cursor")
		}
	}
	// A publish/completion error finishes the action audit as failed while its
	// dead letter remains claimed. Keep that original request visible, but do
	// not bring back ordinary failures whose target is no longer uncertain.
	query := s.db.WithContext(ctx).Model(&actionRunPO{}).
		Where(`org_id = ? AND action_id = ? AND (
			(status IN ? AND started_at < ?) OR
			(status IN ? AND EXISTS (
				SELECT 1 FROM event_delivery_dead_letter AS d
				WHERE d.org_id = system_governance_action_runs.org_id
					AND d.replay_request_id = system_governance_action_runs.request_id
					AND d.retry_disposition = 'automatic'
			))
		)`, orgID, "events.replay_delivery",
			[]string{"running", app.ActionAuditStatusPendingReconciliation}, time.Now().Add(-deliveryReplayReviewAge),
			[]string{"failed", "timeout"})
	if beforeID != 0 {
		query = query.Where("id < ?", beforeID)
	}
	var audits []actionRunPO
	if err := query.Order("id DESC").Limit(limit + 1).Find(&audits).Error; err != nil {
		return app.DeliveryReplayReviewPage{}, err
	}
	page := app.DeliveryReplayReviewPage{Items: make([]app.DeliveryReplayReview, 0, min(limit, len(audits)))}
	if len(audits) > limit {
		page.NextCursor = strconv.FormatUint(audits[limit-1].ID, 10)
		audits = audits[:limit]
	}
	allIDs := make([]uint64, 0)
	allSeen := make(map[uint64]struct{})
	for _, audit := range audits {
		item := app.DeliveryReplayReview{
			RequestID: audit.RequestID, ActorUserID: strconv.FormatUint(audit.ActorUserID, 10),
			Status: audit.Status, StartedAt: audit.StartedAt, UpdatedAt: audit.UpdatedAt,
			Targets: make([]app.DeliveryReplayReviewTarget, 0),
		}
		var input deliveryReplayTargetInput
		if err := json.Unmarshal([]byte(audit.InputJSON), &input); err == nil && len(input.Targets) > 0 && len(input.Targets) <= 100 {
			seen := make(map[uint64]struct{}, len(input.Targets))
			for _, target := range input.Targets {
				if target.ID == 0 {
					break
				}
				if _, exists := seen[target.ID]; exists {
					break
				}
				seen[target.ID] = struct{}{}
				item.Targets = append(item.Targets, app.DeliveryReplayReviewTarget{DeadLetterID: target.ID, Disposition: "unavailable"})
			}
			item.TargetsReadable = len(item.Targets) == len(input.Targets)
			if !item.TargetsReadable {
				item.Targets = item.Targets[:0]
			}
		}
		if item.TargetsReadable {
			for _, target := range item.Targets {
				if _, exists := allSeen[target.DeadLetterID]; !exists {
					allSeen[target.DeadLetterID] = struct{}{}
					allIDs = append(allIDs, target.DeadLetterID)
				}
			}
		}
		page.Items = append(page.Items, item)
	}
	if len(allIDs) == 0 {
		return page, nil
	}
	var rows []deliveryReplayTargetRow
	if err := s.db.WithContext(ctx).Table("event_delivery_dead_letter").
		Select(`id, event_id, delivery_attempts, retry_disposition, replay_request_id,
			IF(JSON_VALID(payload_json), JSON_UNQUOTE(JSON_EXTRACT(payload_json, '$.eventType')), NULL) AS event_type`).
		Where("org_id = ? AND id IN ?", orgID, allIDs).Find(&rows).Error; err != nil {
		return app.DeliveryReplayReviewPage{}, err
	}
	byID := make(map[uint64]deliveryReplayTargetRow, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}
	for i := range page.Items {
		for j, target := range page.Items[i].Targets {
			if row, exists := byID[target.DeadLetterID]; exists {
				page.Items[i].Targets[j].Disposition = row.RetryDisposition
				if row.EventID != nil {
					page.Items[i].Targets[j].EventID = *row.EventID
				}
				if row.EventType != nil {
					page.Items[i].Targets[j].EventType = *row.EventType
				}
				page.Items[i].Targets[j].DeliveryAttempts = row.DeliveryAttempts
				page.Items[i].Targets[j].LinkedToRequest = row.ReplayRequestID != nil && *row.ReplayRequestID == page.Items[i].RequestID
			}
		}
	}
	return page, nil
}

var _ app.DeliveryReplayReviewReader = (*ActionAuditStore)(nil)
