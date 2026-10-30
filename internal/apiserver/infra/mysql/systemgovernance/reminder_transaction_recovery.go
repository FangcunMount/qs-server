package systemgovernance

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	baseerrors "github.com/FangcunMount/component-base/pkg/errors"
	notification "github.com/FangcunMount/qs-server/internal/apiserver/application/notification"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	planrepo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/plan"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/taskopen"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const currentReminderAction = "notifications.recover_missing_reminder_batch.transaction"

type CurrentReminderRecovery struct {
	db      *gorm.DB
	service notification.TaskOpenedReminderService
	capture func(context.Context, int64, uint64, uint32, time.Time) (taskopen.RecoveryOpening, error)
	now     func() time.Time
}

func NewCurrentReminderRecovery(db *gorm.DB, service notification.TaskOpenedReminderService, artifact func(transaction.FrozenPlan) ([]byte, error)) *CurrentReminderRecovery {
	source := taskopen.NewRecoverySource(db, artifact)
	return &CurrentReminderRecovery{db: db, service: service, capture: source.Capture, now: time.Now}
}

func currentReminderRecord(orgID int64, actorID uint64, req app.ReminderGapRecoveryRequest) (app.ActionAuditRecord, error) {
	if err := app.ValidateTransactionReminderGap(orgID, actorID, req, false); err != nil {
		return app.ActionAuditRecord{}, err
	}
	req.OpenedBefore = req.OpenedBefore.UTC()
	raw, err := json.Marshal(req)
	if err != nil {
		return app.ActionAuditRecord{}, err
	}
	var input map[string]interface{}
	if err := json.Unmarshal(raw, &input); err != nil {
		return app.ActionAuditRecord{}, err
	}
	return app.ActionAuditRecord{RequestID: req.RequestID, ActionID: currentReminderAction, OrgID: orgID, ActorUserID: actorID, Component: "notifications", TargetInstance: req.TaskID, Input: input, StartedAt: time.Now()}, nil
}

func (s *CurrentReminderRecovery) source(ctx context.Context, orgID int64, req app.ReminderGapRecoveryRequest) (taskopen.RecoveryOpening, error) {
	id, err := strconv.ParseUint(req.TaskID, 10, 64)
	if err != nil || s == nil || s.db == nil || s.capture == nil {
		return taskopen.RecoveryOpening{}, taskopen.ErrUnsafeRecoveryOpening
	}
	o, err := s.capture(ctx, orgID, id, req.ExpectedScheduleRevision, req.OpenedBefore)
	if err != nil {
		return o, err
	}
	if req.OpeningEventID != "" && req.OpeningEventID != o.Request.OpeningEventID {
		return o, taskopen.ErrUnsafeRecoveryOpening
	}
	if req.SourceFingerprint != "" && req.SourceFingerprint != o.Fingerprint {
		return o, taskopen.ErrUnsafeRecoveryOpening
	}
	return o, nil
}

func (s *CurrentReminderRecovery) InspectReminderGap(ctx context.Context, orgID int64, actorID uint64, req app.ReminderGapRecoveryRequest) (*app.ReminderGapInspection, error) {
	if err := app.ValidateTransactionReminderGap(orgID, actorID, req, true); err != nil {
		return nil, err
	}
	o, err := s.source(ctx, orgID, req)
	if err != nil {
		return nil, err
	}
	req.OpeningEventID = o.Request.OpeningEventID
	progress, err := s.progress(ctx, orgID, req)
	if err != nil {
		return nil, err
	}
	disposition := progress.phase
	var count int64
	if err := s.reservations(s.db.WithContext(ctx), orgID, req).Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		disposition = "original_operation_exists"
	}
	return &app.ReminderGapInspection{TaskID: req.TaskID, OpeningEventID: req.OpeningEventID, ScheduleRevision: req.ExpectedScheduleRevision, SourceFingerprint: o.Fingerprint, Deadline: o.Request.Intent.OpenAt.Add(notification.TaskOpenedReminderWindow), Disposition: disposition, ReadOnly: true}, nil
}

func (s *CurrentReminderRecovery) reservations(db *gorm.DB, orgID int64, req app.ReminderGapRecoveryRequest) *gorm.DB {
	return db.Model(&actionRunPO{}).Where("org_id=? AND action_id=? AND target_instance=? AND JSON_UNQUOTE(JSON_EXTRACT(input_json,'$.opening_event_id'))=?", orgID, currentReminderAction, req.TaskID, req.OpeningEventID)
}

