package publication

import (
	"context"
	"fmt"
	"strconv"
	"time"

	modelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// Publisher 协调快照物质化和持久化
type Publisher struct {
	Registry  definition.Registry              // 定义注册表
	ModelRepo port.ModelRepository             // 模型存储库
	Repo      port.PublishedSnapshotRepository // 已发布模型快照存储库
	Now       func() time.Time                 // 当前时间
}

// PublishOptions 发布选项
type PublishOptions struct {
	ReplaceKind    domain.Kind                                    // 替换类型
	AfterPublished func(ctx context.Context, code, action string) // 发布后回调
}

// BuildSnapshot 构建评估模型快照
func (p Publisher) BuildSnapshot(ctx context.Context, model *domain.AssessmentModel) (*port.AssessmentSnapshot, error) {
	if model == nil {
		return nil, fmt.Errorf("assessment model is nil")
	}
	// 解析模型身份
	handler, err := p.Registry.MustResolveBinding(definition.AlgorithmBindingFromModel(model))
	if err != nil {
		return nil, err
	}
	result, err := handler.MaterializeSnapshot(ctx, model)
	if err != nil {
		return nil, err
	}
	// 创建评估模型快照
	return snapshotFromModel(model, result), nil
}

// Save 保存评估模型快照
func (p Publisher) Save(ctx context.Context, snapshot *port.AssessmentSnapshot) error {
	if p.Repo == nil {
		return fmt.Errorf("已发布模型存储库为空")
	}
	return p.Repo.Save(ctx, snapshot)
}

// Publish 发布评估模型
func (p Publisher) Publish(ctx context.Context, model *domain.AssessmentModel, options PublishOptions) (*port.AssessmentSnapshot, error) {
	if model == nil {
		return nil, fmt.Errorf("评估模型为空")
	}
	if p.ModelRepo == nil {
		return nil, fmt.Errorf("模型存储库为空")
	}
	if p.Repo == nil {
		return nil, fmt.Errorf("已发布模型存储库为空")
	}
	handler, err := p.Registry.MustResolveBinding(definition.AlgorithmBindingFromModel(model))
	if err != nil {
		return nil, err
	}
	if issues := handler.ValidateForPublish(ctx, model); domain.HasValidationErrors(issues) {
		return nil, definition.NewValidationError(issues)
	}
	if model.DefinitionV2 != nil {
		modeldefinition.MaterializeLayers(model.DefinitionV2)
	}
	now := p.now()
	if model.IsPublished() {
		// Allow callers to pre-mark the draft model after canonical validation.
	} else if err := model.MarkPublished(now); err != nil {
		return nil, err
	}
	snapshot, err := p.BuildSnapshot(ctx, model)
	if err != nil {
		return nil, err
	}
	if model.DefinitionV2 != nil {
		defHash, hashErr := modeldefinition.CanonicalContentHash(model.DefinitionV2)
		if hashErr != nil {
			return nil, fmt.Errorf("compute definition content hash: %w", hashErr)
		}
		port.AttachDefinitionHash(snapshot, defHash)
	}
	if err := p.Repo.Save(ctx, snapshot); err != nil {
		return nil, err
	}
	if err := p.ModelRepo.Update(ctx, model); err != nil {
		return nil, modelcatalog.MapDraftWriteError(err)
	}
	if options.AfterPublished != nil {
		options.AfterPublished(ctx, model.Code, "publish")
	}
	return snapshot, nil
}

// now 获取当前时间
func (p Publisher) now() time.Time {
	if p.Now != nil {
		return p.Now().UTC()
	}
	return time.Now().UTC()
}

// snapshotFromModel 从模型和结果创建评估模型快照
func snapshotFromModel(model *domain.AssessmentModel, result definition.Materialization) *port.AssessmentSnapshot {
	version := modelVersionString(model)
	if result.Version != "" {
		version = result.Version
	}
	snapshot := &port.AssessmentSnapshot{
		SchemaVersion:        domain.SchemaVersionV2,
		ProductChannel:       domain.ResolveProductChannel(model.Kind, model.ProductChannel),
		Kind:                 result.Kind,
		SubKind:              result.SubKind,
		Algorithm:            result.Algorithm,
		AlgorithmFamily:      result.AlgorithmFamily,
		Code:                 model.Code,
		Version:              version,
		Title:                model.Title,
		Description:          model.Description,
		Category:             model.Category,
		Stages:               append([]string(nil), model.Stages...),
		ApplicableAges:       append([]string(nil), model.ApplicableAges...),
		Reporters:            append([]string(nil), model.Reporters...),
		Tags:                 append([]string(nil), model.Tags...),
		Status:               string(domain.ModelStatusPublished),
		DecisionKind:         result.DecisionKind,
		QuestionnaireCode:    model.Binding.QuestionnaireCode,
		QuestionnaireVersion: model.Binding.QuestionnaireVersion,
		Source:               map[string]any{},
		DefinitionV2:         model.DefinitionV2,
	}
	return snapshot
}

// modelVersionString 获取模型修订版本
func modelVersionString(model *domain.AssessmentModel) string {
	return "v" + strconv.FormatInt(model.Revision(), 10)
}
