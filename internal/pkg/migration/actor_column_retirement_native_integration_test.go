//go:build integration

package migration

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"reflect"
	"testing"
	"testing/fstest"
	"time"

	operatorDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operatorretirement"
	testeeDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	actor "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	retirement "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor/operatorretirement"
	driver "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const actorColumnsUp = "migrations/mysql/000102_retire_actor_compatibility_columns.up.sql"

func nativeActorColumnPair(t *testing.T) nativeSchemaPair {
	t.Helper()
	t.Setenv("QS_M3_REQUIRE_DATABASE", "1")
	n := openNativeSchemaPairWithPrefix(t, "qs_m4_actor_test")
	r := migrationResourcesThrough(t, 101, 40)
	n.approve(t, r)
	n.runBoth(t, n.preflight(t, r))
	for _, query := range []string{
		`INSERT INTO testee(id,org_id,profile_id,name,gender,tags) VALUES(7,1,11,'historical',0,JSON_ARRAY('retained-in-object-backup')),(8,1,NULL,'empty',0,JSON_ARRAY()),(9,1,NULL,'null',0,NULL)`,
		`INSERT INTO operators(id,org_id,user_id,roles,effective_roles,name,is_active) VALUES(7,1,800007,JSON_ARRAY('qs:result_reviewer'),JSON_ARRAY('qs:result_reviewer','qs:admin'),'target',TRUE),(9,1,900009,JSON_ARRAY('qs:admin'),JSON_ARRAY('qs:admin'),'administrator',TRUE)`,
	} {
		if _, err := n.db.ExecContext(t.Context(), query); err != nil {
			t.Fatal(err)
		}
	}
	return n
}

func actorColumnCanonicalFacts(t *testing.T, n nativeSchemaPair) [][][]sql.NullString {
	t.Helper()
	return [][][]sql.NullString{
		historicalQuery(t, n.db, "SELECT id,org_id,profile_id,name,gender,birthday,source,is_key_focus,store_id,store_version,version,created_at,updated_at,deleted_at,created_by,updated_by,deleted_by FROM testee ORDER BY id"),
		historicalQuery(t, n.db, "SELECT "+actor.CurrentOperatorColumns+" FROM operators ORDER BY id"),
		historicalQuery(t, n.db, "SELECT * FROM operator_retirement_tasks ORDER BY operator_id"),
		historicalQuery(t, n.db, "SELECT * FROM operator_recovery_archives ORDER BY request_id"),
	}
}

func assertActorColumns(t *testing.T, n nativeSchemaPair, tags, effective bool) {
	t.Helper()
	for _, item := range []struct {
		table, column string
		present       bool
	}{{"testee", "tags", tags}, {"operators", "effective_roles", effective}} {
		var count int
		if err := n.db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=? AND column_name=?", item.table, item.column).Scan(&count); err != nil || (count == 1) != item.present {
			t.Fatal("actor object presence differs", err)
		}
	}
}

