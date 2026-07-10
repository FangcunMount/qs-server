package authoring

import (
	"context"
	"encoding/json"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/codes"
	modelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// Service is the DefinitionV2-first authoring use case.
type Service struct {
	ModelRepo  modelcatalogport.ModelRepository
	Authorizer modelcatalog.Authorizer
	Registry   appdefinition.Registry
	Codes      codes.CodesService
	Preview    func(context.Context, string, json.RawMessage) (*modelcatalog.PreviewReportResult, error)
	Now        func() time.Time
}

func (s Service) GetDefinition(ctx context.Context, actor modelcatalog.ActorContext, modelCode string) (*domain.Definition, error) {
	model, err := s.loadAndAuthorize(ctx, actor, modelCode)
	if err != nil {
		return nil, err
	}
	if model.DefinitionV2 == nil {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "definition_v2 is required")
	}
	return model.DefinitionV2, nil
}

func (s Service) SaveDefinition(ctx context.Context, actor modelcatalog.ActorContext, modelCode string, value *domain.Definition) (*domain.Definition, error) {
	model, err := s.loadAndAuthorize(ctx, actor, modelCode)
	if err != nil {
		return nil, err
	}
	if issues := appdefinition.ValidateDefinitionV2(value); len(issues) > 0 {
		return nil, appdefinition.NewValidationError(issues)
	}
	handler, err := s.Registry.MustResolve(domain.Identity{Kind: model.Kind, SubKind: model.SubKind, Algorithm: model.Algorithm})
	if err != nil {
		return nil, err
	}
	candidate := *model
	candidate.DefinitionV2 = value
	built, err := handler.BuildSnapshotPayload(ctx, &candidate)
	if err != nil {
		return nil, err
	}
	if len(built.Payload) == 0 {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "definition payload projection is empty")
	}
	if err := model.UpdateDefinitionWithV2(domain.DefinitionPayload{Format: built.PayloadFormat, Data: built.Payload}, value, s.now()); err != nil {
		return nil, err
	}
	if err := s.ModelRepo.Update(ctx, model); err != nil {
		return nil, err
	}
	return model.DefinitionV2, nil
}

func (s Service) ValidateDefinition(ctx context.Context, actor modelcatalog.ActorContext, modelCode string) (*modelcatalog.ValidationResult, error) {
	value, err := s.GetDefinition(ctx, actor, modelCode)
	if err != nil {
		return nil, err
	}
	issues := appdefinition.ValidateDefinitionV2(value)
	result := make([]modelcatalog.ValidationIssue, 0, len(issues))
	for _, item := range issues {
		result = append(result, modelcatalog.ValidationIssue{Field: item.Field, Code: item.Code, Message: item.Message, Level: string(item.Level)})
	}
	return modelcatalog.NewValidationResult(result), nil
}

func (s Service) ApplyCodes(ctx context.Context, actor modelcatalog.ActorContext, input modelcatalog.ApplyCodesDTO) ([]string, error) {
	if _, err := s.loadAndAuthorize(ctx, actor, input.Code); err != nil {
		return nil, err
	}
	if s.Codes == nil {
		return nil, errors.WithCode(errorCode.ErrInternalServerError, "code service is not configured")
	}
	kind, prefix := codeKindAndPrefix(input.Target)
	if kind == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "code target is invalid")
	}
	return s.Codes.Apply(ctx, kind, input.Count, prefix, map[string]interface{}{"assessment_model_code": input.Code, "target": input.Target})
}

func (s Service) PreviewReport(ctx context.Context, actor modelcatalog.ActorContext, modelCode string, input json.RawMessage) (*modelcatalog.PreviewReportResult, error) {
	if _, err := s.loadAndAuthorize(ctx, actor, modelCode); err != nil {
		return nil, err
	}
	if s.Preview == nil {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "report preview is not configured for this model")
	}
	return s.Preview(ctx, modelCode, input)
}

func (s Service) loadAndAuthorize(ctx context.Context, actor modelcatalog.ActorContext, modelCode string) (*domain.AssessmentModel, error) {
	if modelCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "model code is required")
	}
	if s.ModelRepo == nil || s.Authorizer == nil {
		return nil, errors.WithCode(errorCode.ErrInternalServerError, "definition authoring service is not configured")
	}
	model, err := s.ModelRepo.FindByCode(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	if err := s.Authorizer.Authorize(ctx, actor, modelcatalog.ActionEditDefinition, modelcatalog.Resource{Code: model.Code, Kind: model.Kind}); err != nil {
		return nil, err
	}
	return model, nil
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func codeKindAndPrefix(target string) (string, string) {
	switch target {
	case "dimension":
		return "factor", "dim"
	case "outcome":
		return "outcome", "out"
	case "rule":
		return "rule", "rule"
	default:
		return "", ""
	}
}

var _ modelcatalog.DefinitionAuthoringService = Service{}
