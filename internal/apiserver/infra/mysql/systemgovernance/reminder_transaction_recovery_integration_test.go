//go:build integration

package systemgovernance

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	baseerrors "github.com/FangcunMount/component-base/pkg/errors"
	notification "github.com/FangcunMount/qs-server/internal/apiserver/application/notification"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	ledger "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/notification"
	planrepo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/plan"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/taskopen"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	drivermysql "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type currentReminderEffectFixture struct {
	db                *gorm.DB
	mode              string
	calls, boundaries atomic.Int64
}

func (s *currentReminderEffectFixture) ProcessTaskOpenedReminder(ctx context.Context, req notification.TaskOpenedReminderRequest) error {
	s.calls.Add(1)
	if s.mode == "noop" {
		return nil
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	key := notification.ReminderBatchKey{OrgID: req.Intent.OrgID, TaskID: req.Intent.TaskID, OpeningEventID: req.OpeningEventID, ScheduleRevision: req.Intent.ScheduleRevision, ReminderVersion: 1}
	_, err := ledger.NewReminderBatchLedger(s.db).FreezeRecipients(ctx, key, "owned-app", "owned-template", []notification.ReminderRecipientIdentity{{UserID: "owned-user", LoginIdentityID: "owned-identity"}}, now)
	if err != nil {
		return err
	}
	if s.mode == "freeze-only" {
		return errors.New("controlled process loss after freeze")
	}
	delivery := notification.ReminderDeliveryKey{OrgID: key.OrgID, TaskID: key.TaskID, OpeningEventID: key.OpeningEventID, ScheduleRevision: key.ScheduleRevision, ReminderVersion: 1, LoginIdentityID: "owned-identity", AppID: "owned-app", TemplateID: "owned-template"}
	store := ledger.NewReminderDeliveryLedger(s.db)
	token, claimed, err := store.Claim(ctx, delivery, time.Minute, now)
	if err != nil || !claimed {
		return err
	}
	started, err := store.BeginExternalCall(ctx, delivery, token, now)
	if err != nil || !started {
		return err
	}
	s.boundaries.Add(1)
	if s.mode == "unknown" {
		_, err = store.MarkUnknown(ctx, delivery, token, "controlled_response_loss", now)
		return err
	}
	_, err = store.Confirm(ctx, delivery, token, "owned-platform-receipt", now)
	return err
}

func currentReminderNativeFixture(t *testing.T) (context.Context, *gorm.DB, *CurrentReminderRecovery, *currentReminderEffectFixture, app.ReminderGapRecoveryRequest) {
	t.Helper()
	cfg, err := drivermysql.ParseDSN(os.Getenv("QS_REMINDER_TX_MYSQL_DSN"))
	require.NoError(t, err)
	require.True(t, cfg.Net == "tcp" && strings.HasPrefix(cfg.Addr, "127.0.0.1:"), "owned disposable MySQL required")
	cfg.MultiStatements = true
	admin, err := gorm.Open(gormmysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	adminSQL, err := admin.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, adminSQL.Close()) })
	name := "qs_reminder_recovery_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, admin.Exec("CREATE DATABASE `"+name+"`").Error)
	t.Cleanup(func() { require.NoError(t, admin.Exec("DROP DATABASE `"+name+"`").Error) })
	cfg.DBName = name
	db, err := gorm.Open(gormmysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	require.NoError(t, db.WithContext(ctx).AutoMigrate(&planrepo.AssessmentTaskPO{}))
	require.NoError(t, db.Exec(`CREATE TABLE system_governance_action_runs(
id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,request_id VARCHAR(64) NOT NULL,action_id VARCHAR(128) NOT NULL,org_id BIGINT NOT NULL,actor_user_id BIGINT UNSIGNED NOT NULL,component VARCHAR(64) NOT NULL,target_instance VARCHAR(255) NOT NULL,input_json JSON NOT NULL,status VARCHAR(32) NOT NULL,result_json JSON NOT NULL,started_at DATETIME(3) NOT NULL,finished_at DATETIME(3) NULL,created_at DATETIME(3) NOT NULL,updated_at DATETIME(3) NOT NULL,UNIQUE KEY uk_request(org_id,request_id));
CREATE TABLE task_opened_reminder_batch(
id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,org_id BIGINT NOT NULL,task_id VARCHAR(64) NOT NULL,opening_event_id VARCHAR(64) NOT NULL,schedule_revision INT UNSIGNED NOT NULL,reminder_version INT UNSIGNED NOT NULL,app_id VARCHAR(64) NOT NULL,template_id VARCHAR(128) NOT NULL,recipient_count INT UNSIGNED NOT NULL,state VARCHAR(32) NOT NULL,resolution_code VARCHAR(128) NOT NULL,created_at DATETIME(3) NOT NULL,updated_at DATETIME(3) NOT NULL,UNIQUE KEY uk_batch(org_id,task_id,opening_event_id,schedule_revision,reminder_version));
CREATE TABLE task_opened_reminder_delivery(
id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,org_id BIGINT NOT NULL,task_id VARCHAR(64) NOT NULL,opening_event_id VARCHAR(64) NOT NULL,schedule_revision INT UNSIGNED NOT NULL,user_id VARCHAR(64) NOT NULL,login_identity_id VARCHAR(64) NOT NULL,app_id VARCHAR(64) NOT NULL,template_id VARCHAR(128) NOT NULL,reminder_version INT UNSIGNED NOT NULL,state VARCHAR(32) NOT NULL,claim_token VARCHAR(64) NULL,lease_until DATETIME(3) NULL,external_call_started_at DATETIME(3) NULL,platform_message_id VARCHAR(64) NULL,resolution_code VARCHAR(128) NOT NULL DEFAULT '',created_at DATETIME(3) NOT NULL,updated_at DATETIME(3) NOT NULL,UNIQUE KEY uk_delivery(org_id,task_id,opening_event_id,schedule_revision,login_identity_id,app_id,template_id,reminder_version))`).Error)
	at := time.Now().UTC().Add(-2 * time.Minute).Truncate(time.Millisecond)
	expires := at.Add(2 * time.Hour)
	task := planrepo.AssessmentTaskPO{OrgID: 7, PlanID: 200, TesteeID: 300, PlannedAt: at.Add(-time.Minute), ScheduleRevision: 1, Status: "opened", OpenAt: &at, ExpireAt: &expires, EntryURL: "private-owned-entry"}
	task.ID = meta.FromUint64(100)
	require.NoError(t, db.Create(&task).Error)
	effect := &currentReminderEffectFixture{db: db, mode: "complete"}
	store := NewCurrentReminderRecovery(db, effect, nil)
	o := taskopen.RecoveryOpening{Task: task, Request: notification.TaskOpenedReminderRequest{OpeningEventID: "original-opening", Intent: notification.TaskOpenedReminderIntent{OrgID: 7, TaskID: "100", TesteeID: "300", ScheduleRevision: 1, OpenAt: at}}, Fingerprint: strings.Repeat("ab", 32)}
	store.capture = func(context.Context, int64, uint64, uint32, time.Time) (taskopen.RecoveryOpening, error) {
		return o, nil
	}
	req := app.ReminderGapRecoveryRequest{RequestID: "original-recovery", TaskID: "100", OpeningEventID: "original-opening", OpenedBefore: at.Add(time.Minute), EvidenceReference: "owned-native-fixture", Reason: "controlled original recovery", Confirm: true, SourceProtocol: app.TransactionReminderSource, ExpectedScheduleRevision: 1, SourceFingerprint: o.Fingerprint}
	return ctx, db, store, effect, req
}

