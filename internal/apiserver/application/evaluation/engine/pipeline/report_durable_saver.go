package pipeline

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type ReportDurableSaver interface {
	SaveReportDurably(ctx context.Context, rpt *domainReport.InterpretReport, testeeID testee.ID, events []event.DomainEvent) error
}

type eventfulReportRepositoryAdapter struct {
	repo domainReport.ReportRepository
}

func NewReportDurableSaver(repo domainReport.ReportRepository) ReportDurableSaver {
	if repo == nil {
		return nil
	}
	if saver, ok := repo.(ReportDurableSaver); ok {
		return saver
	}
	return eventfulReportRepositoryAdapter{repo: repo}
}

func (a eventfulReportRepositoryAdapter) SaveReportDurably(ctx context.Context, rpt *domainReport.InterpretReport, testeeID testee.ID, events []event.DomainEvent) error {
	return a.repo.SaveWithTesteeAndEvents(ctx, rpt, testeeID, events)
}
