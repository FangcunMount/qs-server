package modelcatalog

import (
	codesApp "github.com/FangcunMount/qs-server/internal/apiserver/application/codes"
	assessmentModelApp "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	scaleApp "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/behavior/scale"
	appNorming "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/norming"
	appTaskPerformance "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/taskperformance"
	assessmentModelAppTypology "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/typology"
	typologyModelApp "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/typology/consumer"
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
	var typologyQuery = m.typologyQuery()
	var typologyCommand = m.typologyCommand()
	deps.AssessmentModel.Service = assessmentModelApp.NewService(assessmentModelApp.Dependencies{
		TypologyCommand:        typologyCommand,
		NormingCommand:         m.normingCommand(),
		TaskPerformanceCommand: m.taskPerformanceCommand(),
		TypologyQuery:          typologyQuery,
		QuestionnaireQuery:     questionnaireQuery,
		Codes:                  codesService,
		RawQRCodeGenerator:     qrCodeService,
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

func (m *Module) typologyQuery() typologyModelApp.PersonalityModelQueryService {
	if m == nil || m.Typology == nil {
		return nil
	}
	return m.Typology.QueryService
}

func (m *Module) taskPerformanceCommand() appTaskPerformance.Service {
	if m == nil || m.TaskPerformance == nil {
		return nil
	}
	return m.TaskPerformance.CommandService
}

func (m *Module) normingCommand() appNorming.Service {
	if m == nil || m.Norming == nil {
		return nil
	}
	return m.Norming.CommandService
}

func (m *Module) typologyCommand() assessmentModelAppTypology.Service {
	if m == nil || m.Typology == nil {
		return nil
	}
	return m.Typology.CommandService
}
