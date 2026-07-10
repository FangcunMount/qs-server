package grpcclient

import (
	"context"
	"encoding/json"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/assessmentmodel"
)

// AssessmentModelCatalogClient is the collection-server adapter for the
// published-only AssessmentModelCatalogService.
type AssessmentModelCatalogClient struct {
	client     *Client
	grpcClient pb.AssessmentModelCatalogServiceClient
}

type CatalogModelOutput struct {
	Code                 string
	Kind                 string
	SubKind              string
	Algorithm            string
	ProductChannel       string
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

type CatalogListOutput struct {
	Models   []CatalogModelOutput
	Total    int64
	Page     int32
	PageSize int32
}

type HotCatalogModelOutput struct {
	Model           CatalogModelOutput
	Rank            int32
	SubmissionCount int64
	HeatScore       int64
}

type HotCatalogListOutput struct {
	Models     []HotCatalogModelOutput
	Total      int64
	Limit      int32
	WindowDays int32
}

type CatalogOptionOutput struct {
	Label, Value string
	Disabled     bool
}
type CatalogOptionsOutput struct {
	Kinds, ProductChannels, AlgorithmFamilies, Algorithms, SubKinds, Categories, Stages, ApplicableAges, Reporters []CatalogOptionOutput
}

func NewAssessmentModelCatalogClient(client *Client) *AssessmentModelCatalogClient {
	return &AssessmentModelCatalogClient{client: client, grpcClient: pb.NewAssessmentModelCatalogServiceClient(client.Conn())}
}

func (c *AssessmentModelCatalogClient) GetPublishedModel(ctx context.Context, code, version string) (*CatalogModelOutput, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()
	response, err := c.grpcClient.GetPublishedModel(ctx, &pb.GetPublishedModelRequest{Code: code, Version: version})
	if err != nil || response.GetModel() == nil {
		return nil, err
	}
	return catalogModelFromProto(response.GetModel()), nil
}

func (c *AssessmentModelCatalogClient) ListPublishedModels(ctx context.Context, kind, subKind, algorithm, category, keyword, questionnaireCode, questionnaireVersion string, page, pageSize int32) (*CatalogListOutput, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()
	response, err := c.grpcClient.ListPublishedModels(ctx, &pb.ListPublishedModelsRequest{Kind: kind, SubKind: subKind, Algorithm: algorithm, Category: category, Keyword: keyword, QuestionnaireCode: questionnaireCode, QuestionnaireVersion: questionnaireVersion, Page: page, PageSize: pageSize})
	if err != nil {
		return nil, err
	}
	items := make([]CatalogModelOutput, 0, len(response.GetModels()))
	for _, item := range response.GetModels() {
		items = append(items, *catalogModelFromProto(item))
	}
	return &CatalogListOutput{Models: items, Total: response.GetTotal(), Page: response.GetPage(), PageSize: response.GetPageSize()}, nil
}

func (c *AssessmentModelCatalogClient) ListHotPublishedModels(ctx context.Context, kind string, limit, windowDays int32) (*HotCatalogListOutput, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()
	response, err := c.grpcClient.ListHotPublishedModels(ctx, &pb.ListHotPublishedModelsRequest{Kind: kind, Limit: limit, WindowDays: windowDays})
	if err != nil {
		return nil, err
	}
	items := make([]HotCatalogModelOutput, 0, len(response.GetModels()))
	for _, item := range response.GetModels() {
		items = append(items, HotCatalogModelOutput{Model: *catalogSummaryFromProto(item.GetSummary()), Rank: item.GetRank(), SubmissionCount: item.GetSubmissionCount(), HeatScore: item.GetHeatScore()})
	}
	return &HotCatalogListOutput{Models: items, Total: response.GetTotal(), Limit: response.GetLimit(), WindowDays: response.GetWindowDays()}, nil
}

func (c *AssessmentModelCatalogClient) GetCatalogOptions(ctx context.Context, kind string) (*CatalogOptionsOutput, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()
	response, err := c.grpcClient.GetCatalogOptions(ctx, &pb.GetCatalogOptionsRequest{Kind: kind})
	if err != nil {
		return nil, err
	}
	return &CatalogOptionsOutput{Kinds: catalogOptionsFromProto(response.GetKinds()), ProductChannels: catalogOptionsFromProto(response.GetProductChannels()), AlgorithmFamilies: catalogOptionsFromProto(response.GetAlgorithmFamilies()), Algorithms: catalogOptionsFromProto(response.GetAlgorithms()), SubKinds: catalogOptionsFromProto(response.GetSubKinds()), Categories: catalogOptionsFromProto(response.GetCategories()), Stages: catalogOptionsFromProto(response.GetStages()), ApplicableAges: catalogOptionsFromProto(response.GetApplicableAges()), Reporters: catalogOptionsFromProto(response.GetReporters())}, nil
}

func catalogModelFromProto(value *pb.PublishedAssessmentModel) *CatalogModelOutput {
	if value == nil {
		return nil
	}
	result := catalogSummaryFromProto(value.GetSummary())
	result.Version = value.GetVersion()
	result.Definition = append(json.RawMessage(nil), value.GetDefinitionJson()...)
	return result
}

func catalogSummaryFromProto(value *pb.CatalogModelSummary) *CatalogModelOutput {
	if value == nil {
		return &CatalogModelOutput{}
	}
	return &CatalogModelOutput{Code: value.GetCode(), Kind: value.GetKind(), SubKind: value.GetSubKind(), Algorithm: value.GetAlgorithm(), ProductChannel: value.GetProductChannel(), Title: value.GetTitle(), Description: value.GetDescription(), Status: value.GetStatus(), Category: value.GetCategory(), Stages: append([]string(nil), value.GetStages()...), ApplicableAges: append([]string(nil), value.GetApplicableAges()...), Reporters: append([]string(nil), value.GetReporters()...), Tags: append([]string(nil), value.GetTags()...), QuestionnaireCode: value.GetQuestionnaireCode(), QuestionnaireVersion: value.GetQuestionnaireVersion()}
}

func catalogOptionsFromProto(values []*pb.CatalogOption) []CatalogOptionOutput {
	items := make([]CatalogOptionOutput, 0, len(values))
	for _, value := range values {
		items = append(items, CatalogOptionOutput{Label: value.GetLabel(), Value: value.GetValue(), Disabled: value.GetDisabled()})
	}
	return items
}
