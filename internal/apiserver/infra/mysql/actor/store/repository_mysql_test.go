package store_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/store"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	entrydomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/assessmententry"
	actorrepo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	repo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor/store"
	dbctx "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	driver "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func isolatedMySQL(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("QS_SERVER_TEST_MYSQL_DSN")
	if dsn == "" {
		if os.Getenv("QS_STORE_REQUIRE_MYSQL") == "true" {
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
	name := fmt.Sprintf("qs_store_test_%d", time.Now().UnixNano())
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

func mysqlFixture(t *testing.T) (*gorm.DB, *app.Service, context.Context) {
	t.Helper()
	db := isolatedMySQL(t)
	var err error
	// Minimal pre-upgrade tables isolate the additive migration from unrelated modules.
	for _, sql := range []string{
		"CREATE TABLE clinician(id BIGINT UNSIGNED PRIMARY KEY,org_id BIGINT NOT NULL,version INT UNSIGNED NOT NULL DEFAULT 1, is_active BOOLEAN NOT NULL DEFAULT TRUE,deleted_at DATETIME NULL, updated_at DATETIME(6),updated_by BIGINT)",
		"CREATE TABLE assessment_entry(id BIGINT UNSIGNED PRIMARY KEY,org_id BIGINT NOT NULL,clinician_id BIGINT UNSIGNED NOT NULL,version INT UNSIGNED NOT NULL DEFAULT 1,is_active BOOLEAN NOT NULL DEFAULT TRUE,deleted_at DATETIME NULL,updated_at DATETIME(6),updated_by BIGINT)",
	} {
		if err = db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile("../../../../../pkg/migration/migrations/mysql/000073_actor_stores.up.sql")
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
	ctx := authz.WithSnapshot(context.Background(), &authz.Snapshot{Permissions: []authz.Permission{{Resource: "qs:*:*:*", Action: "*", Mode: authz.AuthorizationModeUnconditional}}})
	return db, app.NewService(repo.NewRepository(db), tx), ctx
}
func TestMySQLStoreAssignmentLifecycle(t *testing.T) {
	db, s, ctx := mysqlFixture(t)
	a := app.Actor{OrgID: 7, UserID: 9}
	first, err := s.Create(ctx, a, " a ", "门店A", "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Create(ctx, a, "B", "门店B", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Create(ctx, a, "A", "重名", ""); err == nil {
		t.Fatal("duplicate normalized code accepted")
	}
	if err = db.Exec("INSERT INTO clinician(id,org_id,is_active) VALUES (10,7,false)").Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec("INSERT INTO assessment_entry(id,org_id,clinician_id,is_active) VALUES (20,7,10,true),(21,7,10,false)").Error; err != nil {
		t.Fatal(err)
	}
	c := app.Change{StoreID: first.ID(), ExpectedVersion: 1, Reason: "首次配置", RequestID: "initial"}
	h, err := s.Assign(ctx, a, 10, c)
	if err != nil {
		t.Fatal(err)
	}
	if h.InvalidatedCount != 0 {
		t.Fatal("initial assignment invalidated entries")
	}
	inactive := false
	if _, err = s.Update(ctx, a, first.ID(), first.Version(), "", "", &inactive); err == nil {
		t.Fatal("inactive clinician did not block deactivation")
	}
	c = app.Change{StoreID: second.ID(), ExpectedVersion: 2, Reason: "调店", RequestID: "transfer"}
	h, err = s.Assign(ctx, a, 10, c)
	if err != nil {
		t.Fatal(err)
	}
	if h.InvalidatedCount != 2 {
		t.Fatalf("invalidated %d, want both active and inactive", h.InvalidatedCount)
	}
	replay, err := s.Assign(ctx, a, 10, c)
	if err != nil || replay.ID != h.ID {
		t.Fatalf("replay did not return original history: %v", err)
	}
	var count int64
	if err = db.Table("clinician_store_history").Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("duplicate history: %d %v", count, err)
	}
	if _, err = s.Update(ctx, a, first.ID(), first.Version(), "", "", &inactive); err != nil {
		t.Fatal(err)
	}
	var n int64
	if err = db.Table("assessment_entry").Where("invalidated_at IS NOT NULL AND is_active=false").Count(&n).Error; err != nil || n != 2 {
		t.Fatal("invalidated entries were not preserved")
	}
}
func TestMySQLStoreTransferRollsBackAuditFailure(t *testing.T) {
	db, s, ctx := mysqlFixture(t)
	a := app.Actor{OrgID: 7, UserID: 9}
	first, err := s.Create(ctx, a, "A", "A", "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Create(ctx, a, "B", "B", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec("INSERT INTO clinician(id,org_id,store_id) VALUES (10,7,?)", first.ID()).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec("INSERT INTO assessment_entry(id,org_id,clinician_id) VALUES (20,7,10)").Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec("CREATE TRIGGER fail_history BEFORE INSERT ON clinician_store_history FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected failure'").Error; err != nil {
		t.Fatal(err)
	}
	if _, err = s.Assign(ctx, a, 10, app.Change{StoreID: second.ID(), ExpectedVersion: 1, Reason: "调店", RequestID: "failed"}); err == nil {
		t.Fatal("transaction unexpectedly succeeded")
	}
	var cl struct {
		StoreID uint64
		Version uint32
	}
	if err = db.Table("clinician").Where("id=10").Take(&cl).Error; err != nil {
		t.Fatal(err)
	}
	if cl.StoreID != first.ID() || cl.Version != 1 {
		t.Fatal("clinician update escaped rollback")
	}
	var n int64
	if err = db.Table("assessment_entry").Where("invalidated_at IS NOT NULL").Count(&n).Error; err != nil || n != 0 {
		t.Fatal("entry invalidation escaped rollback")
	}
}

func TestMySQLStoreConcurrentReplay(t *testing.T) {
	db, s, ctx := mysqlFixture(t)
	a := app.Actor{OrgID: 7, UserID: 9}
	target, err := s.Create(ctx, a, "A", "A", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec("INSERT INTO clinician(id,org_id) VALUES(10,7)").Error; err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	c := app.Change{StoreID: target.ID(), ExpectedVersion: 1, Reason: "并发首次配置", RequestID: "same-request"}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for i := 0; i < 2; i++ {
		go func() { _, err := s.Assign(ctx, a, 10, c); results <- err }()
	}
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	var n int64
	if err = db.Table("clinician_store_history").Count(&n).Error; err != nil || n != 1 {
		t.Fatalf("expected one history: count=%d error=%v", n, err)
	}
	var version uint32
	if err = db.Table("clinician").Select("version").Where("id=10").Scan(&version).Error; err != nil || version != 2 {
		t.Fatalf("duplicate update: version=%d error=%v", version, err)
	}
}

func TestMySQLStoreConcurrentDeactivationAndAssignment(t *testing.T) {
	db, s, ctx := mysqlFixture(t)
	a := app.Actor{OrgID: 7, UserID: 9}
	target, err := s.Create(ctx, a, "A", "A", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec("INSERT INTO clinician(id,org_id) VALUES(10,7)").Error; err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	go func() {
		_, err := s.Assign(ctx, a, 10, app.Change{StoreID: target.ID(), ExpectedVersion: 1, Reason: "配置", RequestID: "assign"})
		results <- err
	}()
	go func() { active := false; _, err := s.Update(ctx, a, target.ID(), 1, "", "", &active); results <- err }()
	successes := 0
	for i := 0; i < 2; i++ {
		if <-results == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("expected one winning operation, got %d", successes)
	}
	var n int64
	err = db.Table("clinician c").Joins("JOIN actor_stores s ON c.store_id=s.id").Where("NOT s.is_active").Count(&n).Error
	if err != nil || n != 0 {
		t.Fatalf("doctor assigned to disabled store: %d %v", n, err)
	}
}

// A repeatable-read snapshot established before transfer must not resurrect the old entry.
func TestMySQLStoreEntryCurrentReadAfterTransfer(t *testing.T) {
	db, s, ctx := mysqlFixture(t)
	a := app.Actor{OrgID: 7, UserID: 9}
	first, err := s.Create(ctx, a, "A", "A", "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Create(ctx, a, "B", "B", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec("INSERT INTO clinician(id,org_id,store_id) VALUES(10,7,?)", first.ID()).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec("INSERT INTO assessment_entry(id,org_id,clinician_id) VALUES(20,7,10)").Error; err != nil {
		t.Fatal(err)
	}
	entries := actorrepo.NewAssessmentEntryRepository(db)
	err = db.Transaction(func(tx *gorm.DB) error {
		txCtx := dbctx.WithTx(ctx, tx)
		old, err := entries.FindByID(txCtx, entrydomain.NewID(20))
		if err != nil {
			return err
		}
		if !old.IsActive() {
			t.Fatal("fixture entry inactive")
		}
		if _, err = s.Assign(ctx, a, 10, app.Change{StoreID: second.ID(), ExpectedVersion: 1, Reason: "调店", RequestID: "transfer"}); err != nil {
			return err
		}
		if _, err = repo.NewRepository(db).LockClinician(txCtx, 7, 10); err != nil {
			return err
		}
		fresh, err := entries.LockByID(txCtx, entrydomain.NewID(20))
		if err != nil {
			return err
		}
		if fresh.IsActive() || fresh.InvalidatedAt() == nil {
			t.Fatal("current read reused stale entry state")
		}
		fresh.Reactivate()
		if fresh.IsActive() {
			t.Fatal("domain reactivated permanently invalidated entry")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestMySQLStoreFullMigrationChain(t *testing.T) {
	db := isolatedMySQL(t)
	paths, err := filepath.Glob("../../../../../pkg/migration/migrations/mysql/*.up.sql")
	if err != nil || len(paths) == 0 {
		t.Fatal("migration chain not found")
	}
	versions := map[string]bool{}
	for _, path := range paths {
		version := strings.SplitN(filepath.Base(path), "_", 2)[0]
		if versions[version] {
			t.Fatalf("duplicate migration version %s", version)
		}
		versions[version] = true
		sql, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err = db.Exec(string(sql)).Error; err != nil {
			t.Fatalf("migration %s: %v", filepath.Base(path), err)
		}
	}
	for _, table := range []string{"actor_stores", "clinician_store_history"} {
		if !db.Migrator().HasTable(table) {
			t.Fatalf("missing %s", table)
		}
	}
	for table, column := range map[string]string{"clinician": "store_id", "assessment_entry": "invalidated_at"} {
		if !db.Migrator().HasColumn(table, column) {
			t.Fatalf("missing %s.%s", table, column)
		}
	}
}
