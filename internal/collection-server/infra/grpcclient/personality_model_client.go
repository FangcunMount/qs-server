package grpcclient

import (
	"context"

	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/personalitymodel"
)

type PersonalityModelOutput struct {
	Summary        PersonalityModelSummaryOutput
	DimensionOrder []string
	Dimensions     []PersonalityDimensionOutput
	Outcomes       []PersonalityOutcomeSummaryOutput
}

type PersonalityModelSummaryOutput struct {
	Code                 string
	Version              string
	Title                string
	Algorithm            string
	Description          string
	QuestionnaireCode    string
	QuestionnaireVersion string
	Status               string
	QuestionCount        int32
}

type PersonalityDimensionOutput struct {
	Code      string
	Name      string
	LeftPole  string
	RightPole string
}

type PersonalityOutcomeSummaryOutput struct {
	Code     string
	Name     string
	OneLiner string
	ImageURL string
}

type ListPersonalityModelsOutput struct {
	Models     []PersonalityModelSummaryOutput
	Total      int64
	Page       int32
	PageSize   int32
	TotalPages int32
}

type PersonalityModelCategoriesOutput struct {
	Categories []CategoryOutput
}

type PersonalityModelClient struct {
	client     *Client
	grpcClient pb.PersonalityModelServiceClient
}

func NewPersonalityModelClient(client *Client) *PersonalityModelClient {
	return &PersonalityModelClient{
		client:     client,
		grpcClient: pb.NewPersonalityModelServiceClient(client.Conn()),
	}
}

func (c *PersonalityModelClient) GetPersonalityModel(ctx context.Context, code string) (*PersonalityModelOutput, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()

	resp, err := c.grpcClient.GetPersonalityModel(ctx, &pb.GetPersonalityModelRequest{Code: code})
	if err != nil {
		return nil, err
	}
	model := resp.GetModel()
	if model == nil {
		return nil, nil
	}
	return convertPersonalityModel(model), nil
}

func (c *PersonalityModelClient) ListPersonalityModels(ctx context.Context, page, pageSize int32, algorithm string) (*ListPersonalityModelsOutput, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()

	resp, err := c.grpcClient.ListPersonalityModels(ctx, &pb.ListPersonalityModelsRequest{
		Page:      page,
		PageSize:  pageSize,
		Algorithm: algorithm,
	})
	if err != nil {
		return nil, err
	}
	models := make([]PersonalityModelSummaryOutput, 0, len(resp.GetModels()))
	for _, model := range resp.GetModels() {
		models = append(models, convertPersonalityModelSummary(model))
	}
	return &ListPersonalityModelsOutput{
		Models:     models,
		Total:      resp.GetTotal(),
		Page:       resp.GetPage(),
		PageSize:   resp.GetPageSize(),
		TotalPages: resp.GetTotalPages(),
	}, nil
}

func (c *PersonalityModelClient) GetPersonalityModelCategories(ctx context.Context) (*PersonalityModelCategoriesOutput, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()

	resp, err := c.grpcClient.GetPersonalityModelCategories(ctx, &pb.GetPersonalityModelCategoriesRequest{})
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
	return &PersonalityModelCategoriesOutput{Categories: categories}, nil
}

func convertPersonalityModel(model *pb.PersonalityModel) *PersonalityModelOutput {
	if model == nil {
		return nil
	}
	dimensions := make([]PersonalityDimensionOutput, 0, len(model.GetDimensions()))
	for _, dim := range model.GetDimensions() {
		dimensions = append(dimensions, PersonalityDimensionOutput{
			Code:      dim.GetCode(),
			Name:      dim.GetName(),
			LeftPole:  dim.GetLeftPole(),
			RightPole: dim.GetRightPole(),
		})
	}
	outcomes := make([]PersonalityOutcomeSummaryOutput, 0, len(model.GetOutcomes()))
	for _, outcome := range model.GetOutcomes() {
		outcomes = append(outcomes, PersonalityOutcomeSummaryOutput{
			Code:     outcome.GetCode(),
			Name:     outcome.GetName(),
			OneLiner: outcome.GetOneLiner(),
			ImageURL: outcome.GetImageUrl(),
		})
	}
	return &PersonalityModelOutput{
		Summary:        convertPersonalityModelSummary(model.GetSummary()),
		DimensionOrder: append([]string(nil), model.GetDimensionOrder()...),
		Dimensions:     dimensions,
		Outcomes:       outcomes,
	}
}

func convertPersonalityModelSummary(summary *pb.PersonalityModelSummary) PersonalityModelSummaryOutput {
	if summary == nil {
		return PersonalityModelSummaryOutput{}
	}
	return PersonalityModelSummaryOutput{
		Code:                 summary.GetCode(),
		Version:              summary.GetVersion(),
		Title:                summary.GetTitle(),
		Algorithm:            summary.GetAlgorithm(),
		Description:          summary.GetDescription(),
		QuestionnaireCode:    summary.GetQuestionnaireCode(),
		QuestionnaireVersion: summary.GetQuestionnaireVersion(),
		Status:               summary.GetStatus(),
		QuestionCount:        summary.GetQuestionCount(),
	}
}
