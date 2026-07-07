package reporting

import (
	"context"
	"fmt"
	"time"

	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/pkg/outboxpolicy"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// ReportDurableSaver 持久化reports 和 stages interpretation events atomically。
type ReportDurableSaver interface {
	SaveReportDurably(ctx context.Context, rpt *domainReport.InterpretReport, testeeID testee.ID, events []event.DomainEvent) error
}

// ReportDurableWriter 持久化report records inside transaction。
type ReportDurableWriter interface {
	SaveReportRecord(ctx context.Context, rpt *domainReport.InterpretReport, testeeID testee.ID) error
}

// ReportEventStager 暂存领域事件 为 outbox inside transaction。
type ReportEventStager interface {
	Stage(ctx context.Context, events ...event.DomainEvent) error
}

type transactionalReportDurableSaver struct {
	runner       apptransaction.Runner
	writer       ReportDurableWriter
	stager       ReportEventStager
	readyIndexer *appEventing.PostCommitReadyIndexer
}

// NewTransactionalReportDurableSaver 创建saver that writes report + outbox in 一个transaction。
func NewTransactionalReportDurableSaver(
	runner apptransaction.Runner,
	writer ReportDurableWriter,
	stager ReportEventStager,
	readyIndexer *appEventing.PostCommitReadyIndexer,
) ReportDurableSaver {
	return transactionalReportDurableSaver{
		runner:       runner,
		writer:       writer,
		stager:       stager,
		readyIndexer: readyIndexer,
	}
}

func (s transactionalReportDurableSaver) SaveReportDurably(ctx context.Context, rpt *domainReport.InterpretReport, testeeID testee.ID, events []event.DomainEvent) error {
	if rpt == nil {
		return nil
	}
	if s.runner == nil || s.writer == nil || s.stager == nil {
		return fmt.Errorf("report transactional durable saver requires transaction runner, writer and event stager")
	}

	var stagedEvents []event.DomainEvent
	err := s.runner.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := s.writer.SaveReportRecord(txCtx, rpt, testeeID); err != nil {
			return err
		}
		if len(events) == 0 {
			return nil
		}
		events = outboxpolicy.Filter(events)
		if len(events) == 0 {
			return nil
		}
		stagedEvents = events
		return s.stager.Stage(txCtx, events...)
	})
	if err != nil {
		return err
	}
	if s.readyIndexer != nil && len(stagedEvents) > 0 {
		s.readyIndexer.EnqueueAfterCommit(ctx, stagedEvents, time.Now())
	}
	return nil
}
