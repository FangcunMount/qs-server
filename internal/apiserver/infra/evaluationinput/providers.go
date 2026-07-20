package evaluationinput

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

type InputProviderDeps struct {
	ScaleCatalog            port.ScaleModelCatalog
	TypologyCatalog         port.TypologyModelCatalog
	BehavioralRatingCatalog port.BehavioralRatingModelCatalog
	CognitiveCatalog        port.CognitiveModelCatalog
	PublishedModels         rulesetport.PublishedModelReader
	AnswerSheets            port.AnswerSheetReader
	Questionnaires          port.QuestionnaireReader
	NormSubjectReader       port.NormSubjectReader
}

func MaterializeInputProviders(paths []modelcatalog.ExecutionPath, deps InputProviderDeps) ([]ModelInputProvider, error) {
	if deps.ScaleCatalog == nil || deps.TypologyCatalog == nil || deps.AnswerSheets == nil || deps.Questionnaires == nil {
		return nil, fmt.Errorf("evaluation input provider dependencies are incomplete")
	}
	providers := make([]ModelInputProvider, 0, len(paths)+1)
	for _, path := range paths {
		batch, err := materializeInputProvidersForPath(path, deps)
		if err != nil {
			return nil, err
		}
		providers = append(providers, batch...)
	}
	return providers, nil
}

func materializeInputProvidersForPath(path modelcatalog.ExecutionPath, deps InputProviderDeps) ([]ModelInputProvider, error) {
	switch path {
	case modelcatalog.ExecutionPathScaleDescriptor:
		return []ModelInputProvider{NewScaleModelInputProvider(
			deps.ScaleCatalog,
			deps.PublishedModels,
			deps.AnswerSheets,
			deps.Questionnaires,
		)}, nil
	case modelcatalog.ExecutionPathTypologyDescriptor:
		return []ModelInputProvider{NewConfiguredTypologyModelInputProvider(
			deps.TypologyCatalog,
			deps.PublishedModels,
			deps.AnswerSheets,
			deps.Questionnaires,
		)}, nil
	case modelcatalog.ExecutionPathBehavioralRatingDescriptor:
		if deps.BehavioralRatingCatalog == nil {
			return nil, fmt.Errorf("behavioral_rating catalog is required")
		}
		out := make([]ModelInputProvider, 0, 2)
		for _, algorithm := range []modelcatalog.Algorithm{
			modelcatalog.AlgorithmBrief2,
			modelcatalog.AlgorithmSPMSensory,
		} {
			out = append(out, NewBehavioralRatingModelInputProvider(
				algorithm,
				deps.BehavioralRatingCatalog,
				deps.PublishedModels,
				deps.AnswerSheets,
				deps.Questionnaires,
				deps.NormSubjectReader,
			))
		}
		return out, nil
	case modelcatalog.ExecutionPathCognitiveDescriptor:
		if deps.CognitiveCatalog == nil {
			return nil, fmt.Errorf("cognitive catalog is required")
		}
		return []ModelInputProvider{NewCognitiveModelInputProvider(
			modelcatalog.AlgorithmSPM,
			deps.CognitiveCatalog,
			deps.PublishedModels,
			deps.AnswerSheets,
			deps.Questionnaires,
			deps.NormSubjectReader,
		)}, nil
	default:
		return nil, fmt.Errorf("unsupported evaluation execution path: %s", path)
	}
}
