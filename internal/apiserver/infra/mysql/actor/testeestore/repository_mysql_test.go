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
	filter.RestrictToStoreScope = true
	filter.AllAssignedStores = true
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
