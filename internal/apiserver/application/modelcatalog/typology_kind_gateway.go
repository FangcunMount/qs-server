package modelcatalog

import (
	"context"
	"encoding/json"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/personality"
)

type typologyKindGateway struct {
	cmd personality.Service
}

func (g typologyKindGateway) require() (personality.Service, error) {
	if g.cmd == nil {
		return nil, unavailable("人格模型服务未配置")
	}
	return g.cmd, nil
}

func (g typologyKindGateway) create(ctx context.Context, dto CreateModelDTO) (*ModelSummary, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.Create(ctx, typologyCreateInput(dto))
	if err != nil {
		return nil, err
	}
	return summaryFromTypology(result), nil
}

func (g typologyKindGateway) updateBasicInfo(ctx context.Context, dto UpdateBasicInfoDTO) (*ModelSummary, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.UpdateBasicInfo(ctx, typologyUpdateBasicInfoInput(dto))
	if err != nil {
		return nil, err
	}
	return summaryFromTypology(result), nil
}

func (g typologyKindGateway) delete(ctx context.Context, modelCode string) error {
	cmd, err := g.require()
	if err != nil {
		return err
	}
	return cmd.Delete(ctx, modelCode)
}

func (g typologyKindGateway) publish(ctx context.Context, modelCode string) (*ModelSummary, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.Publish(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return summaryFromTypology(result), nil
}

func (g typologyKindGateway) unpublish(ctx context.Context, modelCode string) (*ModelSummary, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.Unpublish(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return summaryFromTypology(result), nil
}

func (g typologyKindGateway) archive(ctx context.Context, modelCode string) (*ModelSummary, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.Archive(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return summaryFromTypology(result), nil
}

func (g typologyKindGateway) bindQuestionnaire(ctx context.Context, dto BindQuestionnaireDTO) (*QuestionnaireBindingResult, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.BindQuestionnaire(ctx, typologyBindInput(dto))
	if err != nil {
		return nil, err
	}
	return questionnaireFromTypology(result), nil
}

func (g typologyKindGateway) getQuestionnaire(ctx context.Context, modelCode string) (*QuestionnaireBindingResult, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.GetQuestionnaire(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return questionnaireFromTypology(result), nil
}

func (g typologyKindGateway) getDefinition(ctx context.Context, modelCode string) (*DefinitionDTO, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.GetDefinition(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return definitionFromTypology(result), nil
}

func (g typologyKindGateway) updateDefinition(ctx context.Context, modelCode string, dto DefinitionDTO) (*DefinitionDTO, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.UpdateDefinition(ctx, modelCode, typologyDefinitionInput(dto))
	if err != nil {
		return nil, err
	}
	return definitionFromTypology(result), nil
}

func (g typologyKindGateway) validate(ctx context.Context, modelCode string) (*ValidationResult, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.Validate(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return validationFromTypology(result), nil
}

func (g typologyKindGateway) previewReport(ctx context.Context, modelCode string, payload json.RawMessage) (*PreviewReportResult, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.PreviewReport(ctx, modelCode, payload)
	if err != nil {
		if issues, ok := personality.AsValidationFailed(err); ok {
			return nil, validationFailedFromTypologyIssues(issues)
		}
		return nil, err
	}
	return previewFromPersonality(result), nil
}
