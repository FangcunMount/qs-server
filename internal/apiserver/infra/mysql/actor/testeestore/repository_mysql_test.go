package testeestore_test

import (
	"context"
	"fmt"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testeestore"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	actorrepo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	repo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor/testeestore"
	migration "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/testeestore"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
	dbctx "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	driver "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func isolatedMySQL(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("QS_SERVER_TEST_MYSQL_DSN")
	if dsn == "" {
		if os.Getenv("QS_TESTEE_STORE_REQUIRE_MYSQL") == "true" {
			t.Fatal("QS_SERVER_TEST_MYSQL_DSN required")
		}
		t.Skip("MySQL integration: set QS_SERVER_TEST_MYSQL_DSN")
	}
	cfg, err := driver.ParseDSN(dsn)
	if err != nil {
		t.Fatal("invalid test DSN")
	}
	cfg.DBName = ""
	cfg.ParseTime = true
	cfg.MultiStatements = true
	admin, err := gorm.Open(gormmysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("cannot connect to isolated MySQL")
	}
	name := fmt.Sprintf("qs_testee_store_test_%d", time.Now().UnixNano())
	if err = admin.Exec("CREATE DATABASE " + name + " CHARACTER SET utf8mb4").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Exec("DROP DATABASE " + name).Error; sql, _ := admin.DB(); _ = sql.Close() })
	cfg.DBName = name
	db, err := gorm.Open(gormmysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("cannot open test database")
	}
	t.Cleanup(func() { sql, _ := db.DB(); _ = sql.Close() })
	return db
}

