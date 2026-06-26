package evaluationinput

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

//go:embed seed/sbti_fun.json
var defaultSBTIModelJSON []byte

type StaticSBTIModelCatalog struct {
	models []modeltypology.SBTILegacyModel
}

func NewDefaultSBTIModelCatalog() (*StaticSBTIModelCatalog, error) {
	var model modeltypology.SBTILegacyModel
	if err := json.Unmarshal(defaultSBTIModelJSON, &model); err != nil {
		return nil, fmt.Errorf("load default sbti model: %w", err)
	}
	if err := validateSBTIModelSnapshot(model); err != nil {
		return nil, err
	}
	return NewStaticSBTIModelCatalog(model), nil
}

func NewStaticSBTIModelCatalog(models ...modeltypology.SBTILegacyModel) *StaticSBTIModelCatalog {
	copied := make([]modeltypology.SBTILegacyModel, 0, len(models))
	for _, model := range models {
		copied = append(copied, cloneSBTIModelSnapshot(model))
	}
	return &StaticSBTIModelCatalog{models: copied}
}

func (c *StaticSBTIModelCatalog) GetSBTIModelByRef(_ context.Context, ref port.ModelRef) (*modeltypology.SBTILegacyModel, error) {
	if c == nil {
		return nil, fmt.Errorf("sbti model catalog is not configured")
	}
	code := strings.TrimSpace(ref.Code)
	if code == "" {
		code = port.DefaultSBTIModelCode
	}
	for _, model := range c.models {
		if model.Code != code {
			continue
		}
		if ref.Version != "" && model.Version != ref.Version {
			continue
		}
		cloned := cloneSBTIModelSnapshot(model)
		return &cloned, nil
	}
	return nil, fmt.Errorf("sbti model not found: %s@%s", code, ref.Version)
}

func (c *StaticSBTIModelCatalog) FindSBTIModelByQuestionnaire(_ context.Context, code, version string) (*modeltypology.SBTILegacyModel, error) {
	if c == nil {
		return nil, fmt.Errorf("sbti model catalog is not configured")
	}
	for _, model := range c.models {
		if model.MatchesQuestionnaire(code, version) {
			cloned := cloneSBTIModelSnapshot(model)
			return &cloned, nil
		}
	}
	return nil, fmt.Errorf("sbti model not found for questionnaire: %s@%s", code, version)
}

func validateSBTIModelSnapshot(model modeltypology.SBTILegacyModel) error {
	if model.Code == "" {
		return fmt.Errorf("sbti model code is required")
	}
	if model.Version == "" {
		return fmt.Errorf("sbti model version is required")
	}
	if model.QuestionnaireCode == "" {
		return fmt.Errorf("sbti questionnaire code is required")
	}
	if len(model.DimensionOrder) == 0 {
		return fmt.Errorf("sbti dimension order is required")
	}
	if len(model.QuestionMappings) == 0 {
		return fmt.Errorf("sbti question mappings are required")
	}
	if len(model.NormalOutcomes) == 0 {
		return fmt.Errorf("sbti normal outcomes are required")
	}
	return nil
}

func cloneSBTIModelSnapshot(model modeltypology.SBTILegacyModel) modeltypology.SBTILegacyModel {
	cloned := model
	cloned.DimensionOrder = append([]string(nil), model.DimensionOrder...)
	cloned.Dimensions = cloneSBTIDimensions(model.Dimensions)
	cloned.QuestionMappings = append([]modeltypology.SBTILegacyQuestionMapping(nil), model.QuestionMappings...)
	for i := range cloned.QuestionMappings {
		cloned.QuestionMappings[i].OptionScores = cloneFloatMap(model.QuestionMappings[i].OptionScores)
	}
	cloned.NormalOutcomes = cloneSBTIOutcomes(model.NormalOutcomes)
	cloned.SpecialOutcomes = cloneSBTIOutcomes(model.SpecialOutcomes)
	cloned.DrinkTrigger.QuestionCodes = append([]string(nil), model.DrinkTrigger.QuestionCodes...)
	cloned.DrinkTrigger.OptionValues = append([]string(nil), model.DrinkTrigger.OptionValues...)
	return cloned
}

func cloneSBTIDimensions(source map[string]modeltypology.SBTILegacyDimension) map[string]modeltypology.SBTILegacyDimension {
	if source == nil {
		return nil
	}
	cloned := make(map[string]modeltypology.SBTILegacyDimension, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func cloneSBTIOutcomes(source []modeltypology.SBTILegacyOutcome) []modeltypology.SBTILegacyOutcome {
	if source == nil {
		return nil
	}
	return append([]modeltypology.SBTILegacyOutcome(nil), source...)
}

func cloneFloatMap(source map[string]float64) map[string]float64 {
	if source == nil {
		return nil
	}
	cloned := make(map[string]float64, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

type SBTIModelInputProvider struct {
	TypologyModelInputProvider
}

func NewSBTIModelInputProvider(
	catalog port.SBTIModelCatalog,
	answerSheetReader port.AnswerSheetReader,
	questionnaireReader port.QuestionnaireReader,
) SBTIModelInputProvider {
	return SBTIModelInputProvider{
		TypologyModelInputProvider: NewTypologyModelInputProvider(
			assessmentmodel.AlgorithmSBTI,
			NewSBTITypologyCatalog(catalog),
			answerSheetReader,
			questionnaireReader,
		),
	}
}

func (SBTIModelInputProvider) EvaluatorKey() evaldomain.EvaluatorKey {
	return evaldomain.EvaluatorKeySBTI
}
