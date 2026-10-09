package retirement

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestCoordinatorOriginBindingPreservesOriginalExpectedBoundaryAndCursors(t *testing.T) {
	f := sourceAuthFixture(t, true, true)
	c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), DefaultHistoricalCoordinatorLimits())
	if err != nil {
		t.Fatal(err)
	}
	binding, err := c.BindOriginCopies(t.Context(), f.inputs(), DefaultSourceOriginLimits())
	if err != nil {
		t.Fatal(err)
	}
	if binding.copies != c.authenticated {
		t.Fatal("detached source authentication")
	}
	if _, err := json.Marshal(binding); !errors.Is(err, ErrSourceSerialization) {
		t.Fatal("opaque capability serialized", err)
	}
	page, err := c.NextPage(t.Context())
	if err != nil {
		t.Fatal("origin reread disturbed coordinator cursors", err)
	}
	if _, err = page.Events(); err != nil {
		t.Fatal(err)
	}
	// SourceAuth did not retain the original metadata. This coordinator did retain
	// its supplied expectation: a newly self-consistent header cannot replace it.
	changed := f
	changed.expected[0].Boundary.SchemaHash = strings.Repeat("c", 64)
	for _, kind := range []string{"boundary", "count", "swap", "missing"} {
		t.Run(kind, func(t *testing.T) {
			inputs := f.inputs()
			switch kind {
			case "boundary":
				inputs[0].Expected = changed.expected[0]
			case "count":
				inputs[0].Expected.Records++
			case "swap":
				inputs[1], inputs[2] = inputs[2], inputs[1]
			case "missing":
				inputs = inputs[:3]
			}
			if _, err := c.BindOriginCopies(t.Context(), inputs, DefaultSourceOriginLimits()); err == nil {
				t.Fatal("original expected scope replaced")
			}
		})
	}
}

func TestCoordinatorOriginCannotBypassActualSnapshotsOrHostLifetimes(t *testing.T) {
	var c *HistoricalCoordinator
	if _, err := c.BindOriginCopies(t.Context(), nil, DefaultSourceOriginLimits()); err == nil {
		t.Fatal("nil coordinator accepted")
	}
	for _, sql := range []*SQLResponsibilitySnapshot{nil, {}} {
		if _, err := PrepareSourceOriginSnapshotEpoch(t.Context(), nil, sql, nil, []io.Reader{}); err == nil {
			t.Fatal("editable snapshot accepted")
		}
		var epoch *SourceOriginEpoch
		if _, err := epoch.RecheckSnapshots(t.Context(), sql, nil, nil); err == nil {
			t.Fatal("nil origin accepted")
		}
	}
}
