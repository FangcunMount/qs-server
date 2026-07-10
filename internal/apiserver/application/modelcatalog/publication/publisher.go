package publication

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// Publisher coordinates snapshot materialization and persistence.
type Publisher struct {
	Registry  definition.Registry
	ModelRepo port.ModelRepository
	Repo      port.PublishedModelRepository
	Now       func() time.Time
}

type PublishOptions struct {
	ReplaceKind    domain.Kind
	AfterPublished func(ctx context.Context, code, action string)
}

func (p Publisher) BuildSnapshot(ctx context.Context, model *domain.AssessmentModel) (*port.AssessmentSnapshot, error) {
	if model == nil {
		return nil, fmt.Errorf("assessment model is nil")
	}
	handler, err := p.Registry.MustResolve(identityFromModel(model))
	if err != nil {
		return nil, err
	}
	result, err := handler.BuildSnapshotPayload(ctx, model)
	if err != nil {
		return nil, err
	}
	return snapshotFromModel(model, result), nil
}

func (p Publisher) Save(ctx context.Context, snapshot *port.AssessmentSnapshot) error {
	if p.Repo == nil {
		return fmt.Errorf("published model repository is nil")
	}
	return p.Repo.Save(ctx, snapshot)
}

func (p Publisher) Publish(ctx context.Context, model *domain.AssessmentModel, options PublishOptions) (*port.AssessmentSnapshot, error) {
	if model == nil {
		return nil, fmt.Errorf("assessment model is nil")
	}
	if p.ModelRepo == nil {
		return nil, fmt.Errorf("model repository is nil")
	}
	if p.Repo == nil {
		return nil, fmt.Errorf("published model repository is nil")
	}
	handler, err := p.Registry.MustResolve(identityFromModel(model))
	if err != nil {
		return nil, err
	}
	if issues := handler.ValidateForPublish(ctx, model); len(issues) > 0 {
		return nil, definition.NewValidationError(issues)
	}
	now := p.now()
	if model.IsPublished() {
		// Allow callers to pre-mark the draft model after legacy-compatible validation.
	} else if err := model.MarkPublished(now); err != nil {
		return nil, err
	}
	snapshot, err := p.BuildSnapshot(ctx, model)
	if err != nil {
		return nil, err
	}
	replaceKind := options.ReplaceKind
	if replaceKind == "" {
		replaceKind = snapshot.Kind
	}
	if err := p.Repo.DeletePublished(ctx, replaceKind, model.Code); err != nil {
		return nil, err
	}
	if err := p.Repo.Save(ctx, snapshot); err != nil {
		return nil, err
	}
	if err := p.ModelRepo.Update(ctx, model); err != nil {
		_ = p.Repo.DeletePublished(ctx, replaceKind, model.Code)
		return nil, err
	}
	if options.AfterPublished != nil {
		options.AfterPublished(ctx, model.Code, "publish")
	}
	return snapshot, nil
}

func (p Publisher) now() time.Time {
	if p.Now != nil {
		return p.Now().UTC()
	}
	return time.Now().UTC()
}

func identityFromModel(model *domain.AssessmentModel) domain.Identity {
	if model == nil {
		return domain.Identity{}
	}
	return domain.Identity{Kind: model.Kind, SubKind: model.SubKind, Algorithm: model.Algorithm}
}

func snapshotFromModel(model *domain.AssessmentModel, result definition.SnapshotBuildResult) *port.AssessmentSnapshot {
	version := modelVersionString(model)
	if result.Version != "" {
		version = result.Version
	}
	snapshot := &port.AssessmentSnapshot{
		SchemaVersion:        domain.SchemaVersionV2,
		PayloadFormat:        result.PayloadFormat,
		ProductChannel:       domain.ResolveProductChannel(model.Kind, model.ProductChannel),
		Kind:                 result.Kind,
		SubKind:              result.SubKind,
		Algorithm:            result.Algorithm,
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
		Payload:              result.Payload,
		DefinitionV2:         model.DefinitionV2,
	}
	return snapshot
}

func modelVersionString(model *domain.AssessmentModel) string {
	return "v" + strconv.FormatInt(model.Revision(), 10)
}
