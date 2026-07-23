package modelcatalog

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// CatalogReader is the published-model contract consumed by collection-server
// application code. It intentionally has no scale-specific DTO methods.
type CatalogReader interface {
	GetPublishedModel(context.Context, string, string) (*CatalogModel, error)
	ListPublishedModels(context.Context, string, []string, string, string, string, string, string, int32, int32) (*CatalogList, error)
	ListHotPublishedModels(context.Context, string, int32, int32) (*HotCatalogList, error)
	GetCatalogOptions(context.Context, string) (*CatalogOptions, error)
}

type QueryService struct {
	client CatalogReader
}

func NewQueryService(client CatalogReader) *QueryService { return &QueryService{client: client} }

// CatalogModel is the application-owned published-model DTO returned by the
// collection catalogue port. It contains canonical DefinitionV2 JSON only.
type CatalogModel struct {
	Code                 string
	Kind                 string
	Algorithm            string
	DecisionKind         string
	Version              string
	Title                string
	Description          string
	Status               string
	Category             string
	Stages               []string
	ApplicableAges       []string
	Reporters            []string
	Tags                 []string
	QuestionnaireCode    string
	QuestionnaireVersion string
	Definition           json.RawMessage
}

type CatalogList struct {
	Models   []CatalogModel
	Total    int64
	Page     int32
	PageSize int32
}

type HotCatalogModel struct {
	Model           CatalogModel
	Rank            int32
	SubmissionCount int64
	HeatScore       int64
}

type HotCatalogList struct {
	Models     []HotCatalogModel
	Total      int64
	Limit      int32
	WindowDays int32
}

type CatalogOption struct {
	Label    string
	Value    string
	Disabled bool
}

type CatalogOptions struct {
	Kinds, Algorithms, Categories, Stages, ApplicableAges, Reporters []CatalogOption
}

type ModelResponse struct {
	Code                 string          `json:"code"`
	Kind                 string          `json:"kind"`
	Algorithm            string          `json:"algorithm,omitempty"`
	DecisionKind         string          `json:"decision_kind,omitempty"`
	Version              string          `json:"version,omitempty"`
	Title                string          `json:"title"`
	Description          string          `json:"description,omitempty"`
	Status               string          `json:"status"`
	Category             string          `json:"category,omitempty"`
	Stages               []string        `json:"stages,omitempty"`
	ApplicableAges       []string        `json:"applicable_ages,omitempty"`
	Reporters            []string        `json:"reporters,omitempty"`
	Tags                 []string        `json:"tags,omitempty"`
	QuestionnaireCode    string          `json:"questionnaire_code,omitempty"`
	QuestionnaireVersion string          `json:"questionnaire_version,omitempty"`
	Definition           json.RawMessage `json:"definition,omitempty"`
}
type ListRequest struct {
	Kind                 string `form:"kind"`
	Kinds                string `form:"kinds"`
	Algorithm            string `form:"algorithm"`
	Category             string `form:"category"`
	Keyword              string `form:"keyword"`
	QuestionnaireCode    string `form:"questionnaire_code"`
	QuestionnaireVersion string `form:"questionnaire_version"`
	Page                 int32  `form:"page"`
	PageSize             int32  `form:"page_size"`
	kinds                []string
}
type ListResponse struct {
	Models   []ModelResponse `json:"models"`
	Total    int64           `json:"total"`
	Page     int32           `json:"page"`
	PageSize int32           `json:"page_size"`
}
type HotRequest struct {
	Kind       string `form:"kind"`
	Limit      int32  `form:"limit"`
	WindowDays int32  `form:"window_days"`
}
type HotResponse struct {
	Models     []HotModelResponse `json:"models"`
	Total      int64              `json:"total"`
	Limit      int32              `json:"limit"`
	WindowDays int32              `json:"window_days"`
}
type HotModelResponse struct {
	ModelResponse
	Rank            int32 `json:"rank"`
	SubmissionCount int64 `json:"submission_count"`
	HeatScore       int64 `json:"heat_score"`
}
type OptionResponse struct {
	Label    string `json:"label"`
	Value    string `json:"value"`
	Disabled bool   `json:"disabled,omitempty"`
}
type OptionsResponse struct {
	Kinds          []OptionResponse `json:"kinds"`
	Algorithms     []OptionResponse `json:"algorithms"`
	Categories     []OptionResponse `json:"categories"`
	Stages         []OptionResponse `json:"stages"`
	ApplicableAges []OptionResponse `json:"applicable_ages"`
	Reporters      []OptionResponse `json:"reporters"`
}

