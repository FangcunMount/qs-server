package systemgovernance

import (
	"context"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	mysqlDriver "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"os"
	"strings"
	"testing"
	"time"
)

// Exercise the production migration DDL against an isolated MySQL connection.
func TestActionAuditLifecycleWithJSONColumns(t *testing.T) {
	dsn := os.Getenv("QS_SERVER_TEST_MYSQL_DSN")
	if dsn == "" {
		if os.Getenv("QS_ACTION_AUDIT_REQUIRE_MYSQL") == "true" {
			t.Fatal("QS_SERVER_TEST_MYSQL_DSN is required")
		}
		t.Skip("MySQL lifecycle contract runs in the required CI database step")
	}
	db, err := gorm.Open(mysqlDriver.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, e := db.DB()
	if e != nil {
		t.Fatal(e)
	}
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = pool.Close() })
	ddl, e := os.ReadFile("../../../../../internal/pkg/migration/migrations/mysql/000048_add_system_governance_action_runs.up.sql")
	if e != nil {
		t.Fatal(e)
	}
	if e = db.Exec(strings.Replace(string(ddl), "CREATE TABLE", "CREATE TEMPORARY TABLE", 1)).Error; e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	store := NewActionAuditStore(db)
	for _, tc := range []struct {
		name    string
		result  *app.ActionRunResult
		failure *app.ActionAuditError
	}{
		{name: "success", result: &app.ActionRunResult{ActionID: "evaluation.retry", Status: "succeeded"}},
		{name: "failure", failure: &app.ActionAuditError{Code: 400, Message: "invalid state"}},
		{name: "no_payload"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now().UTC()
			record := app.ActionAuditRecord{RequestID: tc.name, ActionID: "evaluation.retry", OrgID: 1, ActorUserID: 110004, Input: map[string]interface{}{"resource_id": "test"}, StartedAt: now}
			replay, claimed, e := store.Claim(ctx, record)
			if e != nil || !claimed || replay != nil {
				t.Fatalf("initial claim: %+v %v %v", replay, claimed, e)
			}
			replay, claimed, e = store.Claim(ctx, record)
			if e != nil || claimed || replay != nil {
				t.Fatalf("running replay: %+v %v %v", replay, claimed, e)
			}
			record.Status = "succeeded"
			if tc.failure != nil {
				record.Status = "failed"
			}
			record.Result = tc.result
			record.Error = tc.failure
			record.FinishedAt = now
			if e = store.Complete(ctx, record); e != nil {
				t.Fatal(e)
			}
			if e = store.Complete(ctx, record); e != nil {
				t.Fatalf("repeat complete: %v", e)
			}
			replay, claimed, e = store.Claim(ctx, record)
			if e != nil || claimed || replay == nil {
				t.Fatalf("completed replay: %+v %v %v", replay, claimed, e)
			}
			if (replay.Result == nil) != (tc.result == nil) || (replay.Error == nil) != (tc.failure == nil) {
				t.Fatalf("replay payload changed: %+v", replay)
			}
			if replay.ActionID != record.ActionID {
				t.Fatalf("wrong replay action: %s", replay.ActionID)
			}
		})
	}
}
