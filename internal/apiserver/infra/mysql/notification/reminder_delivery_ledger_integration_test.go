package notification

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	appnotification "github.com/FangcunMount/qs-server/internal/apiserver/application/notification"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestReminderDeliveryLedgerSurvivesCompetingConsumersAndUnknownSend(t *testing.T) {
	dsn := os.Getenv("QS_M5_REMINDER_LEDGER_MYSQL_DSN")
	if dsn == "" {
		if os.Getenv("QS_M5_REMINDER_LEDGER_MYSQL_REQUIRED") == "1" {
			t.Fatal("QS_M5_REMINDER_LEDGER_MYSQL_DSN required")
		}
		t.Skip("disposable MySQL not configured")
	}
	firstDB, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	secondDB, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	for _, db := range []*gorm.DB{firstDB, secondDB} {
		connection, openErr := db.DB()
		require.NoError(t, openErr)
		t.Cleanup(func() { require.NoError(t, connection.Close()) })
	}
	var database string
	require.NoError(t, firstDB.Raw("SELECT DATABASE()").Scan(&database).Error)
	require.True(t, strings.HasPrefix(database, "qs_m5_reminder_ledger_test_"), "disposable database required")
	require.NoError(t, firstDB.Exec("DROP TABLE IF EXISTS task_opened_reminder_delivery").Error)
	migrationPath := filepath.Join("..", "..", "..", "..", "pkg", "migration", "migrations", "mysql", "000087_task_opened_reminder_delivery.up.sql")
	schema, err := os.ReadFile(migrationPath)
	require.NoError(t, err)
	require.NoError(t, firstDB.Exec(string(schema)).Error)

	first := NewReminderDeliveryLedger(firstDB)
	second := NewReminderDeliveryLedger(secondDB)
	ctx := context.Background()
	now := time.Date(2026, 9, 26, 8, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	base := appnotification.ReminderDeliveryKey{
		OrgID: 501, TaskID: "task-1", OpeningEventID: "event-1", ScheduleRevision: 3,
		LoginIdentityID: "identity-1", AppID: "mini-app", TemplateID: "template-1", ReminderVersion: 1,
	}
	delivery, err := first.EnsurePending(ctx, base, "user-1", now)
	require.NoError(t, err)
	require.Equal(t, appnotification.ReminderPending, delivery.State)

	// Independent store instances race for the same persisted responsibility.
	start := make(chan struct{})
	type claimResult struct {
		token   string
		claimed bool
		err     error
	}
	results := make(chan claimResult, 2)
	var workers sync.WaitGroup
	for _, ledger := range []*ReminderDeliveryLedger{first, second} {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			token, claimed, claimErr := ledger.Claim(ctx, base, time.Minute, now)
			results <- claimResult{token, claimed, claimErr}
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	var winningToken string
	var claimedCount int
	for result := range results {
		require.NoError(t, result.err)
		if result.claimed {
			claimedCount++
			winningToken = result.token
		} else {
			require.Empty(t, result.token)
		}
	}
	require.Equal(t, 1, claimedCount)
	require.NotEmpty(t, winningToken)

	started, err := first.BeginExternalCall(ctx, base, winningToken, now.Add(time.Second))
	require.NoError(t, err)
	require.True(t, started)
	// After this marker, neither a new process nor an expired lease may send.
	_, claimed, err := second.Claim(ctx, base, time.Minute, now.Add(2*time.Minute))
	require.NoError(t, err)
	require.False(t, claimed)
	_, err = second.EnsurePending(ctx, base, "user-1", now.Add(3*time.Minute))
	require.NoError(t, err)
	delivery, err = second.Read(ctx, base)
	require.NoError(t, err)
	require.Equal(t, appnotification.ReminderSending, delivery.State)
	require.NotNil(t, delivery.ExternalCallStartedAt)
	require.Empty(t, delivery.PlatformMessageID)
	confirmed, err := first.Confirm(ctx, base, winningToken, "wechat-msg-1", now.Add(4*time.Minute))
	require.NoError(t, err)
	require.True(t, confirmed)
	confirmed, err = second.Confirm(ctx, base, winningToken, "wechat-msg-2", now.Add(5*time.Minute))
	require.NoError(t, err)
	require.False(t, confirmed)
	delivery, err = second.Read(ctx, base)
	require.NoError(t, err)
	require.Equal(t, appnotification.ReminderConfirmed, delivery.State)
	require.Equal(t, "wechat-msg-1", delivery.PlatformMessageID)
	rotatedTemplate := base
	rotatedTemplate.TemplateID = "template-2"
	delivery, err = second.EnsurePending(ctx, rotatedTemplate, "user-1", now.Add(6*time.Minute))
	require.NoError(t, err)
	require.Equal(t, appnotification.ReminderConfirmed, delivery.State)
	require.Equal(t, "template-1", delivery.Key.TemplateID, "redelivery must preserve the first template")
	_, claimed, err = second.Claim(ctx, rotatedTemplate, time.Minute, now.Add(6*time.Minute))
	require.NoError(t, err)
	require.False(t, claimed)

	unknown := base
	unknown.LoginIdentityID = "identity-2"
	_, err = first.EnsurePending(ctx, unknown, "user-2", now)
	require.NoError(t, err)
	unknownToken, claimed, err := first.Claim(ctx, unknown, time.Minute, now)
	require.NoError(t, err)
	require.True(t, claimed)
	started, err = first.BeginExternalCall(ctx, unknown, unknownToken, now.Add(time.Second))
	require.NoError(t, err)
	require.True(t, started)
	sealed, err := first.MarkUnknown(ctx, unknown, unknownToken, "response_lost", now.Add(2*time.Second))
	require.NoError(t, err)
	require.True(t, sealed)
	_, claimed, err = second.Claim(ctx, unknown, time.Minute, now.Add(3*time.Minute))
	require.NoError(t, err)
	require.False(t, claimed)
	delivery, err = second.Read(ctx, unknown)
	require.NoError(t, err)
	require.Equal(t, appnotification.ReminderManualRequired, delivery.State)
	require.Equal(t, "response_lost", delivery.ResolutionCode)

	crashed := base
	crashed.LoginIdentityID = "identity-3"
	_, err = first.EnsurePending(ctx, crashed, "user-3", now)
	require.NoError(t, err)
	crashedToken, claimed, err := first.Claim(ctx, crashed, time.Minute, now)
	require.NoError(t, err)
	require.True(t, claimed)
	started, err = first.BeginExternalCall(ctx, crashed, crashedToken, now.Add(time.Second))
	require.NoError(t, err)
	require.True(t, started)
	_, claimed, err = second.Claim(ctx, crashed, time.Minute, now.Add(time.Hour))
	require.NoError(t, err)
	require.False(t, claimed, "an interrupted external call must not be sent again")

	unsent := base
	unsent.LoginIdentityID = "identity-4"
	_, err = first.EnsurePending(ctx, unsent, "user-4", now)
	require.NoError(t, err)
	oldToken, claimed, err := first.Claim(ctx, unsent, time.Minute, now)
	require.NoError(t, err)
	require.True(t, claimed)
	newToken, claimed, err := second.Claim(ctx, unsent, time.Minute, now.Add(2*time.Minute))
	require.NoError(t, err)
	require.True(t, claimed, "an expired claim without an external call is recoverable")
	started, err = first.BeginExternalCall(ctx, unsent, oldToken, now.Add(2*time.Minute))
	require.NoError(t, err)
	require.False(t, started, "the superseded claimant must not call the platform")
	suppressed, err := second.SuppressUnsent(ctx, unsent, newToken, "task_completed", now.Add(2*time.Minute))
	require.NoError(t, err)
	require.True(t, suppressed)
	_, claimed, err = first.Claim(ctx, unsent, time.Minute, now.Add(time.Hour))
	require.NoError(t, err)
	require.False(t, claimed)
	released := base
	released.LoginIdentityID = "identity-5"
	_, err = first.EnsurePending(ctx, released, "user-5", now)
	require.NoError(t, err)
	releaseToken, claimed, err := first.Claim(ctx, released, time.Minute, now)
	require.NoError(t, err)
	require.True(t, claimed)
	wasReleased, err := first.ReleaseUnsent(ctx, released, releaseToken, now.Add(time.Second))
	require.NoError(t, err)
	require.True(t, wasReleased)
	_, claimed, err = second.Claim(ctx, released, time.Minute, now.Add(2*time.Second))
	require.NoError(t, err)
	require.True(t, claimed, "a claim released before the external boundary is safe to retry")
	review, err := second.ListNeedsReview(ctx, 501, now.Add(10*time.Minute), 10)
	require.NoError(t, err)
	require.Len(t, review, 2)
	require.Equal(t, appnotification.ReminderSending, review[0].State)
	require.Equal(t, appnotification.ReminderManualRequired, review[1].State)
	otherOrg, err := second.ListNeedsReview(ctx, 502, now.Add(10*time.Minute), 10)
	require.NoError(t, err)
	require.Empty(t, otherOrg)
	firstPage, err := second.ListReminderReviews(ctx, 501, now.Add(10*time.Minute), "", 1)
	require.NoError(t, err)
	require.Len(t, firstPage.Items, 1)
	require.Equal(t, string(appnotification.ReminderManualRequired), firstPage.Items[0].State)
	require.NotEmpty(t, firstPage.NextCursor)
	secondPage, err := second.ListReminderReviews(ctx, 501, now.Add(10*time.Minute), firstPage.NextCursor, 1)
	require.NoError(t, err)
	require.Len(t, secondPage.Items, 1)
	require.Equal(t, string(appnotification.ReminderSending), secondPage.Items[0].State)
	require.Empty(t, secondPage.NextCursor)
	otherOrgPage, err := second.ListReminderReviews(ctx, 502, now.Add(10*time.Minute), "", 10)
	require.NoError(t, err)
	require.Empty(t, otherOrgPage.Items)
	_, err = second.ListReminderReviews(ctx, 501, now.Add(10*time.Minute), "bad-cursor", 10)
	require.ErrorContains(t, err, "invalid reminder review cursor")

	_, err = second.EnsurePending(ctx, base, "different-user", now)
	require.ErrorContains(t, err, "identity conflict")
	var total int64
	require.NoError(t, firstDB.Table("task_opened_reminder_delivery").Count(&total).Error)
	require.EqualValues(t, 5, total)
	downPath := filepath.Join("..", "..", "..", "..", "pkg", "migration", "migrations", "mysql", "000087_task_opened_reminder_delivery.down.sql")
	downSQL, err := os.ReadFile(downPath)
	require.NoError(t, err)
	require.ErrorContains(t, firstDB.Exec(string(downSQL)).Error, "manual review")
	require.NoError(t, firstDB.Table("task_opened_reminder_delivery").Count(&total).Error)
	require.EqualValues(t, 5, total, "application rollback must not delete reminder responsibilities")
	var sensitiveColumns int64
	require.NoError(t, firstDB.Raw(`SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema = DATABASE() AND table_name = 'task_opened_reminder_delivery'
		AND column_name IN ('open_id','entry_url','access_token')`).Scan(&sensitiveColumns).Error)
	require.Zero(t, sensitiveColumns)
}