func TestCurrentReminderRecoveryKeepsOriginalRequestAndKnownReceipt(t *testing.T) {
	ctx, db, store, effect, req := currentReminderNativeFixture(t)
	result, err := store.AuthorizeReminderGap(ctx, 7, 11, req)
	require.NoError(t, err)
	require.Equal(t, "succeeded", result.Status)
	require.Equal(t, true, result.Result["audit_terminal"])
	require.EqualValues(t, 1, effect.boundaries.Load())
	// Reconstruct the coordinator as after restart; no new effect on exact replay.
	resumed := NewCurrentReminderRecovery(db, effect, nil)
	resumed.capture = store.capture
	_, err = resumed.ResumeReminderGap(ctx, 7, 11, req)
	require.NoError(t, err)
	_, err = resumed.AuthorizeReminderGap(ctx, 7, 11, req)
	require.NoError(t, err)
	require.EqualValues(t, 1, effect.calls.Load())
	_, found, err := resumed.ResolveReminderGap(ctx, 7, 11, req)
	require.NoError(t, err)
	require.True(t, found)
	other := req
	other.RequestID = "cannot-bypass-original"
	_, err = resumed.AuthorizeReminderGap(ctx, 7, 11, other)
	require.Error(t, err)
	require.EqualValues(t, 1, effect.boundaries.Load())
	require.False(t, db.Migrator().HasTable("rm_outbox"))
}

