package operatorretirement

import (
	"context"
	"fmt"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operatorretirement"
	driver "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func isolatedMySQL(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("QS_SERVER_TEST_MYSQL_DSN")
	if dsn == "" {
		if os.Getenv("QS_OPERATOR_RETIREMENT_REQUIRE_MYSQL") == "true" {
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
	name := fmt.Sprintf("qs_operator_exit_test_%d", time.Now().UnixNano())
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

func TestMySQLRetirementTaskAndLocalAdmissionAreAtomic(t *testing.T) {
	db := isolatedMySQL(t)
	if err := db.Exec("CREATE TABLE operators (id BIGINT UNSIGNED PRIMARY KEY, org_id BIGINT, user_id BIGINT, version INT UNSIGNED, is_active BOOLEAN, deleted_at DATETIME(3), updated_at DATETIME(3), updated_by BIGINT, roles JSON, effective_roles JSON, authz_policy_version BIGINT, authz_projected_at DATETIME(3), authz_projection_pending BOOLEAN)").Error; err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join("../../../../../pkg/migration/migrations/mysql/000074_operator_retirement_tasks.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(string(raw)).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec("INSERT INTO operators(id,org_id,user_id,version,is_active) VALUES(7,1,8,3,TRUE)").Error; err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(db)
	ctx := context.Background()
	task, err := domain.New(7, 1, 8, 9, 3, "request-7", "retire doctor operator", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	err = repo.WithOperatorLock(ctx, 1, 7, func(locked context.Context) error {
		if err := repo.Begin(locked, task); err != nil {
			return err
		}
		if err := repo.WithOperatorLock(ctx, 1, 7, func(context.Context) error { return nil }); err == nil {
			return fmt.Errorf("concurrent mutation accepted")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var active bool
	db.Table("operators").Select("is_active").Where("id=7").Scan(&active)
	if active {
		t.Fatal("operator not disabled")
	}
	called := false
	if err := repo.WithinMutation(ctx, 8, func(context.Context) error { called = true; return nil }); err == nil || called {
		t.Fatal("retiring operator can be mutated")
	}
	if err := repo.WithinMutation(ctx, 20, func(locked context.Context) error {
		return repo.WithinMutation(locked, 20, func(context.Context) error { return nil })
	}); err != nil {
		t.Fatal("unrelated/nested mutation blocked", err)
	}
	stored, err := repo.FindTask(ctx, 1, 7)
	if err != nil || stored.Stage != domain.Disabled {
		t.Fatal(stored, err)
	}
	if err = stored.MarkRevoked(11, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err = repo.Save(ctx, *stored); err != nil {
		t.Fatal(err)
	}
	if err = stored.Complete(11, false, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err = db.Exec("CREATE TRIGGER refuse_exit BEFORE UPDATE ON operator_retirement_tasks FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected audit failure'").Error; err != nil {
		t.Fatal(err)
	}
	if err = repo.Finish(ctx, *stored); err == nil {
		t.Fatal("injected failure accepted")
	}
	var count int64
	db.Table("operators").Where("deleted_at IS NULL").Count(&count)
	if count != 1 {
		t.Fatal("partial delete committed")
	}
	db.Exec("DROP TRIGGER refuse_exit")
	if err = repo.Finish(ctx, *stored); err != nil {
		t.Fatal(err)
	}
	db.Table("operators").Where("deleted_at IS NULL").Count(&count)
	if count != 0 {
		t.Fatal("operator not soft deleted")
	}
	persisted, err := repo.FindTask(ctx, 1, 7)
	if err != nil || persisted.Stage != domain.Completed {
		t.Fatal(persisted, err)
	}
}

func TestMySQLRetirementRequiresTestConfigurationInCI(t *testing.T) {
	if os.Getenv("CI") == "true" && os.Getenv("QS_OPERATOR_RETIREMENT_REQUIRE_MYSQL") == "true" && os.Getenv("QS_SERVER_TEST_MYSQL_DSN") == "" {
		t.Fatal("missing mandatory isolated MySQL configuration")
	}
}