func fixture(t *testing.T) (*gorm.DB, *app.Service, context.Context) {
	t.Helper()
	db := isolatedMySQL(t)
	for _, sql := range []string{
		"CREATE TABLE testee(id BIGINT UNSIGNED PRIMARY KEY, org_id BIGINT NOT NULL, name VARCHAR(100), deleted_at DATETIME NULL, updated_at DATETIME(6), updated_by BIGINT)",
		"CREATE TABLE actor_stores(id BIGINT UNSIGNED PRIMARY KEY,org_id BIGINT NOT NULL,is_active BOOLEAN NOT NULL,version INT UNSIGNED NOT NULL)",
		"INSERT INTO testee(id,org_id,name) VALUES(10,7,'test')",
		"INSERT INTO actor_stores VALUES(1,7,true,1),(2,7,true,1),(3,8,true,1),(4,7,false,1)",
	} {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile("../../../../../pkg/migration/migrations/mysql/000077_testee_store_ownership.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, sql := range strings.Split(string(raw), ";") {
		if strings.TrimSpace(sql) != "" {
			if err = db.Exec(sql).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	tx := transaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
		return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return fn(dbctx.WithTx(ctx, tx)) })
	})
	ctx := authz.WithSnapshot(actorctx.WithGrantingUserID(actorctx.WithOperatorOrgID(context.Background(), 7), 9), &authz.Snapshot{ScopeContractVersion: 1, AuthzVersion: 1, Permissions: []authz.Permission{{Resource: "qs:*:*:*", Action: "*", Mode: authz.AuthorizationModeUnconditional, Scopes: []authz.DataScope{{OrgID: 7, Kind: "all_stores"}}}}})
	return db, app.NewService(repo.NewRepository(db), tx, testCompanyScope{}), ctx
}
func change(id uint64, version uint32, request string) app.Change {
	return app.Change{StoreID: id, ExpectedVersion: version, Reason: "test ownership", RequestID: request}
}
func assertState(t *testing.T, db *gorm.DB, store *uint64, version uint32, history int64) {
	t.Helper()
	var row struct {
		StoreID      *uint64
		StoreVersion uint32
	}
	if err := db.Table("testee").Where("id=10").Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	if (row.StoreID == nil) != (store == nil) || (store != nil && *row.StoreID != *store) || row.StoreVersion != version {
		t.Fatalf("unexpected ownership: %+v", row)
	}
	var count int64
	if err := db.Table("testee_store_history").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != history {
		t.Fatalf("history=%d want %d", count, history)
	}
}
func TestMySQLTesteeOwnershipLifecycle(t *testing.T) {
	db, s, ctx := fixture(t)
	actor := app.Actor{OrgID: 7, UserID: 9}
	first, err := s.AssignInitial(ctx, actor, 10, change(1, 1, "initial"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AssignInitial(ctx, actor, 10, change(2, 2, "scan")); err == nil {
		t.Fatal("scan overwrote ownership")
	}
	if _, err = s.Transfer(ctx, actor, 10, change(2, 2, "transfer")); err != nil {
		t.Fatal(err)
	}
	repeated, err := s.AssignInitial(ctx, actor, 10, change(1, 1, "initial"))
	if err != nil || repeated.ID != first.ID {
		t.Fatalf("replay: %v", err)
	}
	id := uint64(2)
	assertState(t, db, &id, 3, 2)
	for _, target := range []uint64{3, 4} {
		if _, err = s.Transfer(ctx, actor, 10, change(target, 3, "invalid")); err == nil {
			t.Fatal("invalid store accepted")
		}
	}
	assertState(t, db, &id, 3, 2)
	rows, err := s.History(ctx, actor, 10, 0, 1)
	if err != nil || len(rows) != 1 || rows[0].Kind != "transfer" {
		t.Fatalf("history: %v", err)
	}
	if _, err = s.History(ctx, app.Actor{OrgID: 8, UserID: 9}, 10, 0, 20); err == nil {
		t.Fatal("foreign history visible")
	}
}
func TestMySQLTesteeOwnershipAuditFailureRollsBack(t *testing.T) {
	db, s, ctx := fixture(t)
	if err := db.Exec("CREATE TRIGGER reject_history BEFORE INSERT ON testee_store_history FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected audit failure'").Error; err != nil {
		t.Fatal(err)
	}
	if h, err := s.AssignInitial(ctx, app.Actor{OrgID: 7, UserID: 9}, 10, change(1, 1, "failure")); err == nil || h != nil {
		t.Fatal("failed audit reported success")
	}
	assertState(t, db, nil, 1, 0)
}
func TestMySQLTesteeOwnershipConcurrentInitialHasOneWinner(t *testing.T) {
	db, s, ctx := fixture(t)
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, id := range []uint64{1, 2} {
		wg.Add(1)
		go func(id uint64) {
			defer wg.Done()
			<-start
			_, err := s.AssignInitial(ctx, app.Actor{OrgID: 7, UserID: 9}, 10, change(id, 1, fmt.Sprint(id)))
			results <- err
		}(id)
	}
	close(start)
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("winners=%d", success)
	}
	var count int64
	db.Table("testee_store_history").Count(&count)
	if count != 1 {
		t.Fatal("concurrent history not singular")
	}
}
func TestMySQLTesteeOwnershipAdditiveUpgradeAndDown(t *testing.T) {
	db, _, _ := fixture(t)
	assertState(t, db, nil, 1, 0)
	raw, err := os.ReadFile("../../../../../pkg/migration/migrations/mysql/000077_testee_store_ownership.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, sql := range strings.Split(string(raw), ";") {
		if strings.TrimSpace(sql) != "" {
			if err = db.Exec(sql).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	if db.Migrator().HasColumn("testee", "store_id") || db.Migrator().HasTable("testee_store_history") {
		t.Fatal("down retained new structure")
	}
	var count int64
	db.Table("testee").Count(&count)
	if count != 1 {
		t.Fatal("upgrade/down changed existing testee")
	}
}

func TestMySQLPreflightIsReadOnlyAndDetectsFactDrift(t *testing.T) {
	db, _, _ := fixture(t)
	for _, statement := range []string{
		"CREATE TABLE clinician(id BIGINT UNSIGNED PRIMARY KEY,org_id BIGINT NOT NULL,store_id BIGINT UNSIGNED NULL,is_active BOOLEAN NOT NULL,deleted_at DATETIME NULL)",
		"CREATE TABLE clinician_relation(id BIGINT UNSIGNED PRIMARY KEY,org_id BIGINT NOT NULL,testee_id BIGINT UNSIGNED NOT NULL,clinician_id BIGINT UNSIGNED NOT NULL,relation_type VARCHAR(50),is_active BOOLEAN,deleted_at DATETIME NULL,unbound_at DATETIME NULL)",
		"INSERT INTO clinician VALUES(20,7,1,true,NULL)",
		"INSERT INTO clinician_relation VALUES(30,7,10,20,'attending',true,NULL,NULL)",
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	first, err := migration.Preflight(context.Background(), db, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !first.SchemaReady || !first.Executable || first.Counts["candidate"] != 1 {
		t.Fatalf("unexpected report: %+v", first)
	}
	second, err := migration.Preflight(context.Background(), db, 100)
	if err != nil {
		t.Fatal(err)
	}
	if first.Fingerprint != second.Fingerprint {
		t.Fatal("unchanged facts changed fingerprint")
	}
	assertState(t, db, nil, 1, 0)
	status, err := migration.ReadStatus(context.Background(), db, "status-test", 100)
	if err != nil || status.State != "pending" {
		t.Fatalf("pending status: %+v %v", status, err)
	}
	raw, err := os.ReadFile("../../../../../pkg/migration/migrations/mysql/000078_testee_store_migration_manifest.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range strings.Split(string(raw), ";") {
		if strings.TrimSpace(statement) != "" {
			if err = db.Exec(statement).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	// A historical no-op manifest isolates read-only status classification.
	now := time.Now().UTC()
	digest, _ := migration.ItemsHash(nil)
	manifest := migration.Manifest{MigrationID: "status-test", State: "applied", BeforeHash: first.Fingerprint, AfterHash: first.Fingerprint, ItemsHash: digest, ItemCount: 0, ActorID: 9, CreatedAt: now, CompletedAt: &now}
	if err = db.Create(&manifest).Error; err != nil {
		t.Fatal(err)
	}
	status, err = migration.ReadStatus(context.Background(), db, "status-test", 100)
	if err != nil || status.State != "applied_unchanged" {
		t.Fatalf("applied status: %+v %v", status, err)
	}
	if err := db.Exec("UPDATE clinician SET store_id=NULL WHERE id=20").Error; err != nil {
		t.Fatal(err)
	}
	third, err := migration.Preflight(context.Background(), db, 100)
	if err != nil {
		t.Fatal(err)
	}
	status, err = migration.ReadStatus(context.Background(), db, "status-test", 100)
	if err != nil || status.State != "applied_drifted" {
		t.Fatalf("drift status: %+v %v", status, err)
	}
	if err = db.Table("testee_store_migration_manifests").Where("migration_id=?", "status-test").Update("items_hash", strings.Repeat("0", 64)).Error; err != nil {
		t.Fatal(err)
	}
	status, err = migration.ReadStatus(context.Background(), db, "status-test", 100)
	if err == nil || status.State != "invalid" {
		t.Fatal("corrupted manifest accepted by status")
	}
	if third.Executable || third.Fingerprint == first.Fingerprint || third.Counts["unresolved"] != 1 {
		t.Fatal("drift not surfaced")
	}
	if _, err = migration.Preflight(context.Background(), db, 1); err == nil {
		t.Fatal("truncated source table accepted")
	}
	if err := db.Exec("ALTER TABLE testee DROP COLUMN store_id,DROP COLUMN store_version").Error; err != nil {
		t.Fatal(err)
	}
	legacy, err := migration.Preflight(context.Background(), db, 100)
	if err != nil {
		t.Fatal(err)
	}
	if legacy.SchemaReady || legacy.Executable {
		t.Fatal("legacy schema incorrectly executable")
	}
}

func TestMySQLMigrationManifestStructureAndRollback(t *testing.T) {
	db, _, _ := fixture(t)
	for _, direction := range []string{"up", "down"} {
		raw, err := os.ReadFile("../../../../../pkg/migration/migrations/mysql/000078_testee_store_migration_manifest." + direction + ".sql")
		if err != nil {
			t.Fatal(err)
		}
		for _, sql := range strings.Split(string(raw), ";") {
			if strings.TrimSpace(sql) != "" {
				if err = db.Exec(sql).Error; err != nil {
					t.Fatal(err)
				}
			}
		}
		if direction == "up" {
			now := time.Now().UTC()
			items := []migration.MigrationItem{{MigrationID: "test", TesteeID: 10, OrgID: 7, BeforeVersion: 1, AppliedVersion: 1, Disposition: "deferred", Reason: "no_management_relation"}}
			digest, err := migration.ItemsHash(items)
			if err != nil {
				t.Fatal(err)
			}
			m := migration.Manifest{MigrationID: "test", State: "applied", BeforeHash: strings.Repeat("a", 64), AfterHash: strings.Repeat("b", 64), ItemsHash: digest, ItemCount: 1, ActorID: 9, CreatedAt: now, CompletedAt: &now}
			if err = db.Create(&m).Error; err != nil {
				t.Fatal(err)
			}
			if err = db.Create(&items).Error; err != nil {
				t.Fatal(err)
			}
			var loaded migration.Manifest
			var rows []migration.MigrationItem
			if err = db.First(&loaded, "migration_id=?", "test").Error; err != nil {
				t.Fatal(err)
			}
			if err = db.Where("migration_id=?", "test").Find(&rows).Error; err != nil {
				t.Fatal(err)
			}
			status, err := migration.Classify("test", &loaded, rows, m.AfterHash)
			if err != nil || status.State != "applied_unchanged" {
				t.Fatalf("manifest round trip: %v", err)
			}
		}
	}
	if db.Migrator().HasTable("testee_store_migration_manifests") {
		t.Fatal("manifest down failed")
	}
	assertState(t, db, nil, 1, 0)
}

func TestMySQLStatusRejectsMissingOrChangedOwnershipAudit(t *testing.T) {
	db, _, _ := fixture(t)
	for _, sql := range []string{
		"CREATE TABLE clinician(id BIGINT UNSIGNED PRIMARY KEY,org_id BIGINT,store_id BIGINT UNSIGNED,is_active BOOLEAN,deleted_at DATETIME)",
		"CREATE TABLE clinician_relation(id BIGINT UNSIGNED PRIMARY KEY,org_id BIGINT,testee_id BIGINT UNSIGNED,clinician_id BIGINT UNSIGNED,relation_type VARCHAR(50),is_active BOOLEAN,deleted_at DATETIME,unbound_at DATETIME)",
		"UPDATE testee SET store_id=1,store_version=2 WHERE id=10",
	} {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile("../../../../../pkg/migration/migrations/mysql/000078_testee_store_migration_manifest.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, sql := range strings.Split(string(raw), ";") {
		if strings.TrimSpace(sql) != "" {
			if err = db.Exec(sql).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	report, err := migration.Preflight(context.Background(), db, 100)
	if err != nil {
		t.Fatal(err)
	}
	target, historyID := uint64(1), uint64(99)
	now := time.Now().UTC()
	items := []migration.MigrationItem{{MigrationID: "audit-test", TesteeID: 10, OrgID: 7, BeforeVersion: 1, AppliedVersion: 2, TargetStoreID: &target, HistoryID: &historyID, Disposition: "candidate"}}
	digest, _ := migration.ItemsHash(items)
	manifest := migration.Manifest{MigrationID: "audit-test", State: "applied", BeforeHash: strings.Repeat("a", 64), AfterHash: report.Fingerprint, ItemsHash: digest, ItemCount: 1, ActorID: 9, CreatedAt: now, CompletedAt: &now}
	if err = db.Create(&manifest).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Create(&items).Error; err != nil {
		t.Fatal(err)
	}
	history := repo.HistoryPO{ID: 99, OrgID: 7, TesteeID: 10, ToStoreID: 1, Kind: "migration_initial", ActorID: 9, CreatedAt: now, Reason: "migration", RequestID: migration.MigrationRequestID("audit-test"), Version: 2}
	if err = db.Create(&history).Error; err != nil {
		t.Fatal(err)
	}
	state, err := migration.Verify(context.Background(), db, "audit-test", 100)
	if err != nil || state.State != "applied_unchanged" {
		t.Fatalf("valid audit: %v", err)
	}
	if err = db.Table("testee_store_history").Where("id=99").Update("actor_id", 100).Error; err != nil {
		t.Fatal(err)
	}
	state, err = migration.Verify(context.Background(), db, "audit-test", 100)
	if err == nil || state.State != "invalid" {
		t.Fatal("modified audit accepted")
	}
	if err = db.Exec("DELETE FROM testee_store_history WHERE id=99").Error; err != nil {
		t.Fatal(err)
	}
	state, err = migration.ReadStatus(context.Background(), db, "audit-test", 100)
	if err == nil || state.State != "invalid" {
		t.Fatal("missing audit accepted")
	}
}

func applyFixture(t *testing.T) (*gorm.DB, context.Context, migration.ApplyCommand) {
	t.Helper()
	db, _, ctx := fixture(t)
	for _, sql := range []string{
		"CREATE TABLE clinician(id BIGINT UNSIGNED PRIMARY KEY,org_id BIGINT,store_id BIGINT UNSIGNED,is_active BOOLEAN,deleted_at DATETIME)",
		"CREATE TABLE clinician_relation(id BIGINT UNSIGNED PRIMARY KEY,org_id BIGINT,testee_id BIGINT UNSIGNED,clinician_id BIGINT UNSIGNED,relation_type VARCHAR(50),is_active BOOLEAN,deleted_at DATETIME,unbound_at DATETIME)",
		"CREATE TABLE operators(id BIGINT UNSIGNED PRIMARY KEY,org_id BIGINT,user_id BIGINT,is_active BOOLEAN,deleted_at DATETIME)",
		"INSERT INTO operators VALUES(9,7,9,true,NULL)",
		"INSERT INTO clinician VALUES(20,7,1,true,NULL)",
		"INSERT INTO clinician_relation VALUES(30,7,10,20,'attending',true,NULL,NULL)",
	} {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile("../../../../../pkg/migration/migrations/mysql/000078_testee_store_migration_manifest.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, sql := range strings.Split(string(raw), ";") {
		if strings.TrimSpace(sql) != "" {
			if err = db.Exec(sql).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	report, err := migration.Preflight(ctx, db, 100)
	if err != nil {
		t.Fatal(err)
	}
	return db, ctx, migration.ApplyCommand{MigrationID: "apply-test", Fingerprint: report.Fingerprint, OrgID: 7, ActorID: 9, MaxRows: 100, WritesPaused: true}
}
func TestMySQLApplyIsAtomicIdempotentAndRejectsDrift(t *testing.T) {
	db, ctx, cmd := applyFixture(t)
	first, err := migration.Apply(ctx, db, cmd)
	if err != nil {
		t.Fatal(err)
	}
	id := uint64(1)
	assertState(t, db, &id, 2, 1)
	repeated, err := migration.Apply(ctx, db, cmd)
	if err != nil || repeated.AfterHash != first.AfterHash {
		t.Fatalf("replay: %v", err)
	}
	assertState(t, db, &id, 2, 1)
	state, err := migration.Verify(ctx, db, cmd.MigrationID, 100)
	if err != nil || state.State != "applied_unchanged" {
		t.Fatalf("verify: %v", err)
	}
	if err = db.Exec("UPDATE clinician SET store_id=2 WHERE id=20").Error; err != nil {
		t.Fatal(err)
	}
	if _, err = migration.Apply(ctx, db, cmd); err == nil {
		t.Fatal("drift overwritten")
	}
	assertState(t, db, &id, 2, 1)
}
func TestMySQLApplyHistoryFailureRollsBackManifestAndOwnership(t *testing.T) {
	db, ctx, cmd := applyFixture(t)
	if err := db.Exec("CREATE TRIGGER reject_migration_history BEFORE INSERT ON testee_store_history FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected failure'").Error; err != nil {
		t.Fatal(err)
	}
	if result, err := migration.Apply(ctx, db, cmd); err == nil || result != nil {
		t.Fatal("failed history accepted")
	}
	assertState(t, db, nil, 1, 0)
	for _, table := range []string{"testee_store_migration_manifests", "testee_store_migration_items"} {
		var n int64
		db.Table(table).Count(&n)
		if n != 0 {
			t.Fatal("partial migration evidence committed")
		}
	}
}
func TestMySQLApplyRequiresMaintenanceAdminAndMatchingFingerprint(t *testing.T) {
	db, ctx, cmd := applyFixture(t)
	if _, err := migration.Apply(context.Background(), db, cmd); err == nil {
		t.Fatal("untrusted actor accepted")
	}
	noPause := cmd
	noPause.WritesPaused = false
	if _, err := migration.Apply(ctx, db, noPause); err == nil {
		t.Fatal("live writes allowed")
	}
	wrong := cmd
	wrong.Fingerprint = strings.Repeat("c", 64)
	if _, err := migration.Apply(ctx, db, wrong); err == nil {
		t.Fatal("wrong fingerprint accepted")
	}
	assertState(t, db, nil, 1, 0)
}

func TestMySQLRollbackPreservesHistoryAndNeverReusesMigration(t *testing.T) {
	db, ctx, cmd := applyFixture(t)
	applied, err := migration.Apply(ctx, db, cmd)
	if err != nil {
		t.Fatal(err)
	}
	reverted, err := migration.Rollback(ctx, db, cmd)
	if err != nil {
		t.Fatal(err)
	}
	if reverted.AfterHash != applied.AfterHash || reverted.ItemsHash != applied.ItemsHash || reverted.RollbackHash == applied.BeforeHash {
		t.Fatal("baseline rewritten or version reset")
	}
	assertState(t, db, nil, 3, 1)
	repeated, err := migration.Rollback(ctx, db, cmd)
	if err != nil || repeated.RollbackHash != reverted.RollbackHash {
		t.Fatalf("repeat: %v", err)
	}
	assertState(t, db, nil, 3, 1)
	var n int64
	if err = db.Table("testee_store_migration_reversions").Count(&n).Error; err != nil || n != 1 {
		t.Fatalf("audit %d: %v", n, err)
	}
	state, err := migration.ReadStatus(ctx, db, cmd.MigrationID, 100)
	if err != nil || state.State != "rolled_back" {
		t.Fatalf("status: %+v %v", state, err)
	}
	if _, err = migration.Apply(ctx, db, cmd); err == nil {
		t.Fatal("same migration applied again")
	}
	if err = db.Exec("UPDATE clinician SET store_id=2 WHERE id=20").Error; err != nil {
		t.Fatal(err)
	}
	if _, err = migration.Rollback(ctx, db, cmd); err == nil {
		t.Fatal("post-rollback drift overwritten")
	}
	assertState(t, db, nil, 3, 1)
}
func TestMySQLRollbackRejectsDriftAndAuditFailure(t *testing.T) {
	for _, scenario := range []string{"drift", "audit_failure"} {
		t.Run(scenario, func(t *testing.T) {
			db, ctx, cmd := applyFixture(t)
			if _, err := migration.Apply(ctx, db, cmd); err != nil {
				t.Fatal(err)
			}
			statement := "UPDATE clinician SET store_id=2 WHERE id=20"
			if scenario == "audit_failure" {
				statement = "CREATE TRIGGER reject_reversion BEFORE INSERT ON testee_store_migration_reversions FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected failure'"
			}
			if err := db.Exec(statement).Error; err != nil {
				t.Fatal(err)
			}
			if result, err := migration.Rollback(ctx, db, cmd); err == nil || result != nil {
				t.Fatal("unsafe compensation succeeded")
			}
			id := uint64(1)
			assertState(t, db, &id, 2, 1)
			var manifest migration.Manifest
			if err := db.Where("migration_id=?", cmd.MigrationID).Take(&manifest).Error; err != nil {
				t.Fatal(err)
			}
			if manifest.State != "applied" || manifest.RollbackHash != "" {
				t.Fatal("partial manifest compensation")
			}
		})
	}
}
func TestMySQLRollbackMissingAuditIsInvalid(t *testing.T) {
	db, ctx, cmd := applyFixture(t)
	if _, err := migration.Apply(ctx, db, cmd); err != nil {
		t.Fatal(err)
	}
	if _, err := migration.Rollback(ctx, db, cmd); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("DELETE FROM testee_store_migration_reversions").Error; err != nil {
		t.Fatal(err)
	}
	state, err := migration.ReadStatus(ctx, db, cmd.MigrationID, 100)
	if err == nil || state.State != "invalid" {
		t.Fatalf("missing audit accepted: %+v %v", state, err)
	}
}

func TestMySQLRollbackRejectsFalseCompletedBaseline(t *testing.T) {
	db, ctx, cmd := applyFixture(t)
	if _, err := migration.Apply(ctx, db, cmd); err != nil {
		t.Fatal(err)
	}
	if _, err := migration.Rollback(ctx, db, cmd); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE testee SET store_id=1 WHERE id=10").Error; err != nil {
		t.Fatal(err)
	}
	current, err := migration.Preflight(ctx, db, 100)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Table("testee_store_migration_manifests").Where("migration_id=?", cmd.MigrationID).Update("rollback_hash", current.Fingerprint).Error; err != nil {
		t.Fatal(err)
	}
	state, err := migration.ReadStatus(ctx, db, cmd.MigrationID, 100)
	if err == nil || state.State != "invalid" {
		t.Fatal("false restored ownership accepted")
	}
	if _, err = migration.Rollback(ctx, db, cmd); err == nil {
		t.Fatal("repeat trusted false baseline")
	}
}

func TestMySQLCurrentOwnershipReflectsTransferAndDeletion(t *testing.T) {
	db, _, ctx := fixture(t)
	reader := actorrepo.NewTesteeRepository(db).(testee.OwnershipReader)
	before, err := reader.FindCurrentOwnership(ctx, testee.ID(10))
	if err != nil || before.StoreID != nil || before.Version != 1 || before.OrgID != 7 {
		t.Fatalf("initial ownership: %+v %v", before, err)
	}
	if err = db.Exec("UPDATE testee SET store_id=2,store_version=2 WHERE id=10").Error; err != nil {
		t.Fatal(err)
	}
	current, err := reader.FindCurrentOwnership(ctx, testee.ID(10))
	if err != nil || current.StoreID == nil || *current.StoreID != 2 || current.Version != 2 {
		t.Fatalf("current: %+v %v", current, err)
	}
	sentinel := fmt.Errorf("abort transaction")
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("UPDATE testee SET store_id=1,store_version=3 WHERE id=10").Error; err != nil {
			return err
		}
		local, err := reader.FindCurrentOwnership(dbctx.WithTx(ctx, tx), testee.ID(10))
		if err != nil || local.StoreID == nil || *local.StoreID != 1 || local.Version != 3 {
			t.Fatalf("transaction ignored: %+v %v", local, err)
		}
		return sentinel
	})
	if err != sentinel {
		t.Fatal(err)
	}
	if err = db.Exec("UPDATE testee SET deleted_at=NOW() WHERE id=10").Error; err != nil {
		t.Fatal(err)
	}
	if _, err = reader.FindCurrentOwnership(ctx, testee.ID(10)); err == nil {
		t.Fatal("deleted testee visible")
	}
}

func TestMySQLMigrationProductionScale(t *testing.T) {
	if os.Getenv("QS_TESTEE_STORE_SCALE_TEST") != "true" {
		t.Skip("explicit bounded production-scale drill")
	}
	db, ctx, cmd := applyFixture(t)
	// Purely synthetic identifiers and relations; no production personal data.
	if err := db.Exec("DELETE FROM clinician_relation").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("DELETE FROM testee").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE digits(n INT PRIMARY KEY)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO digits VALUES(0),(1),(2),(3),(4),(5),(6),(7),(8),(9)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO testee(id,org_id,name)
 SELECT 1000+a.n+10*b.n+100*c.n+1000*d.n+10000*e.n,7,'synthetic'
 FROM digits a CROSS JOIN digits b CROSS JOIN digits c CROSS JOIN digits d CROSS JOIN digits e
 WHERE a.n+10*b.n+100*c.n+1000*d.n+10000*e.n < 83796`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO clinician_relation(id,org_id,testee_id,clinician_id,relation_type,is_active)
 SELECT id,org_id,id,20,'attending',true FROM testee ORDER BY id LIMIT 55576`).Error; err != nil {
		t.Fatal(err)
	}
	cmd.MaxRows = 150000
	started := time.Now()
	report, err := migration.Preflight(ctx, db, cmd.MaxRows)
	if err != nil {
		t.Fatal(err)
	}
	if report.Counts["candidate"] != 55576 || report.Counts["deferred"] != 28220 {
		t.Fatalf("unexpected plan: %+v", report.Counts)
	}
	cmd.Fingerprint = report.Fingerprint
	applied, err := migration.Apply(ctx, db, cmd)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = migration.Verify(ctx, db, cmd.MigrationID, cmd.MaxRows); err != nil {
		t.Fatal(err)
	}
	if _, err = migration.Apply(ctx, db, cmd); err != nil {
		t.Fatal(err)
	}
	t.Logf("preflight/apply/verify/repeat: %s", time.Since(started))
	reverted, err := migration.Rollback(ctx, db, cmd)
	if err != nil {
		t.Fatal(err)
	}
	if reverted.ItemsHash != applied.ItemsHash {
		t.Fatal("original manifest changed")
	}
	if _, err = migration.Rollback(ctx, db, cmd); err != nil {
		t.Fatal(err)
	}
	var changed, deferred, restored int64
	for query, target := range map[string]*int64{
		"SELECT COUNT(*) FROM testee WHERE store_id IS NOT NULL":                 &changed,
		"SELECT COUNT(*) FROM testee WHERE store_id IS NULL AND store_version=1": &deferred,
		"SELECT COUNT(*) FROM testee WHERE store_id IS NULL AND store_version=3": &restored,
	} {
		if err = db.Raw(query).Scan(target).Error; err != nil {
			t.Fatal(err)
		}
	}
	if changed != 0 || deferred != 28220 || restored != 55576 {
		t.Fatalf("unsafe scale compensation: %d/%d/%d", changed, deferred, restored)
	}
	t.Logf("complete migration and compensation: %s", time.Since(started))
}

func TestMySQLStoreFilterAppliesBeforePaginationAndCount(t *testing.T) {
	db, _, ctx := fixture(t)
	if err := db.Exec("ALTER TABLE testee ADD created_at DATETIME NULL").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO testee(id,org_id,store_id) VALUES(11,7,1),(12,7,1),(13,7,2),(14,8,1)").Error; err != nil {
		t.Fatal(err)
	}
	reader := actorrepo.NewReadModel(db)
	id := uint64(1)
	filter := actorreadmodel.TesteeFilter{OrgID: 7, StoreID: &id, Limit: 1, Offset: 1}
	rows, err := reader.ListTestees(ctx, filter)
	if err != nil {
		t.Fatal(err)
	}
	count, err := reader.CountTestees(ctx, filter)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 || len(rows) != 1 || rows[0].ID != 11 {
		t.Fatalf("wrong filtered page/count: %+v %d", rows, count)
	}
	filter.StoreID = nil
	filter.UnassignedStore = true
	filter.Offset = 0
	rows, err = reader.ListTestees(ctx, filter)
	if err != nil {
		t.Fatal(err)
	}
	count, err = reader.CountTestees(ctx, filter)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || len(rows) != 1 || rows[0].ID != 10 {
		t.Fatal("unassigned filter failed")
	}
	filter.RestrictToAccessScope = true
	filter.AccessibleTesteeIDs = []uint64{13}
	rows, err = reader.ListTestees(ctx, filter)
	if err != nil {
		t.Fatal(err)
	}
	count, err = reader.CountTestees(ctx, filter)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 || len(rows) != 0 {
		t.Fatal("store filter expanded access scope")
	}
}

// These persistence tests supply the explicit company range; active membership is tested in Actor access.
type testCompanyScope struct{}

func (testCompanyScope) ResolveStoreRange(ctx context.Context, org, user int64, resource, action string) (authz.StoreRange, error) {
	snap, _ := authz.FromContext(ctx)
	return snap.ResolveStoreRange(org, resource, action)
}