func TestCurrentReminderRecoveryNilReturnIsNotCompletion(t *testing.T) {
	ctx, _, store, effect, req := currentReminderNativeFixture(t)
	effect.mode = "noop"
	result, err := store.AuthorizeReminderGap(ctx, 7, 11, req)
	require.NoError(t, err)
	require.Equal(t, app.ActionAuditStatusPendingReconciliation, result.Status)
	require.Equal(t, false, result.Result["audit_terminal"])
	effect.mode = "complete"
	result, err = store.ResumeReminderGap(ctx, 7, 11, req)
	require.NoError(t, err)
	require.Equal(t, "succeeded", result.Status)
	require.EqualValues(t, 1, effect.boundaries.Load())
}

func TestCurrentReminderRecoveryUnknownNeverCallsAgain(t *testing.T) {
	ctx, _, store, effect, req := currentReminderNativeFixture(t)
	effect.mode = "unknown"
	result, err := store.AuthorizeReminderGap(ctx, 7, 11, req)
	require.NoError(t, err)
	require.Equal(t, app.ActionAuditStatusPendingReconciliation, result.Status)
	require.Equal(t, "manual_required", result.Result["phase"])
	effect.mode = "complete"
	_, err = store.ResumeReminderGap(ctx, 7, 11, req)
	require.NoError(t, err)
	require.EqualValues(t, 1, effect.calls.Load())
	require.EqualValues(t, 1, effect.boundaries.Load())
	_, _, err = store.ResolveReminderGap(ctx, 7, 12, req)
	require.ErrorIs(t, err, app.ErrActionAuditInputConflict)
	changed := req
	changed.SourceFingerprint = strings.Repeat("cd", 32)
	_, _, err = store.ResolveReminderGap(ctx, 7, 11, changed)
	require.ErrorIs(t, err, app.ErrActionAuditInputConflict)
}

func TestCurrentReminderRecoveryResumesFrozenOriginalRecipients(t *testing.T) {
	ctx, db, store, effect, req := currentReminderNativeFixture(t)
	effect.mode = "freeze-only"
	result, err := store.AuthorizeReminderGap(ctx, 7, 11, req)
	require.NoError(t, err)
	require.Equal(t, app.ActionAuditStatusPendingReconciliation, result.Status)
	effect.mode = "complete"
	resumed := NewCurrentReminderRecovery(db, effect, nil)
	resumed.capture = store.capture
	result, err = resumed.ResumeReminderGap(ctx, 7, 11, req)
	require.NoError(t, err)
	require.Equal(t, "succeeded", result.Status)
	require.EqualValues(t, 1, effect.boundaries.Load())
	var count int64
	require.NoError(t, db.Table("task_opened_reminder_delivery").Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestCurrentReminderRecoveryConcurrentRequestsKeepOneOperation(t *testing.T) {
	ctx, db, store, effect, req := currentReminderNativeFixture(t)
	start := make(chan struct{})
	errors := make(chan error, 2)
	var workers sync.WaitGroup
	for _, id := range []string{"original-recovery", "competing-recovery"} {
		workers.Go(func() {
			request := req
			request.RequestID = id
			<-start
			_, err := store.AuthorizeReminderGap(ctx, 7, 11, request)
			errors <- err
		})
	}
	close(start)
	workers.Wait()
	close(errors)
	succeeded, conflicted := 0, 0
	for err := range errors {
		if err == nil {
			succeeded++
		} else {
			require.True(t, baseerrors.IsCode(err, code.ErrConflict), "%+v", err)
			conflicted++
		}
	}
	require.Equal(t, 1, succeeded)
	require.Equal(t, 1, conflicted)
	require.EqualValues(t, 1, effect.calls.Load())
	require.EqualValues(t, 1, effect.boundaries.Load())
	var count int64
	require.NoError(t, db.Table("system_governance_action_runs").Count(&count).Error)
	require.EqualValues(t, 1, count)
}
