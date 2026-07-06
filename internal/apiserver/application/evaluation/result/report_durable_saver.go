package result

import (
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
)

func NewTransactionalReportDurableSaver(
	runner apptransaction.Runner,
	writer ReportDurableWriter,
	stager ReportEventStager,
	readyIndexer *appEventing.PostCommitReadyIndexer,
) ReportDurableSaver {
	return interpretationreporting.NewTransactionalReportDurableSaver(runner, writer, stager, readyIndexer)
}
