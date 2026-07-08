package service

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/typologymodel"
	appTypologyModel "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/typology/consumer"
)

// TypologyModelService exposes C-side typology model catalog reads over gRPC.
type TypologyModelService struct {
	pb.UnimplementedTypologyModelServiceServer
	queryService appTypologyModel.TypologyModelQueryService
}

func NewTypologyModelService(queryService appTypologyModel.TypologyModelQueryService) *TypologyModelService {
	return &TypologyModelService{queryService: queryService}
}

func (s *TypologyModelService) RegisterService(server *grpc.Server) {
	pb.RegisterTypologyModelServiceServer(server, s)
}

func (s *TypologyModelService) GetTypologyModel(ctx context.Context, req *pb.GetTypologyModelRequest) (*pb.GetTypologyModelResponse, error) {
	if req.GetCode() == "" {
		return nil, status.Error(codes.InvalidArgument, "code 不能为空")
	}
	result, err := s.queryService.GetPublishedByCode(ctx, req.GetCode())
	if err != nil {
		return nil, status.Error(codes.NotFound, "类型学模型不存在或未发布")
	}
	return &pb.GetTypologyModelResponse{Model: toProtoTypologyModel(result)}, nil
}

func (s *TypologyModelService) ListTypologyModels(ctx context.Context, req *pb.ListTypologyModelsRequest) (*pb.ListTypologyModelsResponse, error) {
	result, err := s.queryService.ListPublished(ctx, appTypologyModel.ListTypologyModelsDTO{
		Page:      int(req.GetPage()),
		PageSize:  int(req.GetPageSize()),
		Algorithm: req.GetAlgorithm(),
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	models := make([]*pb.TypologyModelSummary, 0, len(result.Items))
	for i := range result.Items {
		models = append(models, toProtoTypologyModelSummary(&result.Items[i]))
	}
	return &pb.ListTypologyModelsResponse{
		Models:     models,
		Total:      result.Total,
		Page:       int32(result.Page),
		PageSize:   int32(result.PageSize),
		TotalPages: int32(result.TotalPages),
	}, nil
}

func (s *TypologyModelService) GetTypologyModelCategories(ctx context.Context, _ *pb.GetTypologyModelCategoriesRequest) (*pb.GetTypologyModelCategoriesResponse, error) {
	result, err := s.queryService.GetCategories(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	categories := make([]*pb.TypologyModelCategory, 0, len(result.Categories))
	for _, item := range result.Categories {
		categories = append(categories, &pb.TypologyModelCategory{
			Value: item.Value,
			Label: item.Label,
		})
	}
	return &pb.GetTypologyModelCategoriesResponse{Categories: categories}, nil
}

func toProtoTypologyModelSummary(result *appTypologyModel.TypologyModelSummaryResult) *pb.TypologyModelSummary {
	if result == nil {
		return nil
	}
	return &pb.TypologyModelSummary{
		Code:                 result.Code,
		Version:              result.Version,
		Title:                result.Title,
		Algorithm:            result.Algorithm,
		Description:          result.Description,
		QuestionnaireCode:    result.QuestionnaireCode,
		QuestionnaireVersion: result.QuestionnaireVersion,
		Status:               result.Status,
		QuestionCount:        int32(result.QuestionCount),
		Kind:                 result.Kind,
		SubKind:              result.SubKind,
		ProductChannel:       result.ProductChannel,
		AlgorithmFamily:      result.AlgorithmFamily,
		PayloadFormat:        result.PayloadFormat,
		DecisionKind:         result.DecisionKind,
	}
}

func toProtoTypologyModel(result *appTypologyModel.TypologyModelResult) *pb.TypologyModel {
	if result == nil {
		return nil
	}
	dimensions := make([]*pb.TypologyDimension, 0, len(result.Dimensions))
	for _, dim := range result.Dimensions {
		dimensions = append(dimensions, &pb.TypologyDimension{
			Code:      dim.Code,
			Name:      dim.Name,
			LeftPole:  dim.LeftPole,
			RightPole: dim.RightPole,
		})
	}
	outcomes := make([]*pb.TypologyOutcomeSummary, 0, len(result.Outcomes))
	for _, outcome := range result.Outcomes {
		outcomes = append(outcomes, &pb.TypologyOutcomeSummary{
			Code:     outcome.Code,
			Name:     outcome.Name,
			OneLiner: outcome.OneLiner,
			ImageUrl: outcome.ImageURL,
		})
	}
	return &pb.TypologyModel{
		Summary:        toProtoTypologyModelSummary(&result.TypologyModelSummaryResult),
		DimensionOrder: append([]string(nil), result.DimensionOrder...),
		Dimensions:     dimensions,
		Outcomes:       outcomes,
	}
}
