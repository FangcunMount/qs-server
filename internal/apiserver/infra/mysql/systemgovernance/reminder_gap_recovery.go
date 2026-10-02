package systemgovernance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	notification "github.com/FangcunMount/qs-server/internal/apiserver/application/notification"
	planapp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	plan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/reliable-messaging/message"
	sdkmysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const reminderGapActionID = "notifications.recover_missing_reminder_batch"

type reminderGapTask struct {
	ID, PlanID, TesteeID uint64
	OrgID                int64
	Status               string
	ScheduleRevision     uint32
	OpenAt, ExpireAt     *time.Time
	EntryURL             string
}

type reminderGapOutbox struct {
	id, version uint64
	in          message.Input
	fingerprint []byte
	state       string
	confirmed   bool
}

func reminderGapRecord(orgID int64, actorID uint64, req app.ReminderGapRecoveryRequest) (app.ActionAuditRecord, error) {
	if err := app.ValidateReminderGapRecovery(orgID, actorID, req); err != nil {
		return app.ActionAuditRecord{}, err
	}
	req.OpenedBefore = req.OpenedBefore.In(time.FixedZone("UTC+8", 8*3600))
	raw, err := json.Marshal(req)
	if err != nil {
		return app.ActionAuditRecord{}, err
	}
	var input map[string]interface{}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&input); err != nil {
		return app.ActionAuditRecord{}, err
	}
	return app.ActionAuditRecord{RequestID: req.RequestID, ActionID: reminderGapActionID, OrgID: orgID,
		ActorUserID: actorID, Component: "notifications", TargetInstance: req.TaskID, Input: input, StartedAt: time.Now()}, nil
}

// AuthorizeReminderGap commits the original-row requeue and governance receipt
// in one host transaction. It has no publisher, WeChat sender or identity reader.
func (s *ActionAuditStore) AuthorizeReminderGap(ctx context.Context, orgID int64, actorID uint64, req app.ReminderGapRecoveryRequest) (*app.ActionRunResult, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("host governance database required")
	}
	record, err := reminderGapRecord(orgID, actorID, req)
	if err != nil {
		return nil, err
	}
	var result *app.ActionRunResult
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		audit := NewActionAuditStore(tx)
		prior, claimed, err := audit.Claim(ctx, record)
		if err != nil {
			return err
		}
		if !claimed {
			if prior == nil || prior.Result == nil || prior.Error != nil {
				return errors.New("original reminder recovery receipt is not terminal")
			}
			result = prior.Result
			return nil
		}
		decision, err := recoverMissingReminder(ctx, tx, orgID, req)
		if err != nil {
			return err
		}
		now := time.Now().In(time.FixedZone("UTC+8", 8*3600))
		result = &app.ActionRunResult{RequestID: req.RequestID, ActionID: reminderGapActionID, Status: "succeeded",
			StartedAt: record.StartedAt, FinishedAt: now, Result: decision}
		record.Status, record.FinishedAt, record.Result = "succeeded", now, result
		return audit.Complete(ctx, record)
	})
	return result, err
}

