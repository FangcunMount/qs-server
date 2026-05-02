package pipeline

import (
	"context"
	"fmt"

	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type ReportDurableSaver interface {
	SaveReportDurably(ctx context.Context, rpt *domainReport.InterpretReport, testeeID testee.ID, events []event.DomainEvent) error
}

type ReportDurableWriter interface {
	SaveReportRecord(ctx context.Context, rpt *domainReport.InterpretReport, testeeID testee.ID) error
}

type ReportEventStager interface {
	Stage(ctx context.Context, events ...event.DomainEvent) error
}

type transactionalReportDurableSaver struct {
	runner apptransaction.Runner
	writer ReportDurableWriter
	stager ReportEventStager
}

func NewReportDurableSaver(candidate any) ReportDurableSaver {
	if saver, ok := candidate.(ReportDurableSaver); ok {
		return saver
	}
	return nil
}

func NewTransactionalReportDurableSaver(
	runner apptransaction.Runner,
	writer ReportDurableWriter,
	stager ReportEventStager,
) ReportDurableSaver {
	return transactionalReportDurableSaver{
		runner: runner,
		writer: writer,
		stager: stager,
	}
}

func (s transactionalReportDurableSaver) SaveReportDurably(ctx context.Context, rpt *domainReport.InterpretReport, testeeID testee.ID, events []event.DomainEvent) error {
	if rpt == nil {
		return nil
	}
	if s.runner == nil || s.writer == nil || s.stager == nil {
		return fmt.Errorf("report transactional durable saver requires transaction runner, writer and event stager")
	}

	return s.runner.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := s.writer.SaveReportRecord(txCtx, rpt, testeeID); err != nil {
			return err
		}
		if len(events) == 0 {
			return nil
		}
		return s.stager.Stage(txCtx, events...)
	})
}
