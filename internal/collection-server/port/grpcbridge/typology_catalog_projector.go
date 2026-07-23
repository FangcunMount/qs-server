package grpcbridge

import (
	"context"
	"encoding/json"
	"fmt"

	modeldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
	modelcatalog "github.com/FangcunMount/qs-server/internal/collection-server/application/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/typologymodel"
)

// TypologyCatalogProjector is the collection ACL from generic catalog records
// to the retained typology REST facade. It never calls a typology-specific RPC.
type TypologyCatalogProjector struct {
	inner *modelcatalog.QueryService
}

func NewTypologyCatalogProjector(inner *modelcatalog.QueryService) *TypologyCatalogProjector {
	return &TypologyCatalogProjector{inner: inner}
}

func (p *TypologyCatalogProjector) GetTypologyModel(ctx context.Context, code string) (*typologymodel.TypologyModelResponse, error) {
	if p == nil || p.inner == nil {
		return nil, nil
	}
	return typologyResponseFromCatalog(p.inner.Get(ctx, code))
}

func (p *TypologyCatalogProjector) ListTypologyModels(ctx context.Context, page, pageSize int32) (*typologymodel.ListTypologyModelsResponse, error) {
	if p == nil || p.inner == nil {
		return nil, nil
	}
	result, err := p.inner.List(ctx, &modelcatalog.ListRequest{Kind: string(modeldomain.KindTypology), Page: page, PageSize: pageSize})
	if err != nil || result == nil {
		return nil, err
	}
	out := &typologymodel.ListTypologyModelsResponse{Models: make([]typologymodel.TypologyModelSummaryResponse, 0, len(result.Models)), Total: result.Total, Page: result.Page, PageSize: result.PageSize}
	for index := range result.Models {
		detail, err := typologyResponseFromCatalog(&result.Models[index], nil)
		if err != nil {
			return nil, err
		}
		out.Models = append(out.Models, typologySummaryResponse(detail))
	}
	if out.PageSize > 0 {
		out.TotalPages = int32((out.Total + int64(out.PageSize) - 1) / int64(out.PageSize))
	}
	return out, nil
}

func (p *TypologyCatalogProjector) GetTypologyModelCategories(ctx context.Context) (*typologymodel.TypologyModelCategoriesResponse, error) {
	if p == nil || p.inner == nil {
		return nil, nil
	}
	options, err := p.inner.Options(ctx, string(modeldomain.KindTypology))
	if err != nil || options == nil {
		return nil, err
	}
	result := &typologymodel.TypologyModelCategoriesResponse{Categories: make([]typologymodel.TypologyCategoryResponse, 0, len(options.Algorithms))}
	for _, item := range options.Algorithms {
		result.Categories = append(result.Categories, typologymodel.TypologyCategoryResponse{Value: item.Value, Label: item.Label})
	}
	return result, nil
}

func typologyResponseFromCatalog(model *modelcatalog.ModelResponse, incoming error) (*typologymodel.TypologyModelResponse, error) {
	if incoming != nil || model == nil {
		return nil, incoming
	}
	if !isTypologyCatalogModel(model) {
		return nil, fmt.Errorf("catalog model kind %q is not typology", model.Kind)
	}
	var definition modeldomain.Definition
	if err := json.Unmarshal(model.Definition, &definition); err != nil {
		return nil, fmt.Errorf("decode typology definition_v2: %w", err)
	}
	runtime, err := modeltypology.RuntimeSpecFromDefinition(&definition)
	if err != nil {
		return nil, fmt.Errorf("project typology runtime: %w", err)
	}
	payload, err := modeltypology.PayloadFromDefinition(modeltypology.DefinitionEnvelope{Code: model.Code, Version: model.Version, Title: model.Title, QuestionnaireCode: model.QuestionnaireCode, QuestionnaireVersion: model.QuestionnaireVersion, Status: model.Status, Algorithm: modeldomain.Algorithm(model.Algorithm)}, &definition)
	if err != nil {
		return nil, fmt.Errorf("project typology presentation: %w", err)
	}
	response := &typologymodel.TypologyModelResponse{
		Code: model.Code, Version: model.Version, Title: model.Title, Algorithm: model.Algorithm, Description: model.Description,
		QuestionnaireCode: model.QuestionnaireCode, QuestionnaireVersion: model.QuestionnaireVersion, Status: model.Status,
		Kind: string(modeldomain.KindTypology), SubKind: string(modeldomain.CanonicalSubKindFor(modeldomain.KindTypology)), ProductChannel: string(modeldomain.DefaultProductChannelFor(modeldomain.KindTypology)),
		DecisionKind: model.DecisionKind,
	}
	family, ok := modeldomain.AlgorithmFamilyFromDecisionKind(modeldomain.DecisionKind(response.DecisionKind))
	if response.DecisionKind == "" || !ok {
		return nil, fmt.Errorf("catalog typology runtime identity is incomplete")
	}
	response.AlgorithmFamily = string(family)
	response.QuestionCount = int32(countRuntimeQuestions(runtime))
	response.DimensionOrder = append([]string(nil), runtime.FactorGraph.DecisionFactorOrder()...)
	for _, code := range response.DimensionOrder {
		if dimension, ok := runtime.FactorGraph.Dimensions[code]; ok {
			response.Dimensions = append(response.Dimensions, typologymodel.TypologyDimensionResponse{Code: dimension.Code, Name: dimension.Name, LeftPole: dimension.LeftPole, RightPole: dimension.RightPole})
		}
	}
	for _, outcome := range payload.Outcomes {
		imageURL := outcome.ImageURL
		if imageURL == "" {
			imageURL = outcome.Image
		}
		response.Outcomes = append(response.Outcomes, typologymodel.TypologyOutcomeResponse{Code: outcome.Code, Name: outcome.Name, OneLiner: outcome.OneLiner, ImageURL: imageURL})
	}
	return response, nil
}

func isTypologyCatalogModel(model *modelcatalog.ModelResponse) bool {
	return model != nil && model.Kind == string(modeldomain.KindTypology) &&
		model.Algorithm == string(modeldomain.AlgorithmPersonalityTypology)
}

func typologySummaryResponse(value *typologymodel.TypologyModelResponse) typologymodel.TypologyModelSummaryResponse {
	if value == nil {
		return typologymodel.TypologyModelSummaryResponse{}
	}
	return typologymodel.TypologyModelSummaryResponse{Code: value.Code, Version: value.Version, Title: value.Title, Algorithm: value.Algorithm, Description: value.Description, QuestionnaireCode: value.QuestionnaireCode, QuestionnaireVersion: value.QuestionnaireVersion, Status: value.Status, QuestionCount: value.QuestionCount, Kind: value.Kind, SubKind: value.SubKind, ProductChannel: value.ProductChannel, AlgorithmFamily: value.AlgorithmFamily, DecisionKind: value.DecisionKind}
}

func countRuntimeQuestions(runtime *modeltypology.RuntimeSpec) int {
	seen := make(map[string]struct{})
	if runtime == nil {
		return 0
	}
	for _, factor := range runtime.FactorGraph.Factors {
		for _, contribution := range factor.Contributions {
			if contribution.QuestionCode != "" {
				seen[contribution.QuestionCode] = struct{}{}
			}
		}
	}
	return len(seen)
}

var _ typologymodel.CatalogReader = (*TypologyCatalogProjector)(nil)
