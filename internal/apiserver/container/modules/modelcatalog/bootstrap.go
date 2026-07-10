package modelcatalog

import (
	quesApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/questionnairecatalog"
)

// BootstrapInput carries container integration inputs for assessment-model bootstrap.
type BootstrapInput struct {
	Scoring  ScoringDeps
	Typology TypologyDeps
	Survey   SurveyBootstrapPorts
}

// SurveyBootstrapPorts are survey-side ports required when wiring scale lifecycle.
type SurveyBootstrapPorts struct {
	QuestionnaireCatalog   questionnairecatalog.Catalog
	QuestionnairePublisher quesApp.QuestionnaireLifecycleService
}

// ApplySurveyPorts fills optional scoring deps from survey module ports.
func (p SurveyBootstrapPorts) ApplySurveyPorts(deps *ScoringDeps) {
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

// Bootstrap assembles scoring + typology catalog capabilities.
func Bootstrap(in BootstrapInput) (*Module, error) {
	scoringDeps := in.Scoring
	in.Survey.ApplySurveyPorts(&scoringDeps)
	if scoringDeps.ModelRepo == nil {
		scoringDeps.ModelRepo = in.Typology.ModelRepo
	}
	if scoringDeps.PublishedRepo == nil {
		scoringDeps.PublishedRepo = in.Typology.PublishedRepo
	}
	if scoringDeps.PublishedReader == nil {
		scoringDeps.PublishedReader = in.Typology.PublishedReader
	}
	return New(Deps{
		Scoring:  scoringDeps,
		Typology: in.Typology,
		TaskPerformance: TaskPerformanceDeps{
			ModelRepo:     in.Typology.ModelRepo,
			PublishedRepo: in.Typology.PublishedRepo,
			NormRepo:      in.Typology.NormRepo,
		},
		Norming: NormingDeps{
			ModelRepo:     in.Typology.ModelRepo,
			PublishedRepo: in.Typology.PublishedRepo,
			NormRepo:      in.Typology.NormRepo,
		},
	})
}
