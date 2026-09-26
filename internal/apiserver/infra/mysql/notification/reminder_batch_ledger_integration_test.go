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

func TestReminderBatchFreezesCompleteRecipientSetAcrossConsumers(t *testing.T) {
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
	for _, table := range []string{"task_opened_reminder_batch", "task_opened_reminder_delivery"} {
		require.NoError(t, firstDB.Exec("DROP TABLE IF EXISTS "+table).Error)
	}
	for _, name := range []string{
		"000087_task_opened_reminder_delivery.up.sql",
		"000088_task_opened_reminder_batch.up.sql",
	} {
		schema, readErr := os.ReadFile(filepath.Join("..", "..", "..", "..", "pkg", "migration", "migrations", "mysql", name))
		require.NoError(t, readErr)
		require.NoError(t, firstDB.Exec(string(schema)).Error)
	}

	first := NewReminderBatchLedger(firstDB)
	second := NewReminderBatchLedger(secondDB)
	key := appnotification.ReminderBatchKey{OrgID: 501, TaskID: "task-1", OpeningEventID: "event-1", ScheduleRevision: 3, ReminderVersion: 1}
	now := time.Date(2026, 9, 26, 10, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	type result struct {
		batch appnotification.ReminderBatch
		err   error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	sets := [][]appnotification.ReminderRecipientIdentity{
		{{UserID: "self-a", LoginIdentityID: "identity-a"}},
		{{UserID: "self-b", LoginIdentityID: "identity-b"}},
	}
	var wg sync.WaitGroup
	for index, store := range []*ReminderBatchLedger{first, second} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			batch, freezeErr := store.FreezeRecipients(context.Background(), key, "app-1", "template-1", sets[index], now)
			results <- result{batch: batch, err: freezeErr}
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	var original []appnotification.ReminderRecipientIdentity
	for got := range results {
		require.NoError(t, got.err)
		require.Len(t, got.batch.Recipients, 1)
		if original == nil {
			original = got.batch.Recipients
		} else {
			require.Equal(t, original, got.batch.Recipients)
		}
	}
	var count int64
	require.NoError(t, firstDB.Table("task_opened_reminder_delivery").Count(&count).Error)
	require.EqualValues(t, 1, count, "competing IAM snapshots must not expand the frozen set")
	batch, err := second.FreezeRecipients(context.Background(), key, "app-rotated", "template-rotated",
		[]appnotification.ReminderRecipientIdentity{{UserID: "late", LoginIdentityID: "identity-late"}}, now.Add(time.Minute))
	require.NoError(t, err)
	require.Equal(t, "app-1", batch.AppID)
	require.Equal(t, "template-1", batch.TemplateID)
	require.Equal(t, original, batch.Recipients)

	empty := key
	empty.OpeningEventID = "event-empty"
	suppressed, err := first.SuppressEmpty(context.Background(), empty, "app-1", "template-1", "reminder_window_elapsed", now)
	require.NoError(t, err)
	require.True(t, suppressed.Suppressed)
	require.Empty(t, suppressed.Recipients)
	suppressed, err = second.FreezeRecipients(context.Background(), empty, "app-1", "template-1", sets[0], now.Add(time.Minute))
	require.NoError(t, err)
	require.True(t, suppressed.Suppressed, "a final no-send decision must not later expand into sends")
}
