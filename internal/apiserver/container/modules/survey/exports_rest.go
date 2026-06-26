package survey

import (
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	qrcodeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode"
	questionnaireApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	resttransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest"
)

// RESTExportOptions carries container integration inputs for REST export.
type RESTExportOptions struct {
	QRCodeService qrcodeApp.QRCodeService
}

// ExportRESTDeps exposes survey capabilities to REST transport.
func (m *Module) ExportRESTDeps(opts RESTExportOptions) resttransport.SurveyDeps {
	deps := resttransport.SurveyDeps{}
	if m == nil {
		return deps
	}
	if m.Questionnaire != nil {
		deps.QuestionnaireLifecycleService = m.Questionnaire.LifecycleService
		deps.QuestionnaireContentService = m.Questionnaire.ContentService
		deps.QuestionnaireQueryService = m.Questionnaire.QueryService
		deps.QuestionnaireQRCodeService = questionnaireApp.NewQRCodeQueryService(m.Questionnaire.QueryService, opts.QRCodeService)
	}
	if m.AnswerSheet != nil {
		deps.AnswerSheetManagementService = m.AnswerSheet.ManagementService
		deps.AnswerSheetSubmissionService = m.AnswerSheet.SubmissionService
	}
	return deps
}

// ExportRESTEventStatusOutbox exposes the answer-sheet outbox status reader for platform event status.
func (m *Module) ExportRESTEventStatusOutbox() appEventing.NamedOutboxStatusReader {
	if m == nil || m.AnswerSheet == nil {
		return appEventing.NamedOutboxStatusReader{}
	}
	return m.AnswerSheet.SubmittedEventStatusReader
}
