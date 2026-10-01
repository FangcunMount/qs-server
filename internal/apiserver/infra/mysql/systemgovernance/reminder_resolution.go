package systemgovernance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	baseerrors "github.com/FangcunMount/component-base/pkg/errors"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const reminderResolutionActionID = "notifications.resolve_reminder"

type reminderResolutionRow struct {
	ID                    uint64     `gorm:"column:id;primaryKey"`
	OrgID                 int64      `gorm:"column:org_id"`
	TaskID                string     `gorm:"column:task_id"`
	OpeningEventID        string     `gorm:"column:opening_event_id"`
	State                 string     `gorm:"column:state"`
	ExternalCallStartedAt *time.Time `gorm:"column:external_call_started_at"`
	ResolutionCode        string     `gorm:"column:resolution_code"`
	UpdatedAt             time.Time  `gorm:"column:updated_at"`
}

func (reminderResolutionRow) TableName() string { return "task_opened_reminder_delivery" }

// ResolveReminder atomically records an operator's finding and seals one
// uncertain responsibility. No publisher, sender, or recipient data is used.
// An interrupted sending row may only be closed as unknown, with explicit
// acknowledgment that the original external call can still finish.
func (s *ActionAuditStore) ResolveReminder(ctx context.Context, orgID int64, actorID uint64, req app.ReminderResolutionRequest) (*app.ActionRunResult, error) {
	if s == nil || s.db == nil || orgID <= 0 || actorID == 0 || !req.Confirm || req.DeliveryID == 0 || req.ExpectedUpdatedAt.IsZero() {
		return nil, baseerrors.WithCode(code.ErrInvalidArgument, "confirmed scoped reminder identity is required")
	}
	for _, value := range []string{req.RequestID, req.TaskID, req.OpeningEventID} {
		if strings.TrimSpace(value) == "" || len(value) > 64 {
			return nil, baseerrors.WithCode(code.ErrInvalidArgument, "invalid reminder resolution identity")
		}
	}
	if strings.TrimSpace(req.Reason) == "" || len(req.Reason) > 2000 || strings.TrimSpace(req.EvidenceReference) == "" || len(req.EvidenceReference) > 255 {
		return nil, baseerrors.WithCode(code.ErrInvalidArgument, "bounded reason and evidence reference are required")
	}
	switch req.Finding {
	case "recipient_received", "platform_rejected", "unknown_no_resend":
	default:
		return nil, baseerrors.WithCode(code.ErrInvalidArgument, "invalid manual finding")
	}
	input, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	var result *app.ActionRunResult
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row reminderResolutionRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND org_id = ?", req.DeliveryID, orgID).Take(&row).Error; err != nil {
			return fmt.Errorf("lock reminder: %w", err)
		}
		var prior actionRunPO
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("org_id = ? AND request_id = ?", orgID, req.RequestID).Take(&prior).Error
		if err == nil {
			if prior.ActionID != reminderResolutionActionID || prior.ActorUserID != actorID || prior.Status != "succeeded" || !equalAuditJSON(prior.InputJSON, string(input)) {
				return baseerrors.WithCode(code.ErrConflict, "resolution request identity conflict")
			}
			replay, err := decodeActionAuditReplay(prior.ResultJSON)
			if err != nil || replay == nil || replay.Result == nil {
				return fmt.Errorf("committed reminder receipt unreadable")
			}
			result = replay.Result
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if row.TaskID != req.TaskID || row.OpeningEventID != req.OpeningEventID || (row.State != "manual_required" && row.State != "sending") || row.ExternalCallStartedAt == nil || !row.UpdatedAt.Equal(req.ExpectedUpdatedAt) {
			return baseerrors.WithCode(code.ErrConflict, "reminder identity or expected state changed")
		}
		if row.State == "sending" && (req.Finding != "unknown_no_resend" || !req.AcknowledgeOriginalCallMayComplete) {
			return baseerrors.WithCode(code.ErrConflict, "original call may still complete; only acknowledged unknown closure is permitted")
		}
		now := time.Now().In(time.FixedZone("UTC+8", 8*3600))
		result = &app.ActionRunResult{RequestID: req.RequestID, ActionID: reminderResolutionActionID, Status: "succeeded", StartedAt: now, FinishedAt: now, Result: map[string]interface{}{
			"delivery_id": req.DeliveryID, "task_id": req.TaskID, "opening_event_id": req.OpeningEventID, "finding": req.Finding, "evidence_reference": req.EvidenceReference, "delivery_state": "reviewed", "automatic_resend": false, "original_call_may_complete": row.State == "sending",
		}}
		encoded, err := json.Marshal(actionAuditEnvelope{SchemaVersion: 2, Result: result})
		if err != nil {
			return err
		}
		audit := actionRunPO{RequestID: req.RequestID, ActionID: reminderResolutionActionID, OrgID: orgID, ActorUserID: actorID, InputJSON: string(input), Status: "succeeded", ResultJSON: string(encoded), StartedAt: now, FinishedAt: &now}
		if err := tx.Create(&audit).Error; err != nil {
			return fmt.Errorf("persist reminder resolution audit: %w", err)
		}
		update := tx.Model(&reminderResolutionRow{}).Where("id = ? AND org_id = ? AND state = ? AND updated_at = ?", row.ID, orgID, row.State, row.UpdatedAt).Updates(map[string]interface{}{"state": "reviewed", "resolution_code": "manual_" + req.Finding, "updated_at": now})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return baseerrors.WithCode(code.ErrConflict, "reminder resolution conflict")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// LoadReminderResolution retrieves a committed receipt after a lost response.
// It neither changes the ledger nor authorizes another platform call.
func (s *ActionAuditStore) LoadReminderResolution(ctx context.Context, orgID int64, actorID uint64, requestID string) (*app.ActionRunResult, error) {
	if s == nil || s.db == nil || orgID <= 0 || actorID == 0 || strings.TrimSpace(requestID) == "" || len(requestID) > 64 {
		return nil, baseerrors.WithCode(code.ErrInvalidArgument, "scoped reminder receipt identity is required")
	}
	var row actionRunPO
	if err := s.db.WithContext(ctx).Where("org_id = ? AND actor_user_id = ? AND request_id = ? AND action_id = ? AND status = ?", orgID, actorID, requestID, reminderResolutionActionID, "succeeded").Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, baseerrors.WithCode(code.ErrPageNotFound, "reminder resolution receipt unavailable")
		}
		return nil, err
	}
	replay, err := decodeActionAuditReplay(row.ResultJSON)
	if err != nil || replay == nil || replay.Result == nil || replay.Result.RequestID != requestID || replay.Result.ActionID != reminderResolutionActionID {
		return nil, fmt.Errorf("committed reminder receipt unreadable")
	}
	return replay.Result, nil
}
