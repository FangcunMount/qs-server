package systemgovernance

import (
	"context"
	"errors"
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
	indexDDL, e := os.ReadFile("../../../../../internal/pkg/migration/migrations/mysql/000085_system_governance_pending_replay_index.up.sql")
	if e != nil {
		t.Fatal(e)
	}
	if e = db.Exec(string(indexDDL)).Error; e != nil {
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
			original, found, e := store.LoadRunning(ctx, record)
			if e != nil || !found || original.ActorUserID != record.ActorUserID || original.StartedAt.Sub(record.StartedAt).Abs() > time.Millisecond {
				t.Fatalf("running audit did not return original record: %+v found=%t err=%v", original, found, e)
			}
			changed := record
			changed.Input = map[string]interface{}{"resource_id": "changed"}
			if _, found, e := store.LoadRunning(ctx, changed); found || !errors.Is(e, app.ErrActionAuditInputConflict) {
				t.Fatalf("changed input reused running audit: found=%t err=%v", found, e)
			}
			changed = record
			changed.ActorUserID++
			if _, found, e := store.LoadRunning(ctx, changed); found || !errors.Is(e, app.ErrActionAuditInputConflict) {
				t.Fatalf("changed actor reused running audit: found=%t err=%v", found, e)
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
			changed = record
			changed.Input = map[string]interface{}{"resource_id": "changed"}
			if _, _, e := store.Claim(ctx, changed); !errors.Is(e, app.ErrActionAuditInputConflict) {
				t.Fatalf("completed audit accepted changed input: %v", e)
			}
			if _, found, e := store.LoadRunning(ctx, record); e != nil || found {
				t.Fatalf("completed action still appeared running: found=%t err=%v", found, e)
			}
		})
	}
	ending := app.ActionAuditRecord{
		RequestID: "pending-replay", ActionID: "events.replay_pending", OrgID: 7, ActorUserID: 110004,
		Input: map[string]interface{}{"store": "assessment-mysql-outbox", "reason": "reviewed"}, StartedAt: time.Now(),
	}
	if prior, claimed, err := store.Claim(ctx, ending); err != nil || prior != nil || !claimed {
		t.Fatalf("claim pending replay: prior=%+v claimed=%t err=%v", prior, claimed, err)
	}
	changed := ending
	changed.Input = map[string]interface{}{"store": "another", "reason": "reviewed"}
	if err := store.MarkPending(ctx, changed); !errors.Is(err, app.ErrActionAuditInputConflict) {
		t.Fatalf("different replay input changed audit state: %v", err)
	}
	if err := store.MarkPending(ctx, ending); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkPending(ctx, ending); err != nil {
		t.Fatalf("marking same pending replay twice failed: %v", err)
	}
	if original, found, err := store.LoadRunning(ctx, ending); err != nil || !found || original.Status != app.ActionAuditStatusPendingReconciliation {
		t.Fatalf("pending replay identity was lost: original=%+v found=%t err=%v", original, found, err)
	}
	if prior, claimed, err := store.Claim(ctx, ending); err != nil || prior != nil || claimed {
		t.Fatalf("pending replay was reopened: prior=%+v claimed=%t err=%v", prior, claimed, err)
	}
	second := ending
	second.RequestID = "pending-replay-second"
	if prior, claimed, err := store.Claim(ctx, second); err != nil || prior != nil || !claimed {
		t.Fatalf("claim second pending replay: prior=%+v claimed=%t err=%v", prior, claimed, err)
	}
	if err := store.MarkPending(ctx, second); err != nil {
		t.Fatal(err)
	}
	otherOrg := ending
	otherOrg.OrgID = 8
	otherOrg.RequestID = "pending-other-org"
	if prior, claimed, err := store.Claim(ctx, otherOrg); err != nil || prior != nil || !claimed {
		t.Fatalf("claim other organization replay: prior=%+v claimed=%t err=%v", prior, claimed, err)
	}
	if err := store.MarkPending(ctx, otherOrg); err != nil {
		t.Fatal(err)
	}
	firstPage, err := store.ListPendingReplayAudits(ctx, 7, "", 1)
	if err != nil || len(firstPage.Items) != 1 || firstPage.Items[0].RequestID != second.RequestID ||
		firstPage.Items[0].Store != "assessment-mysql-outbox" || firstPage.Items[0].ActorUserID != "110004" || firstPage.NextCursor == "" {
		t.Fatalf("pending replay first page omitted original input or scope: page=%+v err=%v", firstPage, err)
	}
	secondPage, err := store.ListPendingReplayAudits(ctx, 7, firstPage.NextCursor, 1)
	if err != nil || len(secondPage.Items) != 1 || secondPage.Items[0].RequestID != ending.RequestID || secondPage.NextCursor != "" {
		t.Fatalf("pending replay cursor did not return second audit: page=%+v err=%v", secondPage, err)
	}
	if _, err := store.ListPendingReplayAudits(ctx, 7, "bad", 1); err == nil {
		t.Fatal("pending replay reader accepted invalid cursor")
	}
	ending.Status = "ok"
	ending.FinishedAt = time.Now()
	ending.Result = &app.ActionRunResult{RequestID: ending.RequestID, ActionID: ending.ActionID, Status: "ok"}
	if err := store.Complete(ctx, ending); err != nil {
		t.Fatalf("late durable result did not close pending replay: %v", err)
	}
	if prior, claimed, err := store.Claim(ctx, ending); err != nil || prior == nil || claimed {
		t.Fatalf("resolved pending replay was not repeatable: prior=%+v claimed=%t err=%v", prior, claimed, err)
	}
	remaining, err := store.ListPendingReplayAudits(ctx, 7, "", 10)
	if err != nil || len(remaining.Items) != 1 || remaining.Items[0].RequestID != second.RequestID {
		t.Fatalf("resolved audit still appears pending or another tenant leaked: page=%+v err=%v", remaining, err)
	}
}
