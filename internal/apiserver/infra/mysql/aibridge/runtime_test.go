package aibridge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"os"
	"testing"
	"time"
)

func TestRuntimeIndexAtomicScopeAndStablePagination(t *testing.T) {
	s, a := fixture(t)
	_, b := fixture(t)
	_, foreign := fixture(t)
	ctx := context.Background()
	foreign.Actor.OrgID = "2"
	for _, r := range []app.Start{a, b, foreign} {
		if e := s.StageStart(ctx, r); e != nil {
			t.Fatal(e)
		}
	}
	// Identical timestamps exercise the request-ID tiebreaker, not insertion order.
	stamp := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	if _, e := s.DB.Exec("UPDATE ai_bridge_requests SET created_at=? WHERE request_id IN (?,?)", stamp, a.RequestID, b.RequestID); e != nil {
		t.Fatal(e)
	}
	q := app.RuntimeQuery{AssessmentID: "9", Limit: 1}
	first, e := s.ListRuntime(ctx, 1, q)
	if e != nil || len(first.Items) != 1 || first.NextCursor == "" {
		t.Fatal(first, e)
	}
	if first.Items[0].SessionID != "" || first.Items[0].CommandsPending != 1 {
		t.Fatal(first)
	}
	q.Cursor = first.NextCursor
	second, e := s.ListRuntime(ctx, 1, q)
	if e != nil || len(second.Items) != 1 || second.NextCursor != "" || second.Items[0].RequestID == first.Items[0].RequestID {
		t.Fatal(second, e)
	}
	if _, e = s.GetRuntime(ctx, 1, foreign.RequestID); !errors.Is(e, app.ErrNotFound) {
		t.Fatal(e)
	}
	if _, e = s.ListRuntime(ctx, 2, q); !errors.Is(e, app.ErrInvalid) {
		t.Fatal("cross-organization cursor", e)
	}
	q.TesteeID = "8"
	if _, e = s.ListRuntime(ctx, 1, q); !errors.Is(e, app.ErrInvalid) {
		t.Fatal("changed filters", e)
	}
	changed := a
	changed.AssessmentIDs = []string{"99"}
	if e = s.StageStart(ctx, changed); !errors.Is(e, app.ErrConflict) {
		t.Fatal(e)
	}
	var n int
	if e = s.DB.QueryRow("SELECT COUNT(*) FROM ai_bridge_request_assessments WHERE request_id=? AND assessment_id=99", a.RequestID).Scan(&n); e != nil || n != 0 {
		t.Fatal("conflict leaked association", n, e)
	}
}

func TestRuntimeHistoricalBackfillResumesWithoutInventingDates(t *testing.T) {
	s, a := fixture(t)
	_, b := fixture(t)
	ctx := context.Background()
	for _, r := range []app.Start{a, b} {
		if e := s.StageStart(ctx, r); e != nil {
			t.Fatal(e)
		}
		if _, e := s.DB.Exec("UPDATE ai_bridge_requests SET organization_id=NULL,subject_id=NULL,testee_id=NULL,created_at=NULL,updated_at=NULL WHERE request_id=?", r.RequestID); e != nil {
			t.Fatal(e)
		}
		if _, e := s.DB.Exec("DELETE FROM ai_bridge_request_assessments WHERE request_id=?", r.RequestID); e != nil {
			t.Fatal(e)
		}
	}
	for i := 0; i < 2; i++ {
		if n, e := s.BackfillRuntimeIndexes(ctx, 1); e != nil || n != 1 {
			t.Fatal(n, e)
		}
	}
	if n, e := s.BackfillRuntimeIndexes(ctx, 1); e != nil || n != 0 {
		t.Fatal(n, e)
	}
	page, e := s.ListRuntime(ctx, 1, app.RuntimeQuery{})
	if e != nil || len(page.Items) != 0 {
		t.Fatal(page, e)
	}
	page, e = s.ListRuntime(ctx, 1, app.RuntimeQuery{History: true})
	if e != nil || len(page.Items) != 2 {
		t.Fatal(page, e)
	}
	for _, r := range page.Items {
		if r.CreatedAt != nil || r.UpdatedAt != nil || len(r.AssessmentIDs) != 1 {
			t.Fatal(r)
		}
	}
	exact, e := s.GetRuntime(ctx, 1, a.RequestID)
	if e != nil || exact.TesteeID != "7" {
		t.Fatal(exact, e)
	}
}

func TestRuntimeBackfillRejectsCorruptScopeWithoutPartialWrites(t *testing.T) {
	s, a := fixture(t)
	_, b := fixture(t)
	ctx := context.Background()
	for _, r := range []app.Start{a, b} {
		if e := s.StageStart(ctx, r); e != nil {
			t.Fatal(e)
		}
	}
	b.Actor.OrgID = "invalid"
	raw, _ := json.Marshal(b)
	if _, e := s.DB.Exec("UPDATE ai_bridge_requests SET payload=? WHERE request_id=?", raw, b.RequestID); e != nil {
		t.Fatal(e)
	}
	if _, e := s.DB.Exec("UPDATE ai_bridge_requests SET organization_id=NULL WHERE request_id IN (?,?)", a.RequestID, b.RequestID); e != nil {
		t.Fatal(e)
	}
	if _, e := s.BackfillRuntimeIndexes(ctx, 100); !errors.Is(e, app.ErrInvalid) {
		t.Fatal(e)
	}
	var n int
	if e := s.DB.QueryRow("SELECT COUNT(*) FROM ai_bridge_requests WHERE request_id IN (?,?) AND organization_id IS NULL", a.RequestID, b.RequestID).Scan(&n); e != nil || n != 2 {
		t.Fatal(n, e)
	}
}

func TestRuntimeUTCDoesNotDependOnDriverLocation(t *testing.T) {
	original, r := fixture(t)
	ctx := context.Background()
	db, err := sql.Open("mysql", os.Getenv("QS_AI_BRIDGE_TEST_DSN")+"&loc=Asia%2FShanghai")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	store := &Store{DB: db}
	before := time.Now().UTC().Add(-time.Second)
	if err = store.StageStart(ctx, r); err != nil {
		t.Fatal(err)
	}
	page, err := store.ListRuntime(ctx, 1, app.RuntimeQuery{Since: before, Until: time.Now().UTC().Add(time.Second)})
	if err != nil || len(page.Items) != 1 || page.Items[0].CreatedAt.Before(before) {
		t.Fatal(page, err)
	}
	correct, err := original.GetRuntime(ctx, 1, r.RequestID)
	if err != nil || !correct.CreatedAt.Equal(*page.Items[0].CreatedAt) {
		t.Fatal(correct, err)
	}
}