// ResolveReminderGap only reads the same actor's exact committed request.
func (s *ActionAuditStore) ResolveReminderGap(ctx context.Context, orgID int64, actorID uint64, req app.ReminderGapRecoveryRequest) (*app.ActionRunResult, bool, error) {
	if s == nil || s.db == nil {
		return nil, false, errors.New("host governance database required")
	}
	record, err := reminderGapRecord(orgID, actorID, req)
	if err != nil {
		return nil, false, err
	}
	var row actionRunPO
	err = s.db.WithContext(ctx).Where("org_id=? AND request_id=?", orgID, req.RequestID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	raw, err := json.Marshal(record.Input)
	if err != nil {
		return nil, true, err
	}
	if row.ActionID != reminderGapActionID || row.ActorUserID != actorID || row.TargetInstance != req.TaskID ||
		row.Component != "notifications" || !equalAuditJSON(row.InputJSON, string(raw)) {
		return nil, true, app.ErrActionAuditInputConflict
	}
	replay, err := decodeActionAuditReplay(row.ResultJSON)
	if err != nil || row.Status != "succeeded" || replay == nil || replay.Result == nil || replay.Error != nil {
		return nil, true, errors.New("original reminder recovery receipt is not terminal")
	}
	return replay.Result, true, nil
}

func recoverMissingReminder(ctx context.Context, tx *gorm.DB, orgID int64, req app.ReminderGapRecoveryRequest) (map[string]interface{}, error) {
	deny := func(reason string) (map[string]interface{}, error) {
		return map[string]interface{}{"authorized": false, "code": reason, "task_id": req.TaskID, "opening_event_id": req.OpeningEventID}, nil
	}
	var task reminderGapTask
	err := tx.Table("assessment_task").Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id=? AND org_id=? AND deleted_at IS NULL", req.TaskID, orgID).Take(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return deny("task_missing")
	}
	if err != nil {
		return nil, err
	}
	rows, err := tx.Raw(`SELECT id,version,producer,message_id,destination,event_type,schema_version,scope,
content_type,occurred_at,payload,fingerprint,state,transport_confirmed_at IS NOT NULL
FROM rm_outbox WHERE message_id=? LIMIT 2 FOR UPDATE`, req.OpeningEventID).Rows()
	if err != nil {
		return nil, err
	}
	var found []reminderGapOutbox
	for rows.Next() {
		var o reminderGapOutbox
		if err := rows.Scan(&o.id, &o.version, &o.in.Producer, &o.in.ID, &o.in.Destination, &o.in.EventType,
			&o.in.SchemaVersion, &o.in.Scope, &o.in.ContentType, &o.in.OccurredAt, &o.in.Payload, &o.fingerprint, &o.state, &o.confirmed); err != nil {
			_ = rows.Close()
			return nil, err
		}
		found = append(found, o)
	}
	err = rows.Err()
	closeErr := rows.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(found) != 1 {
		return deny("outbox_missing_or_ambiguous")
	}
	o := found[0]
	if o.version != req.ExpectedVersion {
		return deny("version_conflict")
	}
	if o.state != "published" || !o.confirmed {
		return deny("delivery_pending_or_unconfirmed")
	}
	intent, reason := originalReminderIntent(o, orgID, task, req.OpeningEventID)
	if reason != "" {
		return deny(reason)
	}
	if task.OpenAt == nil || task.OpenAt.After(req.OpenedBefore) {
		return deny("within_grace")
	}
	decision, err := notification.EvaluateTaskOpenedReminder(intent, &planapp.TaskReminderState{
		OrgID: orgID, TaskID: req.TaskID, TesteeID: strconv.FormatUint(task.TesteeID, 10),
		Status: plan.TaskStatus(task.Status), ScheduleRevision: task.ScheduleRevision,
		OpenAt: task.OpenAt, ExpireAt: task.ExpireAt, EntryURL: task.EntryURL}, time.Now())
	if err != nil {
		return deny("task_facts_incomplete")
	}
	if decision.SuppressCode != "" {
		return deny(decision.SuppressCode)
	}
	// Any prior batch or orphan delivery makes this a different recovery case.
	// Locked prefix ranges also serialize a first, still-in-flight batch freeze.
	for _, table := range []string{"task_opened_reminder_batch", "task_opened_reminder_delivery"} {
		var id uint64
		query := tx.Table(table).Select("id").Clauses(clause.Locking{Strength: "UPDATE"}).Where("org_id=? AND task_id=?", orgID, req.TaskID).Limit(1).Scan(&id)
		if query.Error != nil {
			return nil, query.Error
		}
		if query.RowsAffected != 0 {
			return deny("recipient_responsibility_exists")
		}
	}
	appender, err := sdkmysql.BindGORM(tx)
	if err != nil {
		return nil, err
	}
	var fingerprint [32]byte
	copy(fingerprint[:], o.fingerprint)
	after, err := appender.RequeueConfirmed(ctx, sdkmysql.ConfirmedRequeue{RecordID: strconv.FormatUint(o.id, 10), ExpectedVersion: o.version, ExpectedFingerprint: fingerprint, RequestID: req.RequestID})
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"authorized": true, "code": "authorized", "task_id": req.TaskID, "opening_event_id": req.OpeningEventID,
		"outbox_version_before": o.version, "outbox_version_after": after, "deadline": intent.OpenAt.Add(notification.TaskOpenedReminderWindow), "direct_send": false}, nil
}

func originalReminderIntent(o reminderGapOutbox, orgID int64, task reminderGapTask, eventID string) (notification.TaskOpenedReminderIntent, string) {
	bad := func(reason string) (notification.TaskOpenedReminderIntent, string) {
		return notification.TaskOpenedReminderIntent{}, reason
	}
	if o.in.Producer != "qs-server" || o.in.ID != eventID || o.in.Scope != fmt.Sprintf("org:%d", orgID) ||
		o.in.Destination != "qs.plan.task" || o.in.EventType != plan.EventTypeTaskOpenedReminderRequested || o.in.SchemaVersion != "v1" || o.in.ContentType != "application/json" {
		return bad("outbox_identity_mismatch")
	}
	m, err := message.New(o.in)
	fingerprint := m.Fingerprint()
	if err != nil || !bytes.Equal(o.fingerprint, fingerprint[:]) {
		return bad("outbox_fingerprint_mismatch")
	}
	wire, recognized, err := legacy.Decode(o.in.Payload)
	if err != nil || !recognized || wire.UUID != eventID {
		return bad("wire_identity_mismatch")
	}
	canonical, err := legacy.Encode(wire, legacy.Revision2)
	if err != nil || !bytes.Equal(canonical, o.in.Payload) {
		return bad("wire_revision_mismatch")
	}
	entity, err := domainwire.DecodeEnvelope(wire.Payload)
	taskID := strconv.FormatUint(task.ID, 10)
	if err != nil || entity.ID != eventID || entity.EventType != o.in.EventType || entity.AggregateType != "AssessmentTask" || entity.AggregateID != taskID ||
		wire.Metadata["event_type"] != entity.EventType || wire.Metadata["aggregate_type"] != entity.AggregateType || wire.Metadata["aggregate_id"] != entity.AggregateID || wire.Metadata["source"] == "" {
		return bad("domain_identity_mismatch")
	}
	occurred, err := time.Parse(time.RFC3339Nano, o.in.OccurredAt)
	if err != nil || !entity.OccurredAt.Equal(occurred) || wire.Metadata["occurred_at"] != entity.OccurredAt.Format(domainwire.OccurredAtLayout) {
		return bad("occurred_at_mismatch")
	}
	var data eventpayload.TaskOpenedReminderRequestedData
	if err := json.Unmarshal(entity.Data, &data); err != nil || data.OrgID != orgID || data.TaskID != taskID || data.PlanID != strconv.FormatUint(task.PlanID, 10) || data.TesteeID != strconv.FormatUint(task.TesteeID, 10) {
		return bad("frozen_payload_mismatch")
	}
	return notification.TaskOpenedReminderIntent{OrgID: orgID, TaskID: taskID, TesteeID: data.TesteeID, ScheduleRevision: data.ScheduleRevision, OpenAt: data.OpenAt}, ""
}

var _ app.ReminderGapRecovery = (*ActionAuditStore)(nil)
