package retirement

import (
	"context"
	"errors"
	"testing"
	"time"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
)

func TestCoordinatorSQLCASNeverAcceptsEditableCompletion(t *testing.T) {
	var c *HistoricalCoordinator
	if plan, e := c.PrepareSQLCAS(context.Background(), nil); plan != nil || !errors.Is(e, ErrCoordinatorInvalid) {
		t.Fatal(e)
	}
	// Even a package-level positive coverage fixture cannot supply the actual
	// missing production origin, joint responsibility and writer fence handles.
	now := time.Now()
	c = &HistoricalCoordinator{coverage: true, authenticated: &VerifiedSourceCopies{}, started: now, now: func() time.Time { return now }, limits: HistoricalCoordinatorLimits{MaxDuration: time.Minute}}
	b := &SQLBusinessOwnerBatch{facts: &sqlevaluation.SQLHistoricalOwnerBatch{}}
	if plan, e := c.PrepareSQLCAS(context.Background(), b); plan != nil || !errors.Is(e, ErrCoordinatorSQLCASUnqualified) {
		t.Fatal("coverage boolean became authorization", e)
	}
	r := c.SQLCASReadiness()
	r.Required = nil
	r.CASAuthorized = true
	actual := c.SQLCASReadiness()
	if actual.CASAuthorized || actual.DropReady || len(actual.Required) == 0 {
		t.Fatal("editable readiness mutated private gate")
	}
	c.coverage = false
	if _, e := c.PrepareSQLCAS(context.Background(), b); !errors.Is(e, ErrCoordinatorIncomplete) {
		t.Fatal("partial four-source EOF became CAS", e)
	}
}
