package evaluationinput

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

type InputProviderDeps struct {
	ScaleCatalog            port.ScaleModelCatalog
	TypologyCatalog         port.TypologyModelCatalog
	BehavioralRatingCatalog port.BehavioralRatingModelCatalog
	CognitiveCatalog        port.CognitiveModelCatalog
	AnswerSheets            port.AnswerSheetReader
	Questionnaires          port.QuestionnaireReader
	NormSubjectReader       port.NormSubjectReader
}

func MaterializeInputProviders(paths []modelcatalog.ExecutionPath, deps InputProviderDeps) ([]ModelInputProvider, error) {
	if deps.ScaleCatalog == nil || deps.TypologyCatalog == nil || deps.AnswerSheets == nil || deps.Questionnaires == nil {
		return nil, fmt.Errorf("evaluation input provider dependencies are incomplete")
	}
	providers := make([]ModelInputProvider, 0, len(paths))
	for _, path := range paths {
		provider, err := materializeInputProvider(path, deps)
		if err != nil {
			return nil, err
		}
		providers = append(providers, provider)
	}
	return providers, nil
}

func materializeInputProvider(path modelcatalog.ExecutionPath, deps InputProviderDeps) (ModelInputProvider, error) {
	switch path {
	case modelcatalog.ExecutionPathScaleDescriptor:
		return NewScaleModelInputProvider(
			deps.ScaleCatalog,
			deps.AnswerSheets,
			deps.Questionnaires,
		), nil
	case modelcatalog.ExecutionPathTypologyDescriptor:
		return NewConfiguredTypologyModelInputProvider(
			deps.TypologyCatalog,
			deps.AnswerSheets,
			deps.Questionnaires,
		), nil
	case modelcatalog.ExecutionPathBehavioralRatingDescriptor:
		if deps.BehavioralRatingCatalog == nil {
			return nil, fmt.Errorf("behavioral_rating catalog is required")
		}
		return NewBehavioralRatingModelInputProvider(
			deps.BehavioralRatingCatalog,
			deps.AnswerSheets,
			deps.Questionnaires,
			deps.NormSubjectReader,
		), nil
	case modelcatalog.ExecutionPathCognitiveDescriptor:
		if deps.CognitiveCatalog == nil {
			return nil, fmt.Errorf("cognitive catalog is required")
		}
		return NewCognitiveModelInputProvider(
			deps.CognitiveCatalog,
			deps.AnswerSheets,
			deps.Questionnaires,
			deps.NormSubjectReader,
		), nil
	default:
		return nil, fmt.Errorf("unsupported evaluation execution path: %s", path)
	}
}
