package service

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/personalitymodel"
	appPersonalityModel "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/typology/consumer"
)

// PersonalityModelService exposes C-side personality model catalog reads over gRPC.
type PersonalityModelService struct {
	pb.UnimplementedPersonalityModelServiceServer
	queryService appPersonalityModel.PersonalityModelQueryService
}

func NewPersonalityModelService(queryService appPersonalityModel.PersonalityModelQueryService) *PersonalityModelService {
	return &PersonalityModelService{queryService: queryService}
}

func (s *PersonalityModelService) RegisterService(server *grpc.Server) {
	pb.RegisterPersonalityModelServiceServer(server, s)
}

func (s *PersonalityModelService) GetPersonalityModel(ctx context.Context, req *pb.GetPersonalityModelRequest) (*pb.GetPersonalityModelResponse, error) {
	if req.GetCode() == "" {
		return nil, status.Error(codes.InvalidArgument, "code 不能为空")
	}
	result, err := s.queryService.GetPublishedByCode(ctx, req.GetCode())
	if err != nil {
		return nil, status.Error(codes.NotFound, "人格测评模型不存在或未发布")
	}
	return &pb.GetPersonalityModelResponse{Model: toProtoPersonalityModel(result)}, nil
}

func (s *PersonalityModelService) ListPersonalityModels(ctx context.Context, req *pb.ListPersonalityModelsRequest) (*pb.ListPersonalityModelsResponse, error) {
	result, err := s.queryService.ListPublished(ctx, appPersonalityModel.ListPersonalityModelsDTO{
		Page:      int(req.GetPage()),
		PageSize:  int(req.GetPageSize()),
		Algorithm: req.GetAlgorithm(),
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	models := make([]*pb.PersonalityModelSummary, 0, len(result.Items))
	for i := range result.Items {
		models = append(models, toProtoPersonalityModelSummary(&result.Items[i]))
	}
	return &pb.ListPersonalityModelsResponse{
		Models:     models,
		Total:      result.Total,
		Page:       int32(result.Page),
		PageSize:   int32(result.PageSize),
		TotalPages: int32(result.TotalPages),
	}, nil
}

func (s *PersonalityModelService) GetPersonalityModelCategories(ctx context.Context, _ *pb.GetPersonalityModelCategoriesRequest) (*pb.GetPersonalityModelCategoriesResponse, error) {
	result, err := s.queryService.GetCategories(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	categories := make([]*pb.PersonalityModelCategory, 0, len(result.Categories))
	for _, item := range result.Categories {
		categories = append(categories, &pb.PersonalityModelCategory{
			Value: item.Value,
			Label: item.Label,
		})
	}
	return &pb.GetPersonalityModelCategoriesResponse{Categories: categories}, nil
}

func toProtoPersonalityModelSummary(result *appPersonalityModel.PersonalityModelSummaryResult) *pb.PersonalityModelSummary {
	if result == nil {
		return nil
	}
	return &pb.PersonalityModelSummary{
		Code:                 result.Code,
		Version:              result.Version,
		Title:                result.Title,
		Algorithm:            result.Algorithm,
		Description:          result.Description,
		QuestionnaireCode:    result.QuestionnaireCode,
		QuestionnaireVersion: result.QuestionnaireVersion,
		Status:               result.Status,
		QuestionCount:        int32(result.QuestionCount),
	}
}

func toProtoPersonalityModel(result *appPersonalityModel.PersonalityModelResult) *pb.PersonalityModel {
	if result == nil {
		return nil
	}
	dimensions := make([]*pb.PersonalityDimension, 0, len(result.Dimensions))
	for _, dim := range result.Dimensions {
		dimensions = append(dimensions, &pb.PersonalityDimension{
			Code:      dim.Code,
			Name:      dim.Name,
			LeftPole:  dim.LeftPole,
			RightPole: dim.RightPole,
		})
	}
	outcomes := make([]*pb.PersonalityOutcomeSummary, 0, len(result.Outcomes))
	for _, outcome := range result.Outcomes {
		outcomes = append(outcomes, &pb.PersonalityOutcomeSummary{
			Code:     outcome.Code,
			Name:     outcome.Name,
			OneLiner: outcome.OneLiner,
			ImageUrl: outcome.ImageURL,
		})
	}
	return &pb.PersonalityModel{
		Summary:        toProtoPersonalityModelSummary(&result.PersonalityModelSummaryResult),
		DimensionOrder: append([]string(nil), result.DimensionOrder...),
		Dimensions:     dimensions,
		Outcomes:       outcomes,
	}
}
