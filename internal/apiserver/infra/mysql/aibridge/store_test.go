package aibridge

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sync"
	"testing"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

func fixture(t *testing.T) (*Store, app.Start) {
	t.Helper()
	dsn := os.Getenv("QS_AI_BRIDGE_TEST_DSN")
	if dsn == "" {
		t.Skip("requires disposable QS MySQL with bridge migration")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	r := app.Start{RequestID: uuid.NewString(), Actor: app.Actor{OrgID: "1", SubjectID: "42"}, TesteeID: "7", AssessmentIDs: []string{"9"}, Goal: "fixture"}
	t.Cleanup(func() {
		for _, table := range []string{"ai_bridge_events", "ai_bridge_commands", "ai_bridge_requests"} {
			if _, e := db.Exec("DELETE FROM "+table+" WHERE request_id=?", r.RequestID); e != nil {
				t.Error(e)
			}
		}
		db.Close()
	})
	return &Store{DB: db}, r
}
func TestConcurrentStartAndChangedPayload(t *testing.T) {
	store, r := fixture(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- store.StageStart(ctx, r) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := store.DB.QueryRow("SELECT COUNT(*) FROM ai_bridge_commands WHERE request_id=?", r.RequestID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	r.Goal = "changed"
	if err := store.StageStart(ctx, r); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("want conflict, got %v", err)
	}
}
func TestProjectionOutOfOrderDuplicateAndTerminalGuard(t *testing.T) {
	store, r := fixture(t)
	ctx := context.Background()
	if err := store.StageStart(ctx, r); err != nil {
		t.Fatal(err)
	}
	e := app.Event{EventID: uuid.NewString(), RequestID: r.RequestID, SessionID: uuid.NewString(), Actor: r.Actor, TesteeID: r.TesteeID, Version: 8, Status: "cancelled"}
	if err := store.Accept(ctx, e); err != nil {
		t.Fatal(err)
	}
	if err := store.Accept(ctx, e); err != nil {
		t.Fatal(err)
	}
	old := e
	old.EventID = uuid.NewString()
	old.Version = 3
	old.Status = "running"
	if err := store.Accept(ctx, old); err != nil {
		t.Fatal(err)
	}
	got, err := store.Projection(ctx, r.RequestID)
	if err != nil || got.Version != 8 || got.Status != "cancelled" {
		t.Fatalf("projection=%+v err=%v", got, err)
	}
	duplicate := e
	duplicate.FailureCode = "different"
	if err = store.Accept(ctx, duplicate); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("want conflict: %v", err)
	}
	late := old
	late.EventID = uuid.NewString()
	late.Version = 9
	if err = store.Accept(ctx, late); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("want terminal guard: %v", err)
	}
	wrong := old
	wrong.Actor.SubjectID = "other"
	if err = store.Accept(ctx, wrong); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("want actor guard: %v", err)
	}
	commands, err := store.Pending(ctx, 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range commands {
		if command.RequestID == r.RequestID {
			if err = store.Acknowledge(ctx, command, app.Receipt{SessionID: e.SessionID, Version: 2, Status: "queued"}); err != nil {
				t.Fatal(err)
			}
		}
	}
	got, err = store.Projection(ctx, r.RequestID)
	if err != nil || got.Version != 8 {
		t.Fatalf("ack regressed projection: %+v %v", got, err)
	}
}
func TestUnknownRequestAndWrongSessionAreRejected(t *testing.T) {
	store, r := fixture(t)
	ctx := context.Background()
	e := app.Event{EventID: uuid.NewString(), RequestID: r.RequestID, SessionID: uuid.NewString(), Actor: r.Actor, TesteeID: r.TesteeID, Version: 2, Status: "queued"}
	if err := store.Accept(ctx, e); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("expected not found: %v", err)
	}
	if err := store.StageStart(ctx, r); err != nil {
		t.Fatal(err)
	}
	if err := store.Accept(ctx, e); err != nil {
		t.Fatal(err)
	}
	e.SessionID = uuid.NewString()
	e.EventID = uuid.NewString()
	e.Version++
	if err := store.Accept(ctx, e); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("expected association conflict: %v", err)
	}
}
