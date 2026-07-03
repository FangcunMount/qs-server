package assessmentmodel

import (
	"context"
	"encoding/json"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/assessmentmodel/personality"
)

type personalityGateway struct {
	cmd personality.Service
}

func (g personalityGateway) require() (personality.Service, error) {
	if g.cmd == nil {
		return nil, unavailable("人格模型服务未配置")
	}
	return g.cmd, nil
}

func (g personalityGateway) create(ctx context.Context, dto CreateModelDTO) (*ModelSummary, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.Create(ctx, personalityCreateInput(dto))
	if err != nil {
		return nil, err
	}
	return summaryFromPersonality(result), nil
}

func (g personalityGateway) updateBasicInfo(ctx context.Context, dto UpdateBasicInfoDTO) (*ModelSummary, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.UpdateBasicInfo(ctx, personalityUpdateBasicInfoInput(dto))
	if err != nil {
		return nil, err
	}
	return summaryFromPersonality(result), nil
}

func (g personalityGateway) delete(ctx context.Context, modelCode string) error {
	cmd, err := g.require()
	if err != nil {
		return err
	}
	return cmd.Delete(ctx, modelCode)
}

func (g personalityGateway) publish(ctx context.Context, modelCode string) (*ModelSummary, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.Publish(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return summaryFromPersonality(result), nil
}

func (g personalityGateway) unpublish(ctx context.Context, modelCode string) (*ModelSummary, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.Unpublish(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return summaryFromPersonality(result), nil
}

func (g personalityGateway) archive(ctx context.Context, modelCode string) (*ModelSummary, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.Archive(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return summaryFromPersonality(result), nil
}

func (g personalityGateway) bindQuestionnaire(ctx context.Context, dto BindQuestionnaireDTO) (*QuestionnaireBindingResult, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.BindQuestionnaire(ctx, personalityBindInput(dto))
	if err != nil {
		return nil, err
	}
	return questionnaireFromPersonality(result), nil
}

func (g personalityGateway) getQuestionnaire(ctx context.Context, modelCode string) (*QuestionnaireBindingResult, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.GetQuestionnaire(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return questionnaireFromPersonality(result), nil
}

func (g personalityGateway) getDefinition(ctx context.Context, modelCode string) (*DefinitionDTO, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.GetDefinition(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return definitionFromPersonality(result), nil
}

func (g personalityGateway) updateDefinition(ctx context.Context, modelCode string, dto DefinitionDTO) (*DefinitionDTO, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.UpdateDefinition(ctx, modelCode, personalityDefinitionInput(dto))
	if err != nil {
		return nil, err
	}
	return definitionFromPersonality(result), nil
}

func (g personalityGateway) validate(ctx context.Context, modelCode string) (*ValidationResult, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.Validate(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return validationFromPersonality(result), nil
}

func (g personalityGateway) previewReport(ctx context.Context, modelCode string, payload json.RawMessage) (*PreviewReportResult, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.PreviewReport(ctx, modelCode, payload)
	if err != nil {
		if issues, ok := personality.AsValidationFailed(err); ok {
			return nil, validationFailedFromPersonalityIssues(issues)
		}
		return nil, err
	}
	return previewFromPersonality(result), nil
}
