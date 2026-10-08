package retirement

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

func TestHistoricalCoordinatorCrossStoreMissingOpaqueQualificationDoesNotConsume(t *testing.T) {
	f := coordinatorFixture(t, 0, false)
	c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), DefaultHistoricalCoordinatorLimits())
	if err != nil {
		t.Fatal(err)
	}
	p, err := c.NextPage(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"missing", "empty_exported_dto", "different_coordinator", "cancelled_context"} {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			page := p
			cross := &SQLMongoCrossStoreResponsibilityPage{}
			switch name {
			case "missing":
				cross = nil
			case "different_coordinator":
				page = &HistoricalSourcePage{owner: &HistoricalCoordinator{}, sequence: p.sequence}
			case "cancelled_context":
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			}
			if got, err := c.QualifyCrossStorePage(ctx, page, nil, nil, cross); got != nil || err == nil {
				t.Fatal("missing private qualification accepted")
			}
			if p.consumed || len(c.pages) != 0 || len(c.candidates) != 0 || len(c.remaining) != 6 || c.Receipt().SourceCoverageComplete {
				t.Fatal("invalid page consumed source facts")
			}
			if _, err := c.CandidateRange(0, 1); !errors.Is(err, ErrCoordinatorIncomplete) {
				t.Fatal("partial copy qualification exported")
			}
		})
	}
	if err := c.QualifyPage(t.Context(), p, nil, nil); err != nil {
		t.Fatal("rejected cross-store input prevented safe blocked original consumption", err)
	}
	if next, err := c.NextPage(t.Context()); next != nil || err != io.EOF {
		t.Fatal(err)
	}
	if got, err := c.QualifyCrossStorePage(t.Context(), p, nil, nil, nil); got != nil || err == nil {
		t.Fatal("consumed source page replay accepted")
	}
}

func TestHistoricalCoordinatorCrossStoreOpaqueJournalCannotSerializeOrPretendComplete(t *testing.T) {
	journal := &HistoricalCoordinatorCrossStorePage{}
	for _, input := range []any{journal, HistoricalCrossStoreObservationBinding{OriginalID: "private"}} {
		if _, err := json.Marshal(input); !errors.Is(err, ErrSourceSerialization) {
			t.Fatal("private qualification JSON serialization allowed")
		}
		if _, err := bson.Marshal(input); !errors.Is(err, ErrSourceSerialization) {
			t.Fatal("private qualification BSON serialization allowed")
		}
	}
	for _, journal := range []*HistoricalCoordinatorCrossStorePage{nil, journal, {owner: &HistoricalCoordinator{}}} {
		s := journal.Summary()
		if s.WholeFourSourceCoverageBound || s.DropReady || !s.ExternalOriginRequired || !s.AIInboxRequired || !s.GlobalUnboundRequired || !s.FinalFreshRequired || !s.WriterFenceRequired || !s.CASRequired {
			t.Fatal("opaque ticket fabricated closure")
		}
		if _, err := journal.ObservationBindingRange(0, 1); err == nil {
			t.Fatal("unminted observation journal exported")
		}
	}
	if strings.Contains(journal.String(), "private") == false || strings.Contains((HistoricalCrossStoreObservationBinding{OriginalID: "sensitive-id"}).String(), "sensitive-id") {
		t.Fatal("debug representation leaks source identity")
	}
}

func TestHistoricalCoordinatorCrossStoreObservationRequiresEveryOriginalFact(t *testing.T) {
	facts := &DecodedSourceEvent{EventID: "event", EventType: "interpretation.report.generated", OrgID: 7}
	local := MongoLocalResolution{EventID: "event", EventType: "interpretation.report.generated", OrgID: 7}
	view := SQLMongoCrossStoreResponsibilityView{EventID: "event", EventType: "interpretation.report.generated", OrganizationID: 7, CurrentSQLCoverageObserved: true, SourceCopyFactsBound: true}
	// Missing a real cross page/batch is deliberately never enough to mint a
	// binding. Every mismatch is rejected before any private capability use.
	for _, name := range []string{"source_database", "original_id", "original_type", "organization", "row_hash", "legacy_hash", "binding_hash", "assessment", "source_bound", "coverage", "drop_true"} {
		t.Run(name, func(t *testing.T) {
			f := *facts
			f.Source.Database = "mongodb"
			v := view
			switch name {
			case "source_database":
				f.Source.Database = "mysql"
			case "original_id":
				v.EventID = "another"
			case "original_type":
				v.EventType = "answersheet.submitted"
			case "organization":
				v.OrganizationID = 8
			case "row_hash":
				v.SourceRowSHA256 = strings.Repeat("1", 64)
			case "legacy_hash":
				v.LegacyContentSHA256 = strings.Repeat("1", 64)
			case "binding_hash":
				v.BusinessBindingSHA256 = strings.Repeat("1", 64)
			case "assessment":
				v.AssessmentID = 42
			case "source_bound":
				v.SourceCopyFactsBound = false
			case "coverage":
				v.CurrentSQLCoverageObserved = false
			case "drop_true":
				v.DropReady = true
			}
			if _, err := coordinatorCrossStoreObservationBinding(&f, local, v, nil, nil); !errors.Is(err, ErrCoordinatorPage) {
				t.Fatal("original source mismatch accepted", err)
			}
		})
	}
}

func TestHistoricalCoordinatorCrossStorePageLifetimeAndAIPageAreRejected(t *testing.T) {
	f := coordinatorFixture(t, 1, true)
	limits := DefaultHistoricalCoordinatorLimits()
	limits.MaxPageRecords = 4
	c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), limits)
	if err != nil {
		t.Fatal(err)
	}
	p, err := c.NextPage(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := c.QualifyPage(t.Context(), p, nil, nil); err != nil {
		t.Fatal(err)
	}
	ai, err := c.NextPage(t.Context())
	if err != nil || ai.rows[0].bridge == nil {
		t.Fatal("actual AI source page required", err)
	}
	if journal, err := c.QualifyCrossStorePage(t.Context(), ai, nil, nil, nil); journal != nil || err == nil || ai.consumed {
		t.Fatal("AI rows passed Mongo two-type cross-store path")
	}
	now := c.now()
	c.now = func() time.Time { return now.Add(limits.PageTTL + time.Millisecond) }
	if _, err := c.QualifyCrossStorePage(t.Context(), ai, nil, nil, nil); !errors.Is(err, ErrCoordinatorExpired) {
		t.Fatal("expired source page accepted", err)
	}
}
