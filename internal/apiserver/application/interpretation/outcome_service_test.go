package interpretation

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/pkg/event"
)

var errReportBuild = errors.New("report build failed")

type outcomeRepoForReport struct {
	record *domainoutcome.Record
	reads  int
}

func (r *outcomeRepoForReport) Save(context.Context, *domainoutcome.Record) error { return nil }
func (r *outcomeRepoForReport) FindByID(context.Context, domainoutcome.ID) (*domainoutcome.Record, error) {
	r.reads++
	return r.record, nil
}
func (r *outcomeRepoForReport) FindByAssessmentID(context.Context, meta.ID) (*domainoutcome.Record, error) {
	r.reads++
	return r.record, nil
}

type reportStateStoreStub struct {
	report   *domainreport.InterpretReport
	statuses []domainreport.ReportStatus
	attempts []uint
}

func (s *reportStateStoreStub) FindByID(context.Context, domainreport.ID) (*domainreport.InterpretReport, error) {
	if s.report == nil {
		return nil, domainreport.ErrReportNotFound
	}
	return s.report, nil
}
func (s *reportStateStoreStub) SaveState(_ context.Context, rpt *domainreport.InterpretReport, _ testee.ID) error {
	s.report = rpt
	s.statuses = append(s.statuses, rpt.Status())
	s.attempts = append(s.attempts, rpt.Attempt())
	return nil
}

type failThenGenerate struct{ calls int }

func (g *failThenGenerate) Generate(_ context.Context, outcome evaloutcome.Outcome) (interpretationreporting.Generation, error) {
	g.calls++
	if outcome.Assessment == nil || !outcome.Assessment.Status().IsEvaluated() {
		return interpretationreporting.Generation{}, errors.New("outcome context missing")
	}
	if g.calls == 1 {
		return interpretationreporting.Generation{}, errReportBuild
	}
	return interpretationreporting.Generation{Report: domainreport.NewInterpretReport(domainreport.ID(outcome.AssessmentID()), "Scale", "S-1", 12, domainreport.RiskLevelLow, "ok", nil, nil, nil)}, nil
}

type alwaysGenerate struct{ calls int }

func (g *alwaysGenerate) Generate(_ context.Context, outcome evaloutcome.Outcome) (interpretationreporting.Generation, error) {
	g.calls++
	if outcome.Assessment == nil || !outcome.Assessment.Status().IsEvaluated() {
		return interpretationreporting.Generation{}, errors.New("outcome context missing")
	}
	return interpretationreporting.Generation{Report: domainreport.NewInterpretReport(domainreport.ID(outcome.AssessmentID()), "Scale", "S-1", 12, domainreport.RiskLevelLow, "ok", nil, nil, nil)}, nil
}

type durableReportSaverStub struct {
	calls      int
	testeeID   testee.ID
	events     [][]event.DomainEvent
	stateStore *reportStateStoreStub
}

func (s *durableReportSaverStub) SaveReportDurably(_ context.Context, rpt *domainreport.InterpretReport, testeeID testee.ID, events []event.DomainEvent) error {
	s.calls++
	s.testeeID = testeeID
	s.events = append(s.events, append([]event.DomainEvent(nil), events...))
	if s.stateStore != nil {
		return s.stateStore.SaveState(context.Background(), rpt, testeeID)
	}
	return nil
}