func (s *CurrentReminderRecovery) AuthorizeReminderGap(ctx context.Context, orgID int64, actorID uint64, req app.ReminderGapRecoveryRequest) (*app.ActionRunResult, error) {
	record, err := currentReminderRecord(orgID, actorID, req)
	if err != nil {
		return nil, err
	}
	// Exact existing operations remain readable after the source window closes.
	if _, err := s.load(ctx, record); err == nil {
		result, _, err := s.ResolveReminderGap(ctx, orgID, actorID, req)
		return result, err
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if s == nil || s.db == nil || s.service == nil {
		return nil, errors.New("current reminder recovery unavailable")
	}
	o, err := s.source(ctx, orgID, req)
	if err != nil {
		return nil, err
	}
	claimed := false
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current planrepo.AssessmentTaskPO
		if err := tx.Table("assessment_task").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND org_id=? AND deleted_at IS NULL", req.TaskID, orgID).Take(&current).Error; err != nil {
			return err
		}
		if !o.MatchesCurrent(&current) {
			return taskopen.ErrUnsafeRecoveryOpening
		}
		var prior []actionRunPO
		if err := s.reservations(tx, orgID, req).Clauses(clause.Locking{Strength: "UPDATE"}).Limit(2).Find(&prior).Error; err != nil {
			return err
		}
		if len(prior) > 0 {
			if len(prior) != 1 || prior[0].RequestID != req.RequestID {
				return baseerrors.WithCode(code.ErrConflict, "original reminder recovery operation already exists")
			}
			_, _, err := NewActionAuditStore(tx).Claim(ctx, record)
			return err
		}
		progress, err := readCurrentReminderProgress(tx, orgID, req, true)
		if err != nil {
			return err
		}
		if progress.phase != "missing_batch" {
			return baseerrors.WithCode(code.ErrConflict, "original reminder responsibility already exists")
		}
		_, claimed, err = NewActionAuditStore(tx).Claim(ctx, record)
		return err
	})
	if err != nil {
		return nil, err
	}
	if !claimed {
		result, _, err := s.ResolveReminderGap(ctx, orgID, actorID, req)
		return result, err
	}
	return s.drive(ctx, orgID, req, record)
}

func (s *CurrentReminderRecovery) load(ctx context.Context, record app.ActionAuditRecord) (*actionRunPO, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("current reminder recovery unavailable")
	}
	var row actionRunPO
	err := s.db.WithContext(ctx).Where("org_id=? AND request_id=?", record.OrgID, record.RequestID).Take(&row).Error
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(record.Input)
	if err != nil {
		return nil, err
	}
	if row.ActionID != record.ActionID || row.ActorUserID != record.ActorUserID || row.Component != record.Component || row.TargetInstance != record.TargetInstance || !equalAuditJSON(row.InputJSON, string(raw)) {
		return nil, app.ErrActionAuditInputConflict
	}
	return &row, nil
}

func storedCurrentReminderResult(row *actionRunPO) (*app.ActionRunResult, error) {
	if row.Status == "running" || row.Status == app.ActionAuditStatusPendingReconciliation {
		return nil, nil
	}
	prior, err := decodeActionAuditReplay(row.ResultJSON)
	if err != nil || prior == nil || prior.Result == nil || prior.Error != nil {
		return nil, errors.New("original current reminder receipt is invalid")
	}
	return prior.Result, nil
}

func (s *CurrentReminderRecovery) ResolveReminderGap(ctx context.Context, orgID int64, actorID uint64, req app.ReminderGapRecoveryRequest) (*app.ActionRunResult, bool, error) {
	record, err := currentReminderRecord(orgID, actorID, req)
	if err != nil {
		return nil, false, err
	}
	row, err := s.load(ctx, record)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if result, err := storedCurrentReminderResult(row); result != nil || err != nil {
		return result, true, err
	}
	progress, err := s.progress(ctx, orgID, req)
	if err != nil {
		return nil, true, err
	}
	return reminderRecoveryResult(record, row.StartedAt, progress, false, s.now()), true, nil
}

func (s *CurrentReminderRecovery) ResumeReminderGap(ctx context.Context, orgID int64, actorID uint64, req app.ReminderGapRecoveryRequest) (*app.ActionRunResult, error) {
	record, err := currentReminderRecord(orgID, actorID, req)
	if err != nil {
		return nil, err
	}
	row, err := s.load(ctx, record)
	if err != nil {
		return nil, err
	}
	if result, err := storedCurrentReminderResult(row); result != nil || err != nil {
		return result, err
	}
	record.StartedAt = row.StartedAt
	return s.drive(ctx, orgID, req, record)
}

func (s *CurrentReminderRecovery) drive(ctx context.Context, orgID int64, req app.ReminderGapRecoveryRequest, record app.ActionAuditRecord) (*app.ActionRunResult, error) {
	progress, err := s.progress(ctx, orgID, req)
	if err != nil {
		return nil, err
	}
	if progress.phase != "completed" && progress.phase != "manual_required" {
		o, sourceErr := s.source(ctx, orgID, req)
		if sourceErr != nil {
			progress.phase = "source_unavailable_or_changed"
		} else {
			if s.service == nil {
				return nil, errors.New("current reminder recovery unavailable")
			}
			// The original service owns recipient freeze, call markers and result
			// semantics. Its return value alone is never a completion receipt.
			_ = s.service.ProcessTaskOpenedReminder(ctx, o.Request)
			settleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			progress, err = s.progress(settleCtx, orgID, req)
			cancel()
			if err != nil {
				return nil, err
			}
		}
	}
	terminal := progress.phase == "completed"
	result := reminderRecoveryResult(record, record.StartedAt, progress, terminal, s.now())
	audit := NewActionAuditStore(s.db)
	settleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if !terminal {
		if err := audit.MarkPending(settleCtx, record); err != nil {
			return nil, err
		}
		return result, nil
	}
	record.Status, record.FinishedAt, record.Result = "succeeded", result.FinishedAt, result
	if err := audit.Complete(settleCtx, record); err != nil {
		row, readErr := s.load(settleCtx, record)
		if readErr == nil {
			if prior, readErr := storedCurrentReminderResult(row); prior != nil && readErr == nil {
				return prior, nil
			}
		}
		return nil, err
	}
	return result, nil
}

