package retirement

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCoordinatorMongoCASEditableCompletionCannotAuthorize(t *testing.T) {
	var c *HistoricalCoordinator
	if p, e := c.PrepareMongoCAS(context.Background(), nil); p != nil || !errors.Is(e, ErrCoordinatorInvalid) {
		t.Fatal(e)
	}
	now := time.Now()
	c = &HistoricalCoordinator{coverage: true, authenticated: &VerifiedSourceCopies{}, started: now, now: func() time.Time { return now }, limits: HistoricalCoordinatorLimits{MaxDuration: time.Minute}}
	b := &MongoHistoricalOwnerBatch{global: &MongoResponsibilitySnapshot{}}
	if p, e := c.PrepareMongoCAS(context.Background(), b); p != nil || !errors.Is(e, ErrCoordinatorMongoCASUnqualified) {
		t.Fatal("coverage authorized write", e)
	}
	r := c.MongoCASReadiness()
	r.Required = nil
	r.CASAuthorized = true
	if actual := c.MongoCASReadiness(); actual.CASAuthorized || actual.DropReady || len(actual.Required) == 0 {
		t.Fatal("DTO authorized write")
	}
	c.coverage = false
	if _, e := c.PrepareMongoCAS(context.Background(), b); !errors.Is(e, ErrCoordinatorIncomplete) {
		t.Fatal(e)
	}
	c.coverage = true
	c.started = now.Add(-2 * time.Minute)
	if c.MongoCASReadiness().WholeFourCopyCoverage {
		t.Fatal("expired EOF reused")
	}
}