func TestOutcomeReportRetryReadsPersistedOutcomeAndAdvancesIndependentAttempt(t *testing.T) {
	record := reportOutcomeRecord(t)
	outcomes := &outcomeRepoForReport{record: record}
	states := &reportStateStoreStub{}
	generator := &failThenGenerate{}
	saver := &durableReportSaverStub{stateStore: states}
	svc := NewOutcomeReportService(outcomes, states, generator, saver)

	failed, err := svc.GenerateByOutcomeID(context.Background(), record.ID())
	if !errors.Is(err, errReportBuild) {
		t.Fatalf("first generation error = %v", err)
	}
	if failed.Status() != domainreport.ReportStatusFailed || failed.Attempt() != 1 || failed.FailureReason() != errReportBuild.Error() {
		t.Fatalf("failed report = %#v", failed)
	}

	generated, err := svc.GenerateByOutcomeID(context.Background(), record.ID())
	if err != nil {
		t.Fatalf("retry generation: %v", err)
	}
	if generated.Status() != domainreport.ReportStatusGenerated || generated.Attempt() != 2 || generated.OutcomeID() != record.ID() {
		t.Fatalf("generated report = %#v", generated)
	}
	if outcomes.reads != 2 || generator.calls != 2 || saver.calls != 2 || saver.testeeID.Uint64() != record.TesteeID() {
		t.Fatalf("reads=%d generator=%d saver=%d testee=%d", outcomes.reads, generator.calls, saver.calls, saver.testeeID.Uint64())
	}
	if len(saver.events) != 2 || len(saver.events[0]) != 1 || saver.events[0][0].EventType() != eventcatalog.InterpretationReportFailed {
		t.Fatalf("failure outbox event = %#v", saver.events)
	}
	wantStatuses := []domainreport.ReportStatus{domainreport.ReportStatusPending, domainreport.ReportStatusGenerating, domainreport.ReportStatusFailed, domainreport.ReportStatusGenerating}
	for i, want := range wantStatuses {
		if states.statuses[i] != want {
			t.Fatalf("statuses = %#v", states.statuses)
		}
	}
}

// This characterizes the public idempotency behavior that the three-object
// model must retain: once an artifact has been generated, duplicate delivery
// returns it without rebuilding or staging a second terminal event.
func TestOutcomeReportDuplicateDeliveryReturnsGeneratedArtifactWithoutRebuilding(t *testing.T) {
	record := reportOutcomeRecord(t)
	outcomes := &outcomeRepoForReport{record: record}
	states := &reportStateStoreStub{}
	generator := &alwaysGenerate{}
	saver := &durableReportSaverStub{stateStore: states}
	svc := NewOutcomeReportService(outcomes, states, generator, saver)

	first, err := svc.GenerateByOutcomeID(context.Background(), record.ID())
	if err != nil {
		t.Fatalf("first generation: %v", err)
	}
	second, err := svc.GenerateByOutcomeID(context.Background(), record.ID())
	if err != nil {
		t.Fatalf("duplicate generation: %v", err)
	}

	if first != second || second.Status() != domainreport.ReportStatusGenerated {
		t.Fatalf("duplicate delivery returned unexpected artifact: first=%p second=%p status=%s", first, second, second.Status())
	}
	if generator.calls != 1 || saver.calls != 1 {
		t.Fatalf("duplicate delivery rebuilt report: generator=%d saver=%d", generator.calls, saver.calls)
	}
	if outcomes.reads != 2 {
		t.Fatalf("outcome reads = %d, want one read per delivery", outcomes.reads)
	}
}

func reportOutcomeRecord(t *testing.T) *domainoutcome.Record {
	t.Helper()
	execution := domainoutcome.NewExecution(
		domainoutcome.ModelRef{ModelKind: modelcatalog.KindScale, ModelCode: "S-1", ModelVersion: "1.0.0", ModelTitle: "Scale"},
		domainoutcome.Summary{PrimaryLabel: "low"}, domainoutcome.Detail{Kind: modelcatalog.KindScale},
	)
	execution.Primary = &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindRawTotal, Value: 12}
	payload, err := json.Marshal(execution)
	if err != nil {
		t.Fatal(err)
	}
	record, err := domainoutcome.NewRecord(domainoutcome.NewRecordInput{
		ID: meta.FromUint64(9), OrgID: 11, AssessmentID: meta.FromUint64(7), TesteeID: 8, RunID: "7:1",
		Model:   domainoutcome.ModelIdentity{Kind: modelcatalog.KindScale, Code: "S-1", Version: "1.0.0", Title: "Scale"},
		Runtime: domainoutcome.RuntimeIdentity{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring, DecisionKind: modelcatalog.DecisionKindScoreRange},
		Payload: payload, EvaluatedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return record
}