func reminderRecoveryResult(record app.ActionAuditRecord, started time.Time, p currentReminderProgress, terminal bool, now time.Time) *app.ActionRunResult {
	status := app.ActionAuditStatusPendingReconciliation
	if terminal {
		status = "succeeded"
	}
	return &app.ActionRunResult{RequestID: record.RequestID, ActionID: currentReminderAction, Status: status, StartedAt: started, FinishedAt: now, Result: map[string]interface{}{"source_protocol": app.TransactionReminderSource, "phase": p.phase, "audit_terminal": terminal, "platform_confirmed": p.confirmed, "rejected": p.rejected, "suppressed": p.suppressed, "reviewed": p.reviewed, "pending": p.pending, "unknown": p.unknown, "mq_published": false}}
}

func (s *CurrentReminderRecovery) progress(ctx context.Context, orgID int64, req app.ReminderGapRecoveryRequest) (currentReminderProgress, error) {
	tx := s.db.WithContext(ctx).Begin(&sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if tx.Error != nil {
		return currentReminderProgress{}, tx.Error
	}
	defer func() { _ = tx.Rollback().Error }()
	return readCurrentReminderProgress(tx, orgID, req, false)
}

type currentReminderProgress struct {
	phase                                                       string
	confirmed, rejected, suppressed, reviewed, pending, unknown int
}

func readCurrentReminderProgress(db *gorm.DB, orgID int64, req app.ReminderGapRecoveryRequest, lock bool) (currentReminderProgress, error) {
	p := currentReminderProgress{phase: "missing_batch"}
	var batches []struct {
		OpeningEventID    string
		ScheduleRevision  uint32
		ReminderVersion   uint32
		AppID, TemplateID string
		RecipientCount    uint32
		State             string
	}
	query := db.Table("task_opened_reminder_batch").Where("org_id=? AND task_id=?", orgID, req.TaskID).Limit(2)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.Find(&batches).Error; err != nil {
		return p, err
	}
	var deliveries []struct {
		UserID, LoginIdentityID string
		OpeningEventID          string
		ScheduleRevision        uint32
		ReminderVersion         uint32
		AppID, TemplateID       string
		State                   notification.ReminderDeliveryState
		ExternalCallStartedAt   *time.Time
	}
	query = db.Table("task_opened_reminder_delivery").Where("org_id=? AND task_id=?", orgID, req.TaskID).Limit(21)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.Find(&deliveries).Error; err != nil {
		return p, err
	}
	if len(batches) == 0 && len(deliveries) == 0 {
		return p, nil
	}
	p.phase = "manual_required"
	if len(batches) != 1 {
		return p, nil
	}
	b := batches[0]
	if b.OpeningEventID != req.OpeningEventID || b.ScheduleRevision != req.ExpectedScheduleRevision || b.ReminderVersion != 1 || int(b.RecipientCount) != len(deliveries) || len(deliveries) > 20 {
		return p, nil
	}
	if b.State == "suppressed" && b.RecipientCount == 0 {
		p.phase = "completed"
		return p, nil
	}
	if b.State != "frozen" || b.RecipientCount == 0 || b.AppID == "" || b.TemplateID == "" {
		return p, nil
	}
	for _, d := range deliveries {
		if d.UserID == "" || d.LoginIdentityID == "" || d.OpeningEventID != b.OpeningEventID || d.ScheduleRevision != b.ScheduleRevision || d.ReminderVersion != b.ReminderVersion || d.AppID != b.AppID || d.TemplateID != b.TemplateID {
			return p, nil
		}
		switch d.State {
		case notification.ReminderConfirmed:
			p.confirmed++
		case notification.ReminderRejected:
			p.rejected++
		case notification.ReminderSuppressed:
			p.suppressed++
		case notification.ReminderReviewed:
			p.reviewed++
		case notification.ReminderPending, notification.ReminderClaimed:
			if d.ExternalCallStartedAt != nil {
				p.unknown++
			} else {
				p.pending++
			}
		case notification.ReminderSending, notification.ReminderManualRequired:
			p.unknown++
		default:
			return p, nil
		}
	}
	if p.unknown > 0 {
		return p, nil
	}
	if p.pending > 0 {
		p.phase = "pending"
	} else {
		p.phase = "completed"
	}
	return p, nil
}

var _ app.CurrentReminderGapRecovery = (*CurrentReminderRecovery)(nil)
