package pipeline

import (
	"context"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type reportSaverTxMarkerKey struct{}

type reportSaverRunnerStub struct {
	called bool
}

func (r *reportSaverRunnerStub) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	r.called = true
	return fn(context.WithValue(ctx, reportSaverTxMarkerKey{}, "tx"))
}

type reportSaverWriterStub struct {
	report    *domainReport.InterpretReport
	testeeID  testee.ID
	sawTxCtx  bool
	callCount int
	returnErr error
}

func (w *reportSaverWriterStub) SaveReportRecord(ctx context.Context, rpt *domainReport.InterpretReport, testeeID testee.ID) error {
	w.callCount++
	w.report = rpt
	w.testeeID = testeeID
	w.sawTxCtx = ctx.Value(reportSaverTxMarkerKey{}) == "tx"
	return w.returnErr
}

type reportSaverStagerStub struct {
	sawTxCtx   bool
	eventTypes []string
	callCount  int
	returnErr  error
}

func (s *reportSaverStagerStub) Stage(ctx context.Context, events ...event.DomainEvent) error {
	s.callCount++
	s.sawTxCtx = ctx.Value(reportSaverTxMarkerKey{}) == "tx"
	for _, evt := range events {
		s.eventTypes = append(s.eventTypes, evt.EventType())
	}
	return s.returnErr
}

func TestTransactionalReportDurableSaverStagesEventsInTransactionContext(t *testing.T) {
	runner := &reportSaverRunnerStub{}
	writer := &reportSaverWriterStub{}
	stager := &reportSaverStagerStub{}
	saver := NewTransactionalReportDurableSaver(runner, writer, stager)
	rpt := domainReport.NewInterpretReport(domainReport.ID(101), "Scale", "scale", 10, domainReport.RiskLevelLow, "ok", nil, nil)
	events := []event.DomainEvent{
		event.New("assessment.interpreted", "Assessment", "101", map[string]string{"id": "101"}),
		event.New("report.generated", "Report", "101", map[string]string{"id": "101"}),
	}

	if err := saver.SaveReportDurably(t.Context(), rpt, testee.NewID(202), events); err != nil {
		t.Fatalf("SaveReportDurably() error = %v", err)
	}
	if !runner.called || writer.callCount != 1 || stager.callCount != 1 {
		t.Fatalf("expected runner/writer/stager calls, got runner=%v writer=%d stager=%d", runner.called, writer.callCount, stager.callCount)
	}
	if writer.report != rpt || writer.testeeID != testee.NewID(202) {
		t.Fatalf("writer received wrong report/testee: %p/%d", writer.report, writer.testeeID)
	}
	if !writer.sawTxCtx || !stager.sawTxCtx {
		t.Fatalf("writer/stager must receive transaction context: writer=%v stager=%v", writer.sawTxCtx, stager.sawTxCtx)
	}
	if got := strings.Join(stager.eventTypes, ","); got != "assessment.interpreted,report.generated" {
		t.Fatalf("staged event order = %s", got)
	}
}

func TestTransactionalReportDurableSaverMissingDependenciesFailClosed(t *testing.T) {
	saver := NewTransactionalReportDurableSaver(nil, &reportSaverWriterStub{}, &reportSaverStagerStub{})
	rpt := domainReport.NewInterpretReport(domainReport.ID(101), "Scale", "scale", 10, domainReport.RiskLevelLow, "ok", nil, nil)

	err := saver.SaveReportDurably(t.Context(), rpt, testee.NewID(202), nil)
	if err == nil || !strings.Contains(err.Error(), "requires transaction runner") {
		t.Fatalf("SaveReportDurably() error = %v, want fail-closed dependency error", err)
	}
}
