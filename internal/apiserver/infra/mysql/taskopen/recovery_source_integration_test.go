//go:build integration

package taskopen

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	repository "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/plan"
	txmysql "github.com/FangcunMount/reliable-messaging/storage/transaction/mysql"
	"github.com/FangcunMount/reliable-messaging/transaction"
	drivermysql "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func currentOpeningNativeFixture(t *testing.T) (context.Context, *gorm.DB, *RecoverySource, RecoveryOpening) {
	t.Helper()
	dsn := os.Getenv("QS_REMINDER_TX_MYSQL_DSN")
	cfg, err := drivermysql.ParseDSN(dsn)
	require.NoError(t, err)
	require.True(t, cfg.Net == "tcp" && strings.HasPrefix(cfg.Addr, "127.0.0.1:"), "owned disposable MySQL required")
	cfg.MultiStatements = true
	admin, err := gorm.Open(gormmysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	adminSQL, err := admin.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, adminSQL.Close()) })
	name := "qs_reminder_source_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
	require.NoError(t, db.WithContext(ctx).Exec(txmysql.Schema).Error)
	require.NoError(t, db.WithContext(ctx).AutoMigrate(&repository.AssessmentTaskPO{}))
	_, err = pool.ExecContext(ctx, "INSERT INTO rm_tx_binding(owner,store_id,writer_epoch,schema_version) VALUES(?,?,?,1)", []byte("qs.mysql"), []byte("owned-reminder"), []byte("fixture"))
	require.NoError(t, err)
	plan := recoveryPlanFixture(t)
	o, err := readRecoveryOpening(plan)
	require.NoError(t, err)
	store, err := txmysql.New(pool, transaction.Binding{Owner: "qs.mysql", StoreID: "owned-reminder", WriterEpoch: "fixture"})
	require.NoError(t, err)
	reserved, err := store.Reserve(ctx, transaction.RequestFor(plan), plan, transaction.ReserveOptions{Lease: time.Minute})
	require.NoError(t, err)
	require.NoError(t, db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		guard, err := store.BindGORM(ctx, tx, reserved.Execution)
		if err != nil {
			return err
		}
		if err = tx.Create(&o.Task).Error; err != nil {
			return err
		}
		return guard.CommitWitness(ctx)
	}))
	source := NewRecoverySource(db)
	source.now = func() time.Time { return o.Request.Intent.OpenAt.Add(time.Minute) }
	return ctx, db, source, o
}

func TestCurrentReminderSourceReadsCommittedRootWithoutOutbox(t *testing.T) {
	ctx, db, source, original := currentOpeningNativeFixture(t)
	cutoff := original.Request.Intent.OpenAt.Add(10 * time.Second)
	o, err := source.Capture(ctx, original.Task.OrgID, original.Task.ID.Uint64(), original.Task.ScheduleRevision, cutoff)
	require.NoError(t, err)
	require.Equal(t, original.Request, o.Request)
	require.Len(t, o.Fingerprint, 64)
	again, err := source.Capture(ctx, original.Task.OrgID, original.Task.ID.Uint64(), original.Task.ScheduleRevision, cutoff)
	require.NoError(t, err)
	require.Equal(t, o.Fingerprint, again.Fingerprint)
	require.False(t, db.Migrator().HasTable("rm_outbox"))
	// Source inspection neither changes protocol observations nor grants tokens.
	var state string
	require.NoError(t, db.Raw("SELECT state FROM rm_tx_execution").Scan(&state).Error)
	require.Equal(t, string(transaction.Committed), state)
}

func TestCurrentReminderSourceRejectsConflictsAndMissingEvidence(t *testing.T) {
	for _, name := range []string{"prepared", "missing_witness", "wrong_witness", "wrong_owner", "wrong_request_link", "changed_plan", "changed_task", "changed_binding"} {
		t.Run(name, func(t *testing.T) {
			ctx, db, source, o := currentOpeningNativeFixture(t)
			var err error
			switch name {
			case "prepared":
				err = db.Exec("UPDATE rm_tx_execution SET state='prepared'").Error
			case "missing_witness":
				err = db.Exec("DELETE FROM rm_tx_witness").Error
			case "wrong_witness":
				err = db.Exec("UPDATE rm_tx_witness SET reference=?", []byte("bad-reference")).Error
			case "wrong_owner":
				err = db.Exec("UPDATE rm_tx_execution SET owner=?", []byte("other")).Error
			case "wrong_request_link":
				err = db.Exec("UPDATE rm_tx_execution SET request_key=?", make([]byte, 32)).Error
			case "changed_plan":
				err = db.Exec("UPDATE rm_tx_execution SET plan=?", []byte(`{}`)).Error
			case "changed_task":
				err = db.Exec("UPDATE assessment_task SET entry_url='other-entry'").Error
			case "changed_binding":
				err = db.Exec("UPDATE rm_tx_binding SET schema_version=99").Error
			}
			require.NoError(t, err)
			_, err = source.Capture(ctx, o.Task.OrgID, o.Task.ID.Uint64(), o.Task.ScheduleRevision, o.Request.Intent.OpenAt.Add(10*time.Second))
			require.Error(t, err)
		})
	}
}
