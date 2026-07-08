package grpcclient

import (
	"context"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/typologymodel"
)

type TypologyModelOutput struct {
	Summary        TypologyModelSummaryOutput
	DimensionOrder []string
	Dimensions     []TypologyDimensionOutput
	Outcomes       []TypologyOutcomeSummaryOutput
}

type TypologyModelSummaryOutput struct {
	Code                 string
	Version              string
	Title                string
	Algorithm            string
	Description          string
	QuestionnaireCode    string
	QuestionnaireVersion string
	Status               string
	QuestionCount        int32
	Kind                 string
	SubKind              string
	ProductChannel       string
	AlgorithmFamily      string
	PayloadFormat        string
	DecisionKind         string
}

type TypologyDimensionOutput struct {
	Code      string
	Name      string
	LeftPole  string
	RightPole string
}

type TypologyOutcomeSummaryOutput struct {
	Code     string
	Name     string
	OneLiner string
	ImageURL string
}

type ListTypologyModelsOutput struct {
	Models     []TypologyModelSummaryOutput
	Total      int64
	Page       int32
	PageSize   int32
	TotalPages int32
}

type TypologyModelCategoriesOutput struct {
	Categories []CategoryOutput
}

type TypologyModelClient struct {
	client     *Client
	grpcClient pb.TypologyModelServiceClient
}

func NewTypologyModelClient(client *Client) *TypologyModelClient {
	return &TypologyModelClient{
		client:     client,
		grpcClient: pb.NewTypologyModelServiceClient(client.Conn()),
	}
}

func (c *TypologyModelClient) GetTypologyModel(ctx context.Context, code string) (*TypologyModelOutput, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()

	resp, err := c.grpcClient.GetTypologyModel(ctx, &pb.GetTypologyModelRequest{Code: code})
	if err != nil {
		return nil, err
	}
	model := resp.GetModel()
	if model == nil {
		return nil, nil
	}
	return convertTypologyModel(model), nil
}

func (c *TypologyModelClient) ListTypologyModels(ctx context.Context, page, pageSize int32, algorithm string) (*ListTypologyModelsOutput, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()

	resp, err := c.grpcClient.ListTypologyModels(ctx, &pb.ListTypologyModelsRequest{
		Page:      page,
		PageSize:  pageSize,
		Algorithm: algorithm,
	})
	if err != nil {
		return nil, err
	}
	models := make([]TypologyModelSummaryOutput, 0, len(resp.GetModels()))
	for _, model := range resp.GetModels() {
		models = append(models, convertTypologyModelSummary(model))
	}
	return &ListTypologyModelsOutput{
		Models:     models,
		Total:      resp.GetTotal(),
		Page:       resp.GetPage(),
		PageSize:   resp.GetPageSize(),
		TotalPages: resp.GetTotalPages(),
	}, nil
}

func (c *TypologyModelClient) GetTypologyModelCategories(ctx context.Context) (*TypologyModelCategoriesOutput, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()

	resp, err := c.grpcClient.GetTypologyModelCategories(ctx, &pb.GetTypologyModelCategoriesRequest{})
	if err != nil {
		return nil, err
	}
	categories := make([]CategoryOutput, 0, len(resp.GetCategories()))
	for _, item := range resp.GetCategories() {
		categories = append(categories, CategoryOutput{
			Value: item.GetValue(),
			Label: item.GetLabel(),
		})
	}
	return &TypologyModelCategoriesOutput{Categories: categories}, nil
}

func convertTypologyModel(model *pb.TypologyModel) *TypologyModelOutput {
	if model == nil {
		return nil
	}
	dimensions := make([]TypologyDimensionOutput, 0, len(model.GetDimensions()))
	for _, dim := range model.GetDimensions() {
		dimensions = append(dimensions, TypologyDimensionOutput{
			Code:      dim.GetCode(),
			Name:      dim.GetName(),
			LeftPole:  dim.GetLeftPole(),
			RightPole: dim.GetRightPole(),
		})
	}
	outcomes := make([]TypologyOutcomeSummaryOutput, 0, len(model.GetOutcomes()))
	for _, outcome := range model.GetOutcomes() {
		outcomes = append(outcomes, TypologyOutcomeSummaryOutput{
			Code:     outcome.GetCode(),
			Name:     outcome.GetName(),
			OneLiner: outcome.GetOneLiner(),
			ImageURL: outcome.GetImageUrl(),
		})
	}
	return &TypologyModelOutput{
		Summary:        convertTypologyModelSummary(model.GetSummary()),
		DimensionOrder: append([]string(nil), model.GetDimensionOrder()...),
		Dimensions:     dimensions,
		Outcomes:       outcomes,
	}
}

func convertTypologyModelSummary(summary *pb.TypologyModelSummary) TypologyModelSummaryOutput {
	if summary == nil {
		return TypologyModelSummaryOutput{}
	}
	return TypologyModelSummaryOutput{
		Code:                 summary.GetCode(),
		Version:              summary.GetVersion(),
		Title:                summary.GetTitle(),
		Algorithm:            summary.GetAlgorithm(),
		Description:          summary.GetDescription(),
		QuestionnaireCode:    summary.GetQuestionnaireCode(),
		QuestionnaireVersion: summary.GetQuestionnaireVersion(),
		Status:               summary.GetStatus(),
		QuestionCount:        summary.GetQuestionCount(),
		Kind:                 summary.GetKind(),
		SubKind:              summary.GetSubKind(),
		ProductChannel:       summary.GetProductChannel(),
		AlgorithmFamily:      summary.GetAlgorithmFamily(),
		PayloadFormat:        summary.GetPayloadFormat(),
		DecisionKind:         summary.GetDecisionKind(),
	}
}
