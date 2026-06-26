package evaluationinput

import (
	"fmt"

	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

type InputProviderDeps struct {
	ScaleCatalog    port.ScaleModelCatalog
	TypologyCatalog port.TypologyModelCatalog
	AnswerSheets    port.AnswerSheetReader
	Questionnaires  port.QuestionnaireReader
}

func MaterializeInputProviders(descs []evaldomain.ModelDescriptor, deps InputProviderDeps) ([]ModelInputProvider, error) {
	if deps.ScaleCatalog == nil || deps.TypologyCatalog == nil || deps.AnswerSheets == nil || deps.Questionnaires == nil {
		return nil, fmt.Errorf("evaluation input provider dependencies are incomplete")
	}
	providers := make([]ModelInputProvider, 0, len(descs))
	for _, desc := range descs {
		switch desc.Kind {
		case evaldomain.ModelKindScale:
			providers = append(providers, NewScaleModelInputProvider(
				deps.ScaleCatalog,
				deps.AnswerSheets,
				deps.Questionnaires,
			))
		case evaldomain.ModelKindTypology:
			providers = append(providers, NewConfiguredTypologyModelInputProvider(
				deps.TypologyCatalog,
				deps.AnswerSheets,
				deps.Questionnaires,
			))
		default:
			return nil, fmt.Errorf("unsupported evaluation model kind: %s", desc.Kind)
		}
	}
	return providers, nil
}
