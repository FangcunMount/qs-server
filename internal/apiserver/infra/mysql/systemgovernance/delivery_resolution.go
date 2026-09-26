package systemgovernance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const deliveryResolutionActionID = "events.resolve_delivery"
const deliveryResolutionDisposition = "resolved_verified"

// DeliveryResolutionRequest identifies one previously claimed physical dead
// letter. A new request ID records the resolution without rewriting the
// original replay audit or publishing the event again.
type DeliveryResolutionRequest struct {
	OrgID                    int64
	ActorUserID              uint64
	RequestID                string
	OriginalReplayRequestID  string
	DeadLetterID             uint64
	EventID                  string
	ExpectedDeliveryAttempts int
	Reason                   string
}

// DeliveryResolutionSubject is the locked row presented to an event-specific
// verifier. PayloadJSON is evidence input, not an authorization to republish.
type DeliveryResolutionSubject struct {
	OrgID             int64
	DeadLetterID      uint64
	EventID           string
	PayloadJSON       string
	OriginalRequestID string
}

// DeliveryResolutionEvidence must prove every relevant business effect before
// the transport dead letter can be closed. A projection-only verifier must not
// return AllEffectsConfirmed.
type DeliveryResolutionEvidence struct {
	Kind                string
	Reference           string
	AllEffectsConfirmed bool
}

// DeliveryResolutionVerifier checks event-specific, durable business facts
// while the original audit and physical dead letter are locked. A nil verifier
// or partial evidence always rejects the resolution.
type DeliveryResolutionVerifier func(context.Context, *gorm.DB, DeliveryResolutionSubject) (DeliveryResolutionEvidence, error)

type deliveryResolutionDeadLetter struct {
	ID               uint64  `gorm:"column:id"`
	OrgID            *int64  `gorm:"column:org_id"`
	EventID          *string `gorm:"column:event_id"`
	DeliveryAttempts int     `gorm:"column:delivery_attempts"`
	PayloadJSON      string  `gorm:"column:payload_json"`
	RetryDisposition string  `gorm:"column:retry_disposition"`
	ReplayRequestID  *string `gorm:"column:replay_request_id"`
}

func (deliveryResolutionDeadLetter) TableName() string { return "event_delivery_dead_letter" }

type deliveryResolutionInput struct {
	OriginalReplayRequestID  string `json:"original_replay_request_id"`
	DeadLetterID             uint64 `json:"dead_letter_id"`
	EventID                  string `json:"event_id"`
	ExpectedDeliveryAttempts int    `json:"expected_delivery_attempts"`
	Reason                   string `json:"reason"`
}

