package modelcatalog

import (
	codesApp "github.com/FangcunMount/qs-server/internal/apiserver/application/codes"
	assessmentModelApp "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	appquery "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/query"
	qrcodeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode"
	questionnaireApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	resttransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest"
)

type RESTDeps struct {
	AssessmentModel resttransport.AssessmentModelDeps
}

// ExportRESTDeps exposes unified assessment-model capabilities to REST transport.
func (m *Module) ExportRESTDeps(
	qrCodeService qrcodeApp.QRCodeService,
	codesService codesApp.CodesService,
	_ questionnaireApp.QuestionnaireQueryService,
) RESTDeps {
	deps := RESTDeps{}
	if m == nil {
		return deps
	}
	deps.AssessmentModel.Management = m.Management
	if m.Authoring != nil {
		m.Authoring.Codes = codesService
	}
	deps.AssessmentModel.Definition = m.Authoring
	deps.AssessmentModel.Publication = m.Publication
	deps.AssessmentModel.Query = appquery.NewService(appquery.Dependencies{
		Models: m.ModelRepo, Published: m.PublishedLister, Authorizer: assessmentModelApp.SnapshotAuthorizer{}, QRCode: qrCodeService, HotRank: m.HotRank.ReadModel,
	})
	return deps
}
