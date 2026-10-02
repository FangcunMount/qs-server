package systemgovernance

import (
	"os"
	"strings"
	"testing"
	"time"

	notification "github.com/FangcunMount/qs-server/internal/apiserver/application/notification"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	ledger "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/notification"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestReminderResolutionAtomicAndNoResendMySQL(t *testing.T) {
	dsn := os.Getenv("QS_REMINDER_RESOLUTION_MYSQL_DSN")
	if dsn == "" {
		if os.Getenv("QS_REMINDER_RESOLUTION_MYSQL_REQUIRED") == "1" {
			t.Fatal("isolated MySQL required")
		}
		t.Skip("isolated MySQL not configured")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = pool.Close() })
	var name string
	require.NoError(t, db.Raw("SELECT DATABASE()").Scan(&name).Error)
	require.True(t, strings.HasPrefix(name, "qs_m5_reminder_resolution_test_"))
	for _, path := range []string{"000048_add_system_governance_action_runs.up.sql", "000087_task_opened_reminder_delivery.up.sql"} {
		ddl, err := os.ReadFile("../../../../pkg/migration/migrations/mysql/" + path)
		require.NoError(t, err)
		require.NoError(t, db.Exec(strings.Replace(string(ddl), "CREATE TABLE", "CREATE TEMPORARY TABLE", 1)).Error)
	}
	now := time.Date(2026, 10, 1, 14, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	require.NoError(t, db.Exec(`INSERT INTO task_opened_reminder_delivery
 (id,org_id,task_id,opening_event_id,schedule_revision,user_id,login_identity_id,app_id,template_id,reminder_version,state,external_call_started_at,resolution_code,created_at,updated_at)
 VALUES (1,7,'task-1','event-1',1,'user-1','identity-1','app-1','template-1',1,'manual_required',?,'response_lost',?,?)`, now, now, now).Error)
	req := app.ReminderResolutionRequest{RequestID: "review-1", DeliveryID: 1, TaskID: "task-1", OpeningEventID: "event-1", ExpectedUpdatedAt: now, Finding: "unknown_no_resend", EvidenceReference: "incident-review-1", Reason: "platform outcome cannot be established; never resend", Confirm: true}
	store := NewActionAuditStore(db)
	_, err = store.ResolveReminder(t.Context(), 8, 11, req)
	require.Error(t, err)
	stale := req
	stale.ExpectedUpdatedAt = now.Add(time.Second)
	_, err = store.ResolveReminder(t.Context(), 7, 11, stale)
	require.Error(t, err)
	wrong := req
	wrong.OpeningEventID = "other"
	_, err = store.ResolveReminder(t.Context(), 7, 11, wrong)
	require.Error(t, err)
	var audits int64
	require.NoError(t, db.Model(&actionRunPO{}).Count(&audits).Error)
	require.Zero(t, audits)
	// A failed audit insert must roll back the ledger change.
	require.NoError(t, db.Exec("ALTER TABLE system_governance_action_runs ADD CONSTRAINT test_fail_audit CHECK (actor_user_id <> 12)").Error)
	_, err = store.ResolveReminder(t.Context(), 7, 12, req)
	require.Error(t, err)
	var row reminderResolutionRow
	require.NoError(t, db.First(&row, 1).Error)
	require.Equal(t, "manual_required", row.State)
	result, err := store.ResolveReminder(t.Context(), 7, 11, req)
	require.NoError(t, err)
	require.Equal(t, "unknown_no_resend", result.Result["finding"])
	require.Equal(t, false, result.Result["automatic_resend"])
	require.NoError(t, db.First(&row, 1).Error)
	require.Equal(t, "reviewed", row.State)
	require.Equal(t, "manual_unknown_no_resend", row.ResolutionCode)
	retry, err := store.ResolveReminder(t.Context(), 7, 11, req)
	require.NoError(t, err)
	require.Equal(t, result.RequestID, retry.RequestID)
	receipt, err := store.LoadReminderResolution(t.Context(), 7, 11, req.RequestID)
	require.NoError(t, err)
	require.Equal(t, result.RequestID, receipt.RequestID)
	_, err = store.LoadReminderResolution(t.Context(), 8, 11, req.RequestID)
	require.Error(t, err)
	_, err = store.LoadReminderResolution(t.Context(), 7, 13, req.RequestID)
	require.Error(t, err)
	changed := req
	changed.Reason = "changed"
	_, err = store.ResolveReminder(t.Context(), 7, 11, changed)
	require.Error(t, err)
	_, err = store.ResolveReminder(t.Context(), 7, 13, req)
	require.Error(t, err)
	second := req
	second.RequestID = "review-2"
	_, err = store.ResolveReminder(t.Context(), 7, 11, second)
	require.Error(t, err)
	require.NoError(t, db.Model(&actionRunPO{}).Count(&audits).Error)
	require.EqualValues(t, 1, audits)
	// Drive the real sender ledger: reviewed can never be claimed again.
	key := notification.ReminderDeliveryKey{OrgID: 7, TaskID: "task-1", OpeningEventID: "event-1", ScheduleRevision: 1, LoginIdentityID: "identity-1", AppID: "app-1", TemplateID: "template-1", ReminderVersion: 1}
	_, claimed, err := ledger.NewReminderDeliveryLedger(db).Claim(t.Context(), key, time.Minute, now.Add(time.Hour))
	require.NoError(t, err)
	require.False(t, claimed)
	// Exercise both settlement orders with the real sender and real SQL rows.
	sender := ledger.NewReminderDeliveryLedger(db)
	for _, messageID := range []string{"late-platform-id", ""} {
		suffix := "with-id"
		if messageID == "" {
			suffix = "without-id"
		}
		lateKey := key
		lateKey.TaskID = "late-" + suffix
		lateKey.OpeningEventID = "late-event-" + suffix
		_, err := sender.EnsurePending(t.Context(), lateKey, "user-1", now)
		require.NoError(t, err)
		token, claimed, err := sender.Claim(t.Context(), lateKey, time.Minute, now)
		require.NoError(t, err)
		require.True(t, claimed)
		started, err := sender.BeginExternalCall(t.Context(), lateKey, token, now.Add(time.Second))
		require.NoError(t, err)
		require.True(t, started)
		var current reminderResolutionRow
		require.NoError(t, db.Where("task_id = ?", lateKey.TaskID).Take(&current).Error)
		review := app.ReminderResolutionRequest{RequestID: "review-" + suffix, DeliveryID: current.ID, TaskID: lateKey.TaskID, OpeningEventID: lateKey.OpeningEventID, ExpectedUpdatedAt: current.UpdatedAt, Finding: "unknown_no_resend", EvidenceReference: "interrupted-call-review", Reason: "original call may finish; no resend", Confirm: true}
		_, err = store.ResolveReminder(t.Context(), 7, 11, review)
		require.Error(t, err) // no acknowledgment
		review.AcknowledgeOriginalCallMayComplete = true
		invalid := review
		invalid.Finding = "recipient_received"
		_, err = store.ResolveReminder(t.Context(), 7, 11, invalid)
		require.Error(t, err) // cannot invent success for an in-flight call
		receipt, err := store.ResolveReminder(t.Context(), 7, 11, review)
		require.NoError(t, err)
		require.Equal(t, true, receipt.Result["original_call_may_complete"])
		confirmed, err := sender.Confirm(t.Context(), lateKey, "wrong-token", messageID, now.Add(2*time.Second))
		require.NoError(t, err)
		require.False(t, confirmed)
		confirmed, err = sender.Confirm(t.Context(), lateKey, token, messageID, now.Add(3*time.Second))
		require.NoError(t, err)
		require.True(t, confirmed)
		delivery, err := sender.Read(t.Context(), lateKey)
		require.NoError(t, err)
		require.Equal(t, notification.ReminderReviewed, delivery.State)
		require.Equal(t, messageID, delivery.PlatformMessageID)
		require.Contains(t, delivery.ResolutionCode, "late_")
		confirmed, err = sender.Confirm(t.Context(), lateKey, token, messageID, now.Add(4*time.Second))
		require.NoError(t, err)
		require.False(t, confirmed)
		_, claimed, err = sender.Claim(t.Context(), lateKey, time.Minute, now.Add(time.Hour))
		require.NoError(t, err)
		require.False(t, claimed)
		replay, err := store.ResolveReminder(t.Context(), 7, 11, review)
		require.NoError(t, err)
		require.Equal(t, receipt.RequestID, replay.RequestID)
		require.Equal(t, "unknown_no_resend", replay.Result["finding"])
	}
	beforeKey := key
	beforeKey.TaskID = "platform-first"
	beforeKey.OpeningEventID = "platform-first-event"
	_, err = sender.EnsurePending(t.Context(), beforeKey, "user-1", now)
	require.NoError(t, err)
	token, claimed, err := sender.Claim(t.Context(), beforeKey, time.Minute, now)
	require.NoError(t, err)
	require.True(t, claimed)
	started, err := sender.BeginExternalCall(t.Context(), beforeKey, token, now.Add(time.Second))
	require.NoError(t, err)
	require.True(t, started)
	var before reminderResolutionRow
	require.NoError(t, db.Where("task_id = ?", beforeKey.TaskID).Take(&before).Error)
	confirmed, err := sender.Confirm(t.Context(), beforeKey, token, "first-response", now.Add(2*time.Second))
	require.NoError(t, err)
	require.True(t, confirmed)
	_, err = store.ResolveReminder(t.Context(), 7, 11, app.ReminderResolutionRequest{RequestID: "review-platform-first", DeliveryID: before.ID, TaskID: beforeKey.TaskID, OpeningEventID: beforeKey.OpeningEventID, ExpectedUpdatedAt: before.UpdatedAt, Finding: "unknown_no_resend", EvidenceReference: "stale-view", Reason: "must reject stale view", Confirm: true, AcknowledgeOriginalCallMayComplete: true})
	require.Error(t, err)
	require.NoError(t, db.Model(&actionRunPO{}).Where("request_id = ?", "review-platform-first").Count(&audits).Error)
	require.Zero(t, audits)

	rejectedKey := key
	rejectedKey.TaskID, rejectedKey.OpeningEventID = "rejected-task", "rejected-event"
	_, err = sender.EnsurePending(t.Context(), rejectedKey, "user-1", now)
	require.NoError(t, err)
	token, claimed, err = sender.Claim(t.Context(), rejectedKey, time.Minute, now)
	require.NoError(t, err)
	require.True(t, claimed)
	started, err = sender.BeginExternalCall(t.Context(), rejectedKey, token, now.Add(time.Second))
	require.NoError(t, err)
	require.True(t, started)
	rejectedRecorded, err := sender.Reject(t.Context(), rejectedKey, token, 43101, now.Add(2*time.Second))
	require.NoError(t, err)
	require.True(t, rejectedRecorded)
	var rejectedRow reminderResolutionRow
	require.NoError(t, db.Where("task_id = ?", rejectedKey.TaskID).Take(&rejectedRow).Error)
	rejectionReview := app.ReminderResolutionRequest{RequestID: "review-rejected", DeliveryID: rejectedRow.ID, TaskID: rejectedRow.TaskID, OpeningEventID: rejectedRow.OpeningEventID, ExpectedUpdatedAt: rejectedRow.UpdatedAt, Finding: "unknown_no_resend", EvidenceReference: "recorded-platform-errcode", Reason: "acknowledge explicit rejection without resending", Confirm: true}
	_, err = store.ResolveReminder(t.Context(), 7, 11, rejectionReview)
	require.Error(t, err, "must not relabel an explicit platform error")
	rejectionReview.Finding = "platform_rejected"
	rejectionReceipt, err := store.ResolveReminder(t.Context(), 7, 11, rejectionReview)
	require.NoError(t, err)
	require.Equal(t, "platform_rejected_43101", rejectionReceipt.Result["original_resolution_code"])
	require.NoError(t, db.Where("id = ?", rejectedRow.ID).Take(&rejectedRow).Error)
	require.Equal(t, "reviewed", rejectedRow.State)
	require.Equal(t, "manual_platform_rejected_43101", rejectedRow.ResolutionCode)
	_, claimed, err = sender.Claim(t.Context(), rejectedKey, time.Minute, now.Add(time.Hour))
	require.NoError(t, err)
	require.False(t, claimed)
	replayReceipt, err := store.ResolveReminder(t.Context(), 7, 11, rejectionReview)
	require.NoError(t, err)
	require.Equal(t, rejectionReceipt.RequestID, replayReceipt.RequestID)

}