func (s *QueryService) Get(ctx context.Context, code string) (*ModelResponse, error) {
	if s == nil || s.client == nil {
		return nil, nil
	}
	value, err := s.client.GetPublishedModel(ctx, code, "")
	if err != nil || value == nil {
		return nil, err
	}
	return s.modelResponse(value), nil
}
func (s *QueryService) List(ctx context.Context, request *ListRequest) (*ListResponse, error) {
	if request == nil {
		request = &ListRequest{}
	}
	if err := normalizeList(request); err != nil {
		return nil, err
	}
	if s == nil || s.client == nil {
		return nil, nil
	}
	value, err := s.client.ListPublishedModels(ctx, request.Kind, request.kinds, request.Algorithm, request.Category, request.Keyword, request.QuestionnaireCode, request.QuestionnaireVersion, request.Page, request.PageSize)
	if err != nil || value == nil {
		return nil, err
	}
	result := &ListResponse{Models: make([]ModelResponse, 0, len(value.Models)), Total: value.Total, Page: value.Page, PageSize: value.PageSize}
	for index := range value.Models {
		result.Models = append(result.Models, *s.modelResponse(&value.Models[index]))
	}
	return result, nil
}
func (s *QueryService) ListHot(ctx context.Context, request *HotRequest) (*HotResponse, error) {
	if request == nil {
		request = &HotRequest{}
	}
	if request.Kind == "" {
		request.Kind = "scale"
	}
	if request.Limit <= 0 {
		request.Limit = 5
	}
	if request.WindowDays <= 0 {
		request.WindowDays = 30
	}
	if s == nil || s.client == nil {
		return nil, nil
	}
	value, err := s.client.ListHotPublishedModels(ctx, request.Kind, request.Limit, request.WindowDays)
	if err != nil || value == nil {
		return nil, err
	}
	result := &HotResponse{Models: make([]HotModelResponse, 0, len(value.Models)), Total: value.Total, Limit: value.Limit, WindowDays: value.WindowDays}
	for _, item := range value.Models {
		result.Models = append(result.Models, HotModelResponse{ModelResponse: *s.modelResponse(&item.Model), Rank: item.Rank, SubmissionCount: item.SubmissionCount, HeatScore: item.HeatScore})
	}
	return result, nil
}
func (s *QueryService) Options(ctx context.Context, kind string) (*OptionsResponse, error) {
	if s == nil || s.client == nil {
		return nil, nil
	}
	value, err := s.client.GetCatalogOptions(ctx, kind)
	if err != nil || value == nil {
		return nil, err
	}
	return &OptionsResponse{Kinds: options(value.Kinds), Algorithms: options(value.Algorithms), Categories: options(value.Categories), Stages: options(value.Stages), ApplicableAges: options(value.ApplicableAges), Reporters: options(value.Reporters)}, nil
}

// VisibleFactorCodes resolves the factor-score report mapping from canonical
// DefinitionV2 JSON. The bool is false when the model has no factor-score
// mapping, which callers treat as "do not filter" rather than "hide all".
func (s *QueryService) VisibleFactorCodes(ctx context.Context, code string) (map[string]bool, bool, error) {
	model, err := s.Get(ctx, code)
	if err != nil || model == nil {
		return nil, false, err
	}
	return visibleFactorCodesFromDefinition(model.Definition)
}
func normalizeList(request *ListRequest) error {
	if request.Kind != "" && strings.TrimSpace(request.Kinds) != "" {
		return fmt.Errorf("kind and kinds cannot be used together")
	}
	if request.Kinds != "" {
		seen := make(map[string]struct{})
		for _, raw := range strings.Split(request.Kinds, ",") {
			kind := strings.TrimSpace(raw)
			if kind == "" {
				continue
			}
			if _, exists := seen[kind]; exists {
				continue
			}
			seen[kind] = struct{}{}
			request.kinds = append(request.kinds, kind)
		}
	}
	if request.Page <= 0 {
		request.Page = 1
	}
	if request.PageSize <= 0 || request.PageSize > 100 {
		request.PageSize = 20
	}
	return nil
}
func (s *QueryService) modelResponse(value *CatalogModel) *ModelResponse {
	if value == nil {
		return nil
	}
	return &ModelResponse{
		Code: value.Code, Kind: value.Kind, Algorithm: value.Algorithm, DecisionKind: value.DecisionKind,
		Version: value.Version, Title: value.Title, Description: value.Description, Status: value.Status, Category: value.Category,
		Stages: append([]string(nil), value.Stages...), ApplicableAges: append([]string(nil), value.ApplicableAges...),
		Reporters: append([]string(nil), value.Reporters...), Tags: append([]string(nil), value.Tags...),
		QuestionnaireCode: value.QuestionnaireCode, QuestionnaireVersion: value.QuestionnaireVersion,
		Definition: append(json.RawMessage(nil), value.Definition...),
	}
}
func options(values []CatalogOption) []OptionResponse {
	result := make([]OptionResponse, 0, len(values))
	for _, value := range values {
		result = append(result, OptionResponse(value))
	}
	return result
}

func visibleFactorCodesFromDefinition(raw json.RawMessage) (map[string]bool, bool, error) {
	if len(raw) == 0 {
		return nil, false, nil
	}
	var payload struct {
		ReportMap struct {
			Sections []struct {
				Kind       string   `json:"Kind"`
				SourceRefs []string `json:"SourceRefs"`
			} `json:"Sections"`
		} `json:"ReportMap"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, false, err
	}
	for _, section := range payload.ReportMap.Sections {
		if section.Kind != "factor_scores" {
			continue
		}
		visible := make(map[string]bool, len(section.SourceRefs))
		for _, code := range section.SourceRefs {
			if code != "" {
				visible[code] = true
			}
		}
		return visible, true, nil
	}
	return nil, false, nil
}