// ResolveDelivery atomically records a distinct resolution audit and removes
// one proven dead letter from the replay queue. It is intentionally not wired
// to the action registry until an all-effects verifier and operator flow exist.
func (s *ActionAuditStore) ResolveDelivery(ctx context.Context, req DeliveryResolutionRequest, verify DeliveryResolutionVerifier) error {
	if s == nil || s.db == nil || verify == nil || req.OrgID <= 0 || req.DeadLetterID == 0 ||
		req.ExpectedDeliveryAttempts < 1 || req.RequestID == "" || req.RequestID == req.OriginalReplayRequestID ||
		req.OriginalReplayRequestID == "" || req.EventID == "" || strings.TrimSpace(req.Reason) == "" ||
		len(req.RequestID) > 64 || len(req.OriginalReplayRequestID) > 64 {
		return fmt.Errorf("invalid or unverified delivery resolution request")
	}
	input, err := json.Marshal(deliveryResolutionInput{
		OriginalReplayRequestID: req.OriginalReplayRequestID,
		DeadLetterID:            req.DeadLetterID, EventID: req.EventID,
		ExpectedDeliveryAttempts: req.ExpectedDeliveryAttempts, Reason: req.Reason,
	})
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var original actionRunPO
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("org_id = ? AND request_id = ?", req.OrgID, req.OriginalReplayRequestID).
			Take(&original).Error; err != nil {
			return fmt.Errorf("lock original replay audit: %w", err)
		}
		if original.ActionID != "events.replay_delivery" ||
			!resolutionAuditSettled(original) || !originalTargetsDeadLetter(original.InputJSON, req.DeadLetterID, req.ExpectedDeliveryAttempts) {
			return fmt.Errorf("original replay audit does not authorize this resolution")
		}
		var deadLetter deliveryResolutionDeadLetter
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", req.DeadLetterID).Take(&deadLetter).Error; err != nil {
			return fmt.Errorf("lock delivery dead letter: %w", err)
		}
		var prior actionRunPO
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("org_id = ? AND request_id = ?", req.OrgID, req.RequestID).Take(&prior).Error
		if err == nil {
			if prior.ActionID == deliveryResolutionActionID && prior.ActorUserID == req.ActorUserID &&
				prior.Status == "succeeded" && equalAuditJSON(prior.InputJSON, string(input)) {
				return nil
			}
			return fmt.Errorf("resolution request ID already belongs to another outcome")
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if deadLetter.OrgID == nil || *deadLetter.OrgID != req.OrgID ||
			deadLetter.EventID == nil || *deadLetter.EventID != req.EventID ||
			deadLetter.DeliveryAttempts != req.ExpectedDeliveryAttempts ||
			deadLetter.RetryDisposition != "automatic" ||
			deadLetter.ReplayRequestID == nil || *deadLetter.ReplayRequestID != req.OriginalReplayRequestID {
			return fmt.Errorf("delivery dead letter identity or state changed")
		}
		evidence, err := verify(ctx, tx, DeliveryResolutionSubject{
			OrgID: req.OrgID, DeadLetterID: req.DeadLetterID, EventID: req.EventID,
			PayloadJSON: deadLetter.PayloadJSON, OriginalRequestID: req.OriginalReplayRequestID,
		})
		if err != nil {
			return fmt.Errorf("verify delivery business effects: %w", err)
		}
		if !evidence.AllEffectsConfirmed || evidence.Kind == "" || evidence.Reference == "" {
			return fmt.Errorf("complete business-effect evidence is required")
		}
		now := time.Now().In(time.FixedZone("UTC+8", 8*60*60))
		result := &app.ActionRunResult{
			RequestID: req.RequestID, ActionID: deliveryResolutionActionID,
			StartedAt: now, FinishedAt: now, Status: "succeeded",
			Result: map[string]interface{}{
				"dead_letter_id": req.DeadLetterID, "event_id": req.EventID,
				"original_replay_request_id": req.OriginalReplayRequestID,
				"evidence_kind":              evidence.Kind, "evidence_reference": evidence.Reference,
			},
		}
		encodedResult, err := json.Marshal(actionAuditEnvelope{SchemaVersion: 2, Result: result})
		if err != nil {
			return err
		}
		row := actionRunPO{
			RequestID: req.RequestID, ActionID: deliveryResolutionActionID, OrgID: req.OrgID,
			ActorUserID: req.ActorUserID, InputJSON: string(input), Status: "succeeded",
			ResultJSON: string(encodedResult), StartedAt: now, FinishedAt: &now,
		}
		if err := tx.Create(&row).Error; err != nil {
			return fmt.Errorf("persist delivery resolution audit: %w", err)
		}
		updated := tx.Model(&deliveryResolutionDeadLetter{}).
			Where("id = ? AND org_id = ? AND retry_disposition = ? AND replay_request_id = ? AND delivery_attempts = ?",
				req.DeadLetterID, req.OrgID, "automatic", req.OriginalReplayRequestID, req.ExpectedDeliveryAttempts).
			Updates(map[string]interface{}{"retry_disposition": deliveryResolutionDisposition, "updated_at": now})
		if updated.Error != nil {
			return fmt.Errorf("settle delivery dead letter: %w", updated.Error)
		}
		if updated.RowsAffected != 1 {
			return fmt.Errorf("delivery dead letter settlement conflict")
		}
		return nil
	})
}

func resolutionAuditSettled(original actionRunPO) bool {
	switch original.Status {
	case "failed", "timeout", app.ActionAuditStatusPendingReconciliation:
		return true
	case "running":
		return original.StartedAt.Before(time.Now().Add(-deliveryReplayReviewAge))
	default:
		return false
	}
}

func originalTargetsDeadLetter(inputJSON string, deadLetterID uint64, attempts int) bool {
	var input struct {
		Targets []struct {
			ID                       uint64 `json:"id"`
			ExpectedDeliveryAttempts int    `json:"expected_delivery_attempts"`
		} `json:"targets"`
	}
	if err := json.Unmarshal([]byte(inputJSON), &input); err != nil {
		return false
	}
	for _, target := range input.Targets {
		if target.ID == deadLetterID && target.ExpectedDeliveryAttempts == attempts {
			return true
		}
	}
	return false
}
