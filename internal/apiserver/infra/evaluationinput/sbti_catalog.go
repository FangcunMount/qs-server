package evaluationinput

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	rulesetsbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/sbti"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

//go:embed seed/sbti_fun.json
var defaultSBTIModelJSON []byte

type StaticSBTIModelCatalog struct {
	models []rulesetsbti.ModelSnapshot
}

func NewDefaultSBTIModelCatalog() (*StaticSBTIModelCatalog, error) {
	var model rulesetsbti.ModelSnapshot
	if err := json.Unmarshal(defaultSBTIModelJSON, &model); err != nil {
		return nil, fmt.Errorf("load default sbti model: %w", err)
	}
	if err := validateSBTIModelSnapshot(model); err != nil {
		return nil, err
	}
	return NewStaticSBTIModelCatalog(model), nil
}

func NewStaticSBTIModelCatalog(models ...rulesetsbti.ModelSnapshot) *StaticSBTIModelCatalog {
	copied := make([]rulesetsbti.ModelSnapshot, 0, len(models))
	for _, model := range models {
		copied = append(copied, cloneSBTIModelSnapshot(model))
	}
	return &StaticSBTIModelCatalog{models: copied}
}

func (c *StaticSBTIModelCatalog) GetSBTIModelByRef(_ context.Context, ref port.ModelRef) (*rulesetsbti.ModelSnapshot, error) {
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

func (c *StaticSBTIModelCatalog) FindSBTIModelByQuestionnaire(_ context.Context, code, version string) (*rulesetsbti.ModelSnapshot, error) {
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

func validateSBTIModelSnapshot(model rulesetsbti.ModelSnapshot) error {
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

func cloneSBTIModelSnapshot(model rulesetsbti.ModelSnapshot) rulesetsbti.ModelSnapshot {
	cloned := model
	cloned.DimensionOrder = append([]string(nil), model.DimensionOrder...)
	cloned.Dimensions = cloneSBTIDimensions(model.Dimensions)
	cloned.QuestionMappings = append([]rulesetsbti.QuestionMappingSnapshot(nil), model.QuestionMappings...)
	for i := range cloned.QuestionMappings {
		cloned.QuestionMappings[i].OptionScores = cloneFloatMap(model.QuestionMappings[i].OptionScores)
	}
	cloned.NormalOutcomes = cloneSBTIOutcomes(model.NormalOutcomes)
	cloned.SpecialOutcomes = cloneSBTIOutcomes(model.SpecialOutcomes)
	cloned.DrinkTrigger.QuestionCodes = append([]string(nil), model.DrinkTrigger.QuestionCodes...)
	cloned.DrinkTrigger.OptionValues = append([]string(nil), model.DrinkTrigger.OptionValues...)
	return cloned
}

func cloneSBTIDimensions(source map[string]rulesetsbti.DimensionSnapshot) map[string]rulesetsbti.DimensionSnapshot {
	if source == nil {
		return nil
	}
	cloned := make(map[string]rulesetsbti.DimensionSnapshot, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func cloneSBTIOutcomes(source []rulesetsbti.OutcomeSnapshot) []rulesetsbti.OutcomeSnapshot {
	if source == nil {
		return nil
	}
	return append([]rulesetsbti.OutcomeSnapshot(nil), source...)
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
	catalog             port.SBTIModelCatalog
	answerSheetReader   port.AnswerSheetReader
	questionnaireReader port.QuestionnaireReader
}

func NewSBTIModelInputProvider(
	catalog port.SBTIModelCatalog,
	answerSheetReader port.AnswerSheetReader,
	questionnaireReader port.QuestionnaireReader,
) SBTIModelInputProvider {
	return SBTIModelInputProvider{
		catalog:             catalog,
		answerSheetReader:   answerSheetReader,
		questionnaireReader: questionnaireReader,
	}
}

func (SBTIModelInputProvider) Kind() port.EvaluationModelKind {
	return port.EvaluationModelKindSBTI
}

func (p SBTIModelInputProvider) ResolveInput(ctx context.Context, ref port.InputRef) (*port.InputSnapshot, error) {
	if p.catalog == nil {
		return nil, port.NewResolveError(port.FailureKindModelNotFound, fmt.Errorf("sbti model catalog is not configured"), "SBTI 模型不存在", "加载解释模型失败")
	}
	model, err := p.catalog.GetSBTIModelByRef(ctx, ref.ModelRef)
	if err != nil {
		return nil, port.NewResolveError(port.FailureKindModelNotFound, err, "SBTI 模型不存在", "加载解释模型失败")
	}
	if !model.IsPublished() {
		err := fmt.Errorf("sbti model is not published: %s", model.Code)
		return nil, port.NewResolveError(port.FailureKindModelNotFound, err, "SBTI 模型不可用", "加载解释模型失败")
	}

	answerSheet, err := p.answerSheetReader.GetAnswerSheet(ctx, ref.AnswerSheetID)
	if err != nil {
		return nil, err
	}
	if !model.MatchesQuestionnaire(answerSheet.QuestionnaireCode, answerSheet.QuestionnaireVersion) {
		err := fmt.Errorf("answersheet questionnaire %s@%s does not match sbti model questionnaire %s@%s",
			answerSheet.QuestionnaireCode,
			answerSheet.QuestionnaireVersion,
			model.QuestionnaireCode,
			model.QuestionnaireVersion,
		)
		return nil, port.NewResolveError(port.FailureKindQuestionnaireVersionMismatch, err, "问卷版本不匹配", "加载问卷失败")
	}

	qnr, err := p.questionnaireReader.GetQuestionnaire(ctx, answerSheet.QuestionnaireCode, answerSheet.QuestionnaireVersion)
	if err != nil {
		return nil, err
	}
	payload := port.SBTIModelPayload{Model: model}
	return &port.InputSnapshot{
		Model:         port.NewSBTIModelSnapshot(model),
		ModelPayload:  payload,
		AnswerSheet:   answerSheet,
		Questionnaire: qnr,
	}, nil
}
