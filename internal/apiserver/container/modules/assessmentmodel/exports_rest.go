package assessmentmodel

import (
	assessmentModelApp "github.com/FangcunMount/qs-server/internal/apiserver/application/assessmentmodel"
	codesApp "github.com/FangcunMount/qs-server/internal/apiserver/application/codes"
	personalityModelApp "github.com/FangcunMount/qs-server/internal/apiserver/application/personalitymodel"
	qrcodeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode"
	scaleApp "github.com/FangcunMount/qs-server/internal/apiserver/application/scale"
	questionnaireApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	resttransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest"
)

type RESTDeps struct {
	Scale           resttransport.ScaleDeps
	AssessmentModel resttransport.AssessmentModelDeps
}

// ExportRESTDeps exposes unified assessment-model and legacy scale capabilities to REST transport.
func (m *Module) ExportRESTDeps(
	qrCodeService qrcodeApp.QRCodeService,
	codesService codesApp.CodesService,
	questionnaireQuery questionnaireApp.QuestionnaireQueryService,
) RESTDeps {
	deps := RESTDeps{}
	if m == nil || m.Scale == nil {
		return deps
	}
	deps.Scale = m.Scale.ExportRESTDeps(qrCodeService)
	var personalityQuery = m.personalityQuery()
	deps.AssessmentModel.Service = assessmentModelApp.NewService(assessmentModelApp.Dependencies{
		ScaleLifecycle:     deps.Scale.LifecycleService,
		ScaleFactor:        deps.Scale.FactorService,
		ScaleQuery:         deps.Scale.QueryService,
		ScaleCategory:      deps.Scale.CategoryService,
		ScaleQRCode:        deps.Scale.QRCodeService,
		PersonalityQuery:   personalityQuery,
		QuestionnaireQuery: questionnaireQuery,
		Codes:              codesService,
		RawQRCodeGenerator: qrCodeService,
	})
	return deps
}

// ExportRESTDeps exposes scale capabilities to REST transport.
func (s *Scale) ExportRESTDeps(qrCodeService qrcodeApp.QRCodeService) resttransport.ScaleDeps {
	deps := resttransport.ScaleDeps{}
	if s == nil {
		return deps
	}
	deps.LifecycleService = s.LifecycleService
	deps.FactorService = s.FactorService
	deps.QueryService = s.QueryService
	deps.CategoryService = s.CategoryService
	deps.QRCodeService = scaleApp.NewQRCodeQueryService(qrCodeService)
	return deps
}

func (m *Module) personalityQuery() personalityModelApp.PersonalityModelQueryService {
	if m == nil || m.Personality == nil {
		return nil
	}
	return m.Personality.QueryService
}