func actorColumnOfflineApproval(t *testing.T, n nativeSchemaPair, r *migrationResources) OfflineConfig {
	t.Helper()
	cfg := offlineNativeApproval(t, n, r)
	_, snapshot, err := inspectOffline(t.Context(), n.db, n.client, cfg.Pair, r)
	if err != nil {
		t.Fatal(err)
	}
	a, err := readOfflineAuthorization(cfg, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	a.ProtectionScope = offlineProtectionActorColumns
	writeOfflineApproval(t, &cfg, a)
	return cfg
}

func TestActorColumnsNativeUpgradeRestartAndHistoricalArchive(t *testing.T) {
	n := nativeActorColumnPair(t)
	db, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: n.db, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if actor.RequireCurrentOperatorPlanSchema(t.Context(), db) == nil {
		t.Fatal("new plans admitted the pre-DDL layout")
	}
	// The original raw retirement facts are captured on the genuine old schema.
	repo := retirement.NewRepository(db)
	now := time.Now().UTC()
	task, err := domain.New(7, 1, 800007, 900009, 1, "actor-exit", "reviewed synthetic exit", now)
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Begin(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	if err = task.MarkRevoked(10, now); err != nil {
		t.Fatal(err)
	}
	if err = repo.Save(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	if err = task.Complete(10, false, now); err != nil {
		t.Fatal(err)
	}
	if err = repo.Finish(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	var original string
	if err = n.db.QueryRowContext(t.Context(), "SELECT before_state FROM operator_retirement_tasks WHERE operator_id=7").Scan(&original); err != nil || !bytes.Contains([]byte(original), []byte("effective_roles")) {
		t.Fatal("old raw evidence was not captured", err)
	}
	before := actorColumnCanonicalFacts(t, n)
	mongoBefore, err := n.mongoDB.Collection("schema_migrations").FindOne(t.Context(), bson.D{}).Raw()
	if err != nil {
		t.Fatal(err)
	}
	r := migrationResourcesThrough(t, 102, 40)
	cfg := actorColumnOfflineApproval(t, n, r)
	result, err := runOffline(t.Context(), n.db, n.client, cfg, r)
	if err != nil || !result.Verified || !result.AuthorizationConsumed || result.MySQLChanged == nil || !*result.MySQLChanged || result.MongoChanged == nil || *result.MongoChanged {
		t.Fatal("actor offline upgrade failed", result, err)
	}
	assertActorColumns(t, n, false, false)
	if !reflect.DeepEqual(before, actorColumnCanonicalFacts(t, n)) {
		t.Fatal("column DDL changed retained facts or archive bytes")
	}
	mongoAfter, err := n.mongoDB.Collection("schema_migrations").FindOne(t.Context(), bson.D{}).Raw()
	if err != nil || !bytes.Equal(mongoBefore, mongoAfter) {
		t.Fatal("actor SQL release changed Mongo metadata", err)
	}
	if actor.RequireCurrentOperatorPlanSchema(t.Context(), db) == nil {
		t.Fatal("current SQL103 plans accepted the historical SQL102 release")
	}
	restart := n.preflight(t, r)
	for _, m := range []*Migrator{NewMigrator(n.db, restart.MySQLConfig(false)), NewMongoMigrator(n.client, restart.MongoConfig(false))} {
		if _, changed, err := m.Run(); err != nil || changed {
			t.Fatal("clean restart changed schema", err)
		}
	}
	if err := restart.VerifyComplete(t.Context()); err != nil {
		t.Fatal(err)
	}
	old := migrationResourcesThrough(t, 101, 40)
	if _, err := preflightCompatibilityPair(t.Context(), n.db, n.client, cfg.Pair, old); err == nil {
		t.Fatal("old binary accepted the future SQL head")
	}
	// Current repositories must also create records on the actual post-DDL shape.
	// These new facts are never covered by the earlier capture or restored over.
	op := operatorDomain.NewOperator(1, 800010, "post-DDL")
	op.SetID(10)
	if err := actor.NewOperatorRepository(db).Save(t.Context(), op); err != nil {
		t.Fatal("current Operator insert referenced the retired column", err)
	}
	subject := testeeDomain.NewTestee(1, "post-DDL", testeeDomain.GenderUnknown, nil)
	subject.SetID(10)
	if err := actor.NewTesteeRepository(db).Save(t.Context(), subject); err != nil {
		t.Fatal("current Testee insert referenced the retired column", err)
	}
	recovery, err := domain.NewRecovery(task, 900009, 3, 10, "actor-recovery", "reviewed synthetic identity recovery", now)
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.WithOperatorLock(t.Context(), 1, 7, func(ctx context.Context) error { return repo.Recover(ctx, recovery) }); err != nil {
		t.Fatal(err)
	}
	receipt, err := repo.FindRecovery(t.Context(), recovery.RequestID)
	if err != nil || receipt == nil || *receipt != recovery {
		t.Fatal("current recovery failed immutable archive validation", err)
	}
	var archived string
	if err = n.db.QueryRowContext(t.Context(), "SELECT retirement_before_state FROM operator_recovery_archives WHERE request_id=?", recovery.RequestID).Scan(&archived); err != nil || archived != original {
		t.Fatal("historical raw evidence was rewritten", err)
	}
}

func TestActorColumnsNativePartialDDLRefusesRestartForceAndDown(t *testing.T) {
	n := nativeActorColumnPair(t)
	before := actorColumnCanonicalFacts(t, n)
	release := migrationResourcesThrough(t, 102, 40)
	source := release.source.(fstest.MapFS)
	source[actorColumnsUp].Data = bytes.Replace(source[actorColumnsUp].Data, []byte("DROP COLUMN `effective_roles`"), []byte("DROP COLUMN `actor_missing_column`"), 1)
	r, err := readMigrationResources(source, release.pairs)
	if err != nil {
		t.Fatal(err)
	}
	p := n.preflight(t, r)
	cfg := actorColumnOfflineApproval(t, n, r)
	result, err := runOffline(t.Context(), n.db, n.client, cfg, r)
	if err == nil || result.Verified || result.Stage != "mysql" || !result.AuthorizationConsumed {
		t.Fatal("second real ALTER failure not retained", result, err)
	}
	assertActorColumns(t, n, false, true)
	if sql, dirty, mongo, mongoDirty := n.heads(t); sql != 102 || !dirty || mongo != 40 || mongoDirty {
		t.Fatal("partial actor DDL did not retain102dirty/40clean")
	}
	if !reflect.DeepEqual(before, actorColumnCanonicalFacts(t, n)) {
		t.Fatal("partial actor DDL changed canonical facts")
	}
	if _, err := preflightCompatibilityPair(t.Context(), n.db, n.client, cfg.Pair, release); err == nil {
		t.Fatal("fresh process adopted dirty actor DDL")
	}
	d := NewMySQLDriver(n.db)
	instance, err := d.CreateInstance(r.source, ensureConfigDefaults(p.MySQLConfig(false)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := d.finishRun(); err != nil {
			t.Error(err)
		}
	})
	for _, version := range []int{101, 102} {
		if instance.Force(version) == nil {
			t.Fatal("force accepted dirty actor DDL")
		}
	}
	down, err := migrations.ReadFile("migrations/mysql/000102_retire_actor_compatibility_columns.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	_, err = n.db.ExecContext(t.Context(), string(down))
	var refusal *driver.MySQLError
	if !errors.As(err, &refusal) || string(refusal.SQLState[:]) != "45000" {
		t.Fatal("actor down did not refuse", err)
	}
	if sql, dirty, mongo, mongoDirty := n.heads(t); sql != 102 || !dirty || mongo != 40 || mongoDirty {
		t.Fatal("refused actions changed the dirty ledger")
	}
}

// Exact synthetic value/schema material rehearsal only. Metadata stays the
// genuine clean101/40 fixture; this is not a dirty-ledger or production recovery.
func TestActorColumnsNativeExactObjectValueRestoration(t *testing.T) {
	n := nativeActorColumnPair(t)
	before := actorColumnCanonicalFacts(t, n)
	ddls := [][][]sql.NullString{historicalQuery(t, n.db, "SHOW CREATE TABLE testee"), historicalQuery(t, n.db, "SHOW CREATE TABLE operators")}
	values := [][][]sql.NullString{historicalQuery(t, n.db, "SELECT id,CAST(tags AS CHAR) FROM testee ORDER BY id"), historicalQuery(t, n.db, "SELECT id,CAST(effective_roles AS CHAR) FROM operators ORDER BY id")}
	up, err := migrations.ReadFile(actorColumnsUp)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = n.db.ExecContext(t.Context(), string(up)); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"ALTER TABLE testee ADD COLUMN tags JSON DEFAULT NULL COMMENT '标签（JSON数组）' AFTER birthday", "ALTER TABLE operators ADD COLUMN effective_roles JSON NULL AFTER roles"} {
		if _, err = n.db.ExecContext(t.Context(), query); err != nil {
			t.Fatal(err)
		}
	}
	for i, item := range []struct{ table, column string }{{"testee", "tags"}, {"operators", "effective_roles"}} {
		for _, row := range values[i] {
			if _, err = n.db.ExecContext(t.Context(), "UPDATE `"+item.table+"` SET `"+item.column+"`=?,updated_at=updated_at WHERE id=?", row[1], row[0].String); err != nil {
				t.Fatal(err)
			}
		}
		if !reflect.DeepEqual(values[i], historicalQuery(t, n.db, "SELECT id,CAST(`"+item.column+"` AS CHAR) FROM `"+item.table+"` ORDER BY id")) {
			t.Fatal("restored full DDL or exact values differ")
		}
	}
	if _, err = n.db.ExecContext(t.Context(), "ALTER TABLE operators MODIFY COLUMN effective_roles JSON NOT NULL"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ddls, [][][]sql.NullString{historicalQuery(t, n.db, "SHOW CREATE TABLE testee"), historicalQuery(t, n.db, "SHOW CREATE TABLE operators")}) {
		t.Fatal("restored full DDL differs")
	}
	assertActorColumns(t, n, true, true)
	if !reflect.DeepEqual(before, actorColumnCanonicalFacts(t, n)) {
		t.Fatal("object restoration changed retained facts")
	}
	if sql, dirty, mongo, mongoDirty := n.heads(t); sql != 101 || dirty || mongo != 40 || mongoDirty {
		t.Fatal("object-only rehearsal changed genuine migration metadata")
	}
}

func TestActorColumnsNativeOtherObjectScopesRefusedBeforeClaim(t *testing.T) {
	n := nativeActorColumnPair(t)
	r := migrationResourcesThrough(t, 102, 40)
	before := actorColumnCanonicalFacts(t, n)
	for _, scope := range []string{offlineProtectionSQLHistoricalColumns, offlineProtectionMongoHistoricalIndexes} {
		t.Run(scope, func(t *testing.T) {
			cfg := offlineNativeApproval(t, n, r)
			_, snapshot, err := inspectOffline(t.Context(), n.db, n.client, cfg.Pair, r)
			if err != nil {
				t.Fatal(err)
			}
			a, err := readOfflineAuthorization(cfg, snapshot)
			if err != nil {
				t.Fatal(err)
			}
			a.ProtectionScope = scope
			writeOfflineApproval(t, &cfg, a)
			result, err := runOffline(t.Context(), n.db, n.client, cfg, r)
			if err == nil || result.Stage != "approval" || result.AuthorizationConsumed || result.Verified {
				t.Fatal("other objects authorized Actor DDL", result, err)
			}
			if _, err := os.Lstat(cfg.AuthorizationFile + ".consumed"); !os.IsNotExist(err) {
				t.Fatal("wrong scope was claimed", err)
			}
			assertActorColumns(t, n, true, true)
			if !reflect.DeepEqual(before, actorColumnCanonicalFacts(t, n)) {
				t.Fatal("rejected scope changed retained facts")
			}
			if sql, dirty, mongo, mongoDirty := n.heads(t); sql != 101 || dirty || mongo != 40 || mongoDirty {
				t.Fatal("rejected scope changed the initial pair")
			}
		})
	}
}
