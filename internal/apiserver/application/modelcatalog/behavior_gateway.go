package modelcatalog

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/behavior"
)

type behaviorGateway struct {
	cmd behavior.Command
}

func (g behaviorGateway) require() (behavior.Command, error) {
	if g.cmd == nil {
		return nil, unavailable("行为能力模型服务未配置")
	}
	return g.cmd, nil
}

func (g behaviorGateway) delete(ctx context.Context, modelCode string) error {
	cmd, err := g.require()
	if err != nil {
		return err
	}
	return cmd.Delete(ctx, modelCode)
}

func (g behaviorGateway) publish(ctx context.Context, modelCode string) (*ModelSummary, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.Publish(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return summaryFromBehavior(result), nil
}

func (g behaviorGateway) unpublish(ctx context.Context, modelCode string) (*ModelSummary, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.Unpublish(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return summaryFromBehavior(result), nil
}

func (g behaviorGateway) archive(ctx context.Context, modelCode string) (*ModelSummary, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.Archive(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return summaryFromBehavior(result), nil
}

func (g behaviorGateway) bindQuestionnaire(ctx context.Context, dto BindQuestionnaireDTO) (*behavior.Binding, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	return cmd.BindQuestionnaire(ctx, behavior.BindQuestionnaireInput{
		Code:                 dto.Code,
		QuestionnaireCode:    dto.QuestionnaireCode,
		QuestionnaireVersion: dto.QuestionnaireVersion,
	})
}

func (g behaviorGateway) getQRCode(ctx context.Context, modelCode string) (string, error) {
	cmd, err := g.require()
	if err != nil {
		return "", unavailable("模型二维码服务未配置")
	}
	return cmd.GetQRCode(ctx, modelCode)
}
