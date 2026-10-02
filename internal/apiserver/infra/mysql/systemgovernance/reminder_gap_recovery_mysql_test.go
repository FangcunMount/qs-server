package systemgovernance

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	notification "github.com/FangcunMount/qs-server/internal/apiserver/application/notification"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	plan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	reminderledger "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/notification"
	planinfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/plan"
	standard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/standardoutbox"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	sdkmysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// This is the SQL authorization boundary, not a substitute for broker-loss
// or WeChat delivery proof. Messages use the actual Task constructor, stager,
// original GORM transaction and fixed SDK claim/confirmation implementation.
func TestReminderGapRecoveryAtomicOriginalIdentityMySQL(t *testing.T) {
	dsn := os.Getenv("QS_REMINDER_GAP_MYSQL_DSN")
	if dsn == "" {
		if os.Getenv("QS_REMINDER_GAP_MYSQL_REQUIRED") == "1" {
			t.Fatal("disposable MySQL required")
		}
		t.Skip("disposable MySQL not configured")
	}
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{PrepareStmt: true})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(8)
	t.Cleanup(func() { _ = pool.Close() })
	var database string
	require.NoError(t, db.Raw("SELECT DATABASE()").Scan(&database).Error)
	require.True(t, strings.HasPrefix(database, "qs_m6_reminder_gap_test_"), "disposable database required")
	require.NoError(t, db.AutoMigrate(&planinfra.AssessmentTaskPO{}))
	require.NoError(t, db.Exec("ALTER TABLE assessment_task MODIFY open_at DATETIME(3) NULL").Error)
	_, err = pool.ExecContext(t.Context(), sdkmysql.Schema)
	require.NoError(t, err)
	for _, migration := range []string{"000048_add_system_governance_action_runs.up.sql", "000087_task_opened_reminder_delivery.up.sql", "000088_task_opened_reminder_batch.up.sql"} {
		raw, err := os.ReadFile("../../../../pkg/migration/migrations/mysql/" + migration)
		require.NoError(t, err)
		require.NoError(t, db.Exec(string(raw)).Error)
	}
	catalog, err := eventcatalog.Parse([]byte(`version: "1"
topics:
  task-lifecycle:
    name: qs.plan.task
events:
  task.opened.reminder.requested:
    topic: task-lifecycle
    delivery: durable_outbox
    aggregate: AssessmentTask
    domain: plan
    handler: task_opened_reminder_handler
`))
	require.NoError(t, err)
	stager, err := standard.NewStager(eventcatalog.NewCatalog(catalog), eventruntime.SourceAPIServer)
	require.NoError(t, err)
	repository := planinfra.NewTaskRepository(db)
	sdk, err := sdkmysql.New(pool)
	require.NoError(t, err)
	store := NewActionAuditStore(db)
	sequence := 0
	seed := func(openAt time.Time, confirmed bool) app.ReminderGapRecoveryRequest {
		t.Helper()
		sequence++
		task := plan.NewAssessmentTaskAt(plan.NewAssessmentPlanID(), sequence, 7, testee.NewID(99), "scale", openAt.Add(-time.Minute), openAt.Add(-time.Minute))
		require.NoError(t, plan.NewTaskLifecycle().OpenAt(t.Context(), task, "test-entry", "https://example.invalid/task", openAt))
		opened := plan.NewTaskOpenedEvent(task.GetID(), task.GetPlanID(), 7, task.GetTesteeID(), "https://example.invalid/task", openAt)
		reminder := plan.NewTaskOpenedReminderRequestedEvent(opened, task.GetScheduleRevision())
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
			ctx := hostmysql.WithTx(t.Context(), tx)
			if err := repository.Save(ctx, task); err != nil {
				return err
			}
			if !confirmed {
				return stager.StageAt(ctx, time.Now().Add(4*time.Hour), reminder)
			}
			return stager.Stage(ctx, reminder)
		}))
		if confirmed {
			claims, err := sdk.ClaimDue(t.Context(), 1, time.Minute)
			require.NoError(t, err)
			require.Len(t, claims, 1)
			require.Equal(t, reminder.EventID(), claims[0].Message.Input().ID)
			require.NoError(t, sdk.Confirm(t.Context(), claims[0]))
		}
		var version uint64
		require.NoError(t, db.Table("rm_outbox").Select("version").Where("message_id=?", reminder.EventID()).Scan(&version).Error)
		return app.ReminderGapRecoveryRequest{RequestID: fmt.Sprintf("recovery-%d", sequence), TaskID: task.GetID().String(), OpeningEventID: reminder.EventID(), ExpectedVersion: version,
			OpenedBefore: time.Now().Add(-10 * time.Second), EvidenceReference: "isolated-original-intent-review", Reason: "original intent confirmed; recipient responsibility missing", Confirm: true}
	}
	type snapshot struct {
		Payload, Fingerprint                []byte
		State                               string
		Version, AttemptCount, FailureCount uint64
		TransportConfirmedAt                *time.Time
		ManualReplayRequestID               string
	}
	read := func(req app.ReminderGapRecoveryRequest) snapshot {
		t.Helper()
		var row snapshot
		require.NoError(t, db.Table("rm_outbox").Where("message_id=?", req.OpeningEventID).Take(&row).Error)
		return row
	}

	t.Run("atomic audit rollback and lost response lookup", func(t *testing.T) {
		req := seed(time.Now().Add(-time.Minute), true)
		before := read(req)
		// Claim succeeds, but completing the audit fails after the SDK UPDATE.
		require.NoError(t, db.Exec("ALTER TABLE system_governance_action_runs ADD CONSTRAINT fail_completion CHECK (actor_user_id <> 12 OR status = 'running')").Error)
		_, err := store.AuthorizeReminderGap(t.Context(), 7, 12, req)
		require.Error(t, err)
		require.Equal(t, before, read(req), "audit failure must roll back the original-row requeue")
		_, found, err := store.ResolveReminderGap(t.Context(), 7, 12, req)
		require.NoError(t, err)
		require.False(t, found)
		result, err := store.AuthorizeReminderGap(t.Context(), 7, 11, req)
		require.NoError(t, err)
		require.Equal(t, true, result.Result["authorized"])
		after := read(req)
		require.Equal(t, "retry_wait", after.State)
		require.Equal(t, before.Version+1, after.Version)
		require.Equal(t, req.RequestID, after.ManualReplayRequestID)
		require.True(t, bytes.Equal(before.Payload, after.Payload))
		require.Equal(t, before.Fingerprint, after.Fingerprint)
		require.Equal(t, before.AttemptCount, after.AttemptCount)
		require.Equal(t, before.FailureCount, after.FailureCount)
		require.Equal(t, before.TransportConfirmedAt, after.TransportConfirmedAt)
		// Disabled writes still resolve the exact original receipt after EOF.
		readonly := app.NewFacade(app.FacadeDeps{ReminderGapRecovery: store}).(app.ReminderGapRecovery)
		_, err = readonly.AuthorizeReminderGap(t.Context(), 7, 11, req)
		require.Error(t, err)
		receipt, found, err := readonly.ResolveReminderGap(t.Context(), 7, 11, req)
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, result.RequestID, receipt.RequestID)
		retry, err := store.AuthorizeReminderGap(t.Context(), 7, 11, req)
		require.NoError(t, err)
		require.Equal(t, receipt.Result, retry.Result)
		require.Equal(t, after, read(req))
		_, _, err = readonly.ResolveReminderGap(t.Context(), 7, 13, req)
		require.ErrorIs(t, err, app.ErrActionAuditInputConflict)
		changed := req
		changed.Reason = "different review"
		_, _, err = readonly.ResolveReminderGap(t.Context(), 7, 11, changed)
		require.ErrorIs(t, err, app.ErrActionAuditInputConflict)
		_, found, err = readonly.ResolveReminderGap(t.Context(), 8, 11, req)
		require.NoError(t, err)
		require.False(t, found)
		// Do not leave this due row eligible when later fixtures claim their event.
		require.NoError(t, db.Table("rm_outbox").Where("message_id=?", req.OpeningEventID).Update("next_attempt_at", time.Now().Add(4*time.Hour)).Error)
	})

	t.Run("concurrent same request requeues once", func(t *testing.T) {
		req := seed(time.Now().Add(-time.Minute), true)
		before := read(req)
		start := make(chan struct{})
		errors := make(chan error, 2)
		var group sync.WaitGroup
		for range 2 {
			group.Add(1)
			go func() {
				defer group.Done()
				<-start
				_, err := store.AuthorizeReminderGap(context.Background(), 7, 11, req)
				errors <- err
			}()
		}
		close(start)
		group.Wait()
		close(errors)
		for err := range errors {
			require.NoError(t, err)
		}
		require.Equal(t, before.Version+1, read(req).Version)
		var count int64
		require.NoError(t, db.Model(&actionRunPO{}).Where("request_id=?", req.RequestID).Count(&count).Error)
		require.EqualValues(t, 1, count)
		require.NoError(t, db.Table("rm_outbox").Where("message_id=?", req.OpeningEventID).Update("next_attempt_at", time.Now().Add(4*time.Hour)).Error)
	})

	t.Run("first recipient batch in flight wins over recovery", func(t *testing.T) {
		req := seed(time.Now().Add(-time.Minute), true)
		before := read(req)
		tx := db.Begin()
		require.NoError(t, tx.Error)
		defer tx.Rollback()
		key := notification.ReminderBatchKey{OrgID: 7, TaskID: req.TaskID, OpeningEventID: req.OpeningEventID, ScheduleRevision: 1, ReminderVersion: 1}
		_, err := reminderledger.NewReminderBatchLedger(tx).FreezeRecipients(t.Context(), key, "app", "template",
			[]notification.ReminderRecipientIdentity{{UserID: "user", LoginIdentityID: "identity"}}, time.Now())
		require.NoError(t, err)
		type outcome struct {
			result *app.ActionRunResult
			err    error
		}
		finished := make(chan outcome, 1)
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		go func() { result, err := store.AuthorizeReminderGap(ctx, 7, 11, req); finished <- outcome{result, err} }()
		// Observe the actual InnoDB wait, rather than using a sleep to assume
		// the competing transaction has reached its absence check.
		require.Eventually(t, func() bool {
			var waiting int64
			err := db.Raw(`SELECT COUNT(*) FROM performance_schema.data_lock_waits w
JOIN performance_schema.data_locks l ON l.ENGINE_LOCK_ID=w.REQUESTING_ENGINE_LOCK_ID AND l.ENGINE=w.ENGINE
WHERE l.OBJECT_SCHEMA=? AND l.OBJECT_NAME='task_opened_reminder_batch'`, database).Scan(&waiting).Error
			return err == nil && waiting > 0
		}, 2*time.Second, 10*time.Millisecond, "recovery must reach and wait for the in-flight batch lock")
		require.NoError(t, tx.Commit().Error)
		result := <-finished
		require.NoError(t, result.err)
		require.Equal(t, "recipient_responsibility_exists", result.result.Result["code"])
		require.Equal(t, before, read(req))
		// A normal delivery already entering the external-call boundary must
		// retain its unknown receipt; gap authorization cannot reset it.
		deliveryKey := notification.ReminderDeliveryKey{OrgID: 7, TaskID: req.TaskID, OpeningEventID: req.OpeningEventID, ScheduleRevision: 1,
			LoginIdentityID: "identity", AppID: "app", TemplateID: "template", ReminderVersion: 1}
		ledger := reminderledger.NewReminderDeliveryLedger(db)
		token, claimed, err := ledger.Claim(t.Context(), deliveryKey, time.Minute, time.Now())
		require.NoError(t, err)
		require.True(t, claimed)
		started, err := ledger.BeginExternalCall(t.Context(), deliveryKey, token, time.Now())
		require.NoError(t, err)
		require.True(t, started)
		marked, err := ledger.MarkUnknown(t.Context(), deliveryKey, token, "response_lost", time.Now())
		require.NoError(t, err)
		require.True(t, marked)
		req.RequestID += "-after-unknown"
		result.result, result.err = store.AuthorizeReminderGap(t.Context(), 7, 11, req)
		require.NoError(t, result.err)
		require.Equal(t, "recipient_responsibility_exists", result.result.Result["code"])
		_, claimed, err = ledger.Claim(t.Context(), deliveryKey, time.Minute, time.Now().Add(2*time.Minute))
		require.NoError(t, err)
		require.False(t, claimed)
		require.Equal(t, before, read(req))
	})

	cases := []struct {
		name, code string
		openAgo    time.Duration
		pending    bool
		mutate     func(app.ReminderGapRecoveryRequest)
	}{
		{"terminal task", "task_not_opened", time.Minute, false, func(req app.ReminderGapRecoveryRequest) {
			require.NoError(t, db.Table("assessment_task").Where("id=?", req.TaskID).Update("status", "completed").Error)
		}},
		{"past reminder deadline", "reminder_window_elapsed", 2 * time.Hour, false, nil},
		{"changed opening", "opening_changed", time.Minute, false, func(req app.ReminderGapRecoveryRequest) {
			require.NoError(t, db.Table("assessment_task").Where("id=?", req.TaskID).Update("schedule_revision", 2).Error)
		}},
		{"unconfirmed intent", "delivery_pending_or_unconfirmed", time.Minute, true, nil},
		{"changed fingerprint", "outbox_fingerprint_mismatch", time.Minute, false, func(req app.ReminderGapRecoveryRequest) {
			require.NoError(t, db.Table("rm_outbox").Where("message_id=?", req.OpeningEventID).Update("fingerprint", bytes.Repeat([]byte{0}, 32)).Error)
		}},
		{"orphan unknown delivery", "recipient_responsibility_exists", time.Minute, false, func(req app.ReminderGapRecoveryRequest) {
			require.NoError(t, db.Exec(`INSERT INTO task_opened_reminder_delivery (org_id,task_id,opening_event_id,schedule_revision,user_id,login_identity_id,app_id,template_id,reminder_version,state,created_at,updated_at) VALUES (7,?, ?,1,'user','identity','app','template',1,'manual_required',NOW(6),NOW(6))`, req.TaskID, req.OpeningEventID).Error)
		}},
		{"frozen empty batch", "recipient_responsibility_exists", time.Minute, false, func(req app.ReminderGapRecoveryRequest) {
			require.NoError(t, db.Exec(`INSERT INTO task_opened_reminder_batch (org_id,task_id,opening_event_id,schedule_revision,reminder_version,app_id,template_id,recipient_count,state,created_at,updated_at) VALUES (7,?,?,1,1,'app','template',0,'frozen',NOW(6),NOW(6))`, req.TaskID, req.OpeningEventID).Error)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := seed(time.Now().Add(-tc.openAgo), !tc.pending)
			if tc.pending {
				req.ExpectedVersion = 1
			} // Version mismatch is separately fenced before state.
			if tc.pending {
				require.NoError(t, db.Table("rm_outbox").Where("message_id=?", req.OpeningEventID).Update("version", 1).Error)
			}
			if tc.mutate != nil {
				tc.mutate(req)
			}
			before := read(req)
			result, err := store.AuthorizeReminderGap(t.Context(), 7, 11, req)
			require.NoError(t, err)
			require.Equal(t, false, result.Result["authorized"])
			require.Equal(t, tc.code, result.Result["code"])
			require.Equal(t, before, read(req))
			_, found, err := store.ResolveReminderGap(t.Context(), 7, 11, req)
			require.NoError(t, err)
			require.True(t, found, "denial must also be durably auditable")
		})
	}
}
