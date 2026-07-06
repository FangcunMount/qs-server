package modelcatalog

import (
	codesApp "github.com/FangcunMount/qs-server/internal/apiserver/application/codes"
	assessmentModelApp "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	assessmentModelBehavior "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/behavior"
	scaleApp "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/behavior/scale"
	assessmentModelAppPersonality "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/personality"
	personalityModelApp "github.com/FangcunMount/qs-server/internal/apiserver/application/personalitymodel"
	qrcodeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode"
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
	var personalityCommand = m.personalityCommand()
	deps.AssessmentModel.Service = assessmentModelApp.NewService(assessmentModelApp.Dependencies{
		BehaviorCommand: assessmentModelBehavior.NewLegacyScaleCommand(assessmentModelBehavior.LegacyScaleDeps{
			Lifecycle: deps.Scale.LifecycleService,
			Factor:    deps.Scale.FactorService,
			Query:     deps.Scale.QueryService,
			Category:  deps.Scale.CategoryService,
			QRCode:    deps.Scale.QRCodeService,
		}),
		PersonalityCommand: personalityCommand,
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

func (m *Module) personalityCommand() assessmentModelAppPersonality.Service {
	if m == nil || m.Personality == nil {
		return nil
	}
	return m.Personality.CommandService
}
