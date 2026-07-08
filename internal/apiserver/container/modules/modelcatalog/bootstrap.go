package modelcatalog

import (
	quesApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/questionnairecatalog"
)

// BootstrapInput carries container integration inputs for assessment-model bootstrap.
type BootstrapInput struct {
	Scale    ScaleDeps
	Typology TypologyDeps
	Survey   SurveyBootstrapPorts
}

// SurveyBootstrapPorts are survey-side ports required when wiring scale lifecycle.
type SurveyBootstrapPorts struct {
	QuestionnaireCatalog   questionnairecatalog.Catalog
	QuestionnairePublisher quesApp.QuestionnaireLifecycleService
}

// ApplySurveyPorts fills optional scale deps from survey module ports.
func (p SurveyBootstrapPorts) ApplySurveyPorts(deps *ScaleDeps) {
	if deps == nil {
		return
	}
	if p.QuestionnaireCatalog != nil {
		deps.QuestionnaireCatalog = p.QuestionnaireCatalog
	}
	if p.QuestionnairePublisher != nil {
		deps.QuestionnairePublisher = p.QuestionnairePublisher
	}
}

// Bootstrap assembles scale + personality catalog capabilities.
func Bootstrap(in BootstrapInput) (*Module, error) {
	scaleDeps := in.Scale
	in.Survey.ApplySurveyPorts(&scaleDeps)
	return New(Deps{
		Scale:       scaleDeps,
		Typology: in.Typology,
		TaskPerformance: TaskPerformanceDeps{
			ModelRepo:     in.Typology.ModelRepo,
			PublishedRepo: in.Typology.PublishedRepo,
		},
		Norming: NormingDeps{
			ModelRepo:     in.Typology.ModelRepo,
			PublishedRepo: in.Typology.PublishedRepo,
		},
	})
}
