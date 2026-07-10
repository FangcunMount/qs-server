package modelcatalog

import (
	"context"
	"encoding/json"

	codesApp "github.com/FangcunMount/qs-server/internal/apiserver/application/codes"
	assessmentModelApp "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
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
		m.Authoring.Preview = m.previewAdapter()
	}
	deps.AssessmentModel.Definition = m.Authoring
	deps.AssessmentModel.Publication = m.Publication
	deps.AssessmentModel.Query = assessmentModelApp.NewCatalogQueryService(assessmentModelApp.CatalogQueryDependencies{
		Models: m.ModelRepo, Published: m.PublishedLister, Authorizer: assessmentModelApp.SnapshotAuthorizer{}, QRCode: qrCodeService, HotRank: m.HotRank.ReadModel,
	})
	return deps
}

func (m *Module) previewAdapter() func(context.Context, string, json.RawMessage) (*assessmentModelApp.PreviewReportResult, error) {
	if m == nil || m.Typology == nil || m.Typology.CommandService == nil {
		return nil
	}
	command := m.Typology.CommandService
	return func(ctx context.Context, code string, input json.RawMessage) (*assessmentModelApp.PreviewReportResult, error) {
		result, err := command.PreviewReport(ctx, code, input)
		if err != nil || result == nil {
			return nil, err
		}
		out := &assessmentModelApp.PreviewReportResult{Outcome: assessmentModelApp.PreviewOutcome{Code: result.Outcome.Code, Title: result.Outcome.Title}, ScoreDetail: result.ScoreDetail, RawReport: result.RawReport}
		out.ReportSections = make([]assessmentModelApp.PreviewReportSection, 0, len(result.ReportSections))
		for _, section := range result.ReportSections {
			out.ReportSections = append(out.ReportSections, assessmentModelApp.PreviewReportSection{Title: section.Title, Content: section.Content, Kind: section.Kind})
		}
		return out, nil
	}
}
