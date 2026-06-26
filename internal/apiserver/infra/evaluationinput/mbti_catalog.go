package evaluationinput

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

//go:embed seed/mbti_oejts.json
var defaultMBTIModelJSON []byte

type StaticMBTIModelCatalog struct {
	models []modeltypology.MBTILegacyModel
}

func NewDefaultMBTIModelCatalog() (*StaticMBTIModelCatalog, error) {
	var model modeltypology.MBTILegacyModel
	if err := json.Unmarshal(defaultMBTIModelJSON, &model); err != nil {
		return nil, fmt.Errorf("load default mbti model: %w", err)
	}
	if err := validateMBTIModelSnapshot(model); err != nil {
		return nil, err
	}
	return NewStaticMBTIModelCatalog(model), nil
}

func NewStaticMBTIModelCatalog(models ...modeltypology.MBTILegacyModel) *StaticMBTIModelCatalog {
	copied := make([]modeltypology.MBTILegacyModel, 0, len(models))
	for _, model := range models {
		copied = append(copied, cloneMBTIModelSnapshot(model))
	}
	return &StaticMBTIModelCatalog{models: copied}
}

func (c *StaticMBTIModelCatalog) GetMBTIModelByRef(_ context.Context, ref port.ModelRef) (*modeltypology.MBTILegacyModel, error) {
	if c == nil {
		return nil, fmt.Errorf("mbti model catalog is not configured")
	}
	code := strings.TrimSpace(ref.Code)
	if code == "" {
		code = port.DefaultMBTIModelCode
	}
	for _, model := range c.models {
		if model.Code != code {
			continue
		}
		if ref.Version != "" && model.Version != ref.Version {
			continue
		}
		cloned := cloneMBTIModelSnapshot(model)
		return &cloned, nil
	}
	return nil, fmt.Errorf("mbti model not found: %s@%s", code, ref.Version)
}

func (c *StaticMBTIModelCatalog) FindMBTIModelByQuestionnaire(_ context.Context, code, version string) (*modeltypology.MBTILegacyModel, error) {
	if c == nil {
		return nil, fmt.Errorf("mbti model catalog is not configured")
	}
	for _, model := range c.models {
		if model.MatchesQuestionnaire(code, version) {
			cloned := cloneMBTIModelSnapshot(model)
			return &cloned, nil
		}
	}
	return nil, fmt.Errorf("mbti model not found for questionnaire: %s@%s", code, version)
}

func validateMBTIModelSnapshot(model modeltypology.MBTILegacyModel) error {
	if model.Code == "" {
		return fmt.Errorf("mbti model code is required")
	}
	if model.Version == "" {
		return fmt.Errorf("mbti model version is required")
	}
	if model.QuestionnaireCode == "" {
		return fmt.Errorf("mbti questionnaire code is required")
	}
	if len(model.DimensionOrder) == 0 {
		return fmt.Errorf("mbti dimension order is required")
	}
	if len(model.QuestionMappings) == 0 {
		return fmt.Errorf("mbti question mappings are required")
	}
	if len(model.TypeProfiles) == 0 {
		return fmt.Errorf("mbti type profiles are required")
	}
	return nil
}

func cloneMBTIModelSnapshot(model modeltypology.MBTILegacyModel) modeltypology.MBTILegacyModel {
	cloned := model
	cloned.DimensionOrder = append([]string(nil), model.DimensionOrder...)
	cloned.Dimensions = cloneMBTIDimensions(model.Dimensions)
	cloned.QuestionMappings = append([]modeltypology.MBTILegacyQuestionMapping(nil), model.QuestionMappings...)
	cloned.TypeProfiles = cloneMBTITypeProfiles(model.TypeProfiles)
	return cloned
}

func cloneMBTIDimensions(source map[string]modeltypology.MBTILegacyDimension) map[string]modeltypology.MBTILegacyDimension {
	if source == nil {
		return nil
	}
	cloned := make(map[string]modeltypology.MBTILegacyDimension, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func cloneMBTITypeProfiles(source []modeltypology.MBTILegacyTypeProfile) []modeltypology.MBTILegacyTypeProfile {
	if source == nil {
		return nil
	}
	cloned := make([]modeltypology.MBTILegacyTypeProfile, len(source))
	for i, profile := range source {
		cloned[i] = profile
		cloned[i].Traits = append([]string(nil), profile.Traits...)
		cloned[i].Strengths = append([]string(nil), profile.Strengths...)
		cloned[i].Weaknesses = append([]string(nil), profile.Weaknesses...)
		cloned[i].Suggestions = append([]string(nil), profile.Suggestions...)
	}
	return cloned
}

type MBTIModelInputProvider struct {
	catalog             port.MBTIModelCatalog
	answerSheetReader   port.AnswerSheetReader
	questionnaireReader port.QuestionnaireReader
}

func NewMBTIModelInputProvider(
	catalog port.MBTIModelCatalog,
	answerSheetReader port.AnswerSheetReader,
	questionnaireReader port.QuestionnaireReader,
) MBTIModelInputProvider {
	return MBTIModelInputProvider{
		catalog:             catalog,
		answerSheetReader:   answerSheetReader,
		questionnaireReader: questionnaireReader,
	}
}

func (MBTIModelInputProvider) EvaluatorKey() evaldomain.EvaluatorKey {
	return evaldomain.EvaluatorKeyMBTI
}

func (p MBTIModelInputProvider) ResolveInput(ctx context.Context, ref port.InputRef) (*port.InputSnapshot, error) {
	if p.catalog == nil {
		return nil, port.NewResolveError(port.FailureKindModelNotFound, fmt.Errorf("mbti model catalog is not configured"), "MBTI 模型不存在", "加载解释模型失败")
	}
	model, err := p.catalog.GetMBTIModelByRef(ctx, ref.ModelRef)
	if err != nil {
		return nil, port.NewResolveError(port.FailureKindModelNotFound, err, "MBTI 模型不存在", "加载解释模型失败")
	}
	if !model.IsPublished() {
		err := fmt.Errorf("mbti model is not published: %s", model.Code)
		return nil, port.NewResolveError(port.FailureKindModelNotFound, err, "MBTI 模型不可用", "加载解释模型失败")
	}

	answerSheet, err := p.answerSheetReader.GetAnswerSheet(ctx, ref.AnswerSheetID)
	if err != nil {
		return nil, err
	}
	if !model.MatchesQuestionnaire(answerSheet.QuestionnaireCode, answerSheet.QuestionnaireVersion) {
		err := fmt.Errorf("answersheet questionnaire %s@%s does not match mbti model questionnaire %s@%s",
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
	payload := port.MBTIModelPayload{Model: model}
	return &port.InputSnapshot{
		Model:         port.NewMBTIModelSnapshot(model),
		ModelPayload:  payload,
		AnswerSheet:   answerSheet,
		Questionnaire: qnr,
	}, nil
}
