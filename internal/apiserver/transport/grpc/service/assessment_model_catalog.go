package service

import (
	"context"
	"encoding/json"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pkgerrors "github.com/FangcunMount/component-base/pkg/errors"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/assessmentmodel"
	modelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/securityplane"
)

// AssessmentModelCatalogService is the published-only gRPC catalogue. It
// never decodes legacy payload JSON and exposes DefinitionV2 as canonical JSON
// bytes for downstream adapters.
type AssessmentModelCatalogService struct {
	pb.UnimplementedAssessmentModelCatalogServiceServer
	query modelcatalog.CatalogQueryService
	actor modelcatalog.ActorContext
}

func NewAssessmentModelCatalogService(query modelcatalog.CatalogQueryService) *AssessmentModelCatalogService {
	return &AssessmentModelCatalogService{
		query: query,
		actor: modelcatalog.ActorContext{Principal: securityplane.Principal{Kind: securityplane.PrincipalKindService, Source: securityplane.PrincipalSourceMTLS}},
	}
}

func (s *AssessmentModelCatalogService) RegisterService(server *grpc.Server) {
	pb.RegisterAssessmentModelCatalogServiceServer(server, s)
}

func (s *AssessmentModelCatalogService) GetPublishedModel(ctx context.Context, req *pb.GetPublishedModelRequest) (*pb.GetPublishedModelResponse, error) {
	model, err := s.query.GetPublished(ctx, s.actor, req.GetCode(), req.GetVersion())
	if err != nil {
		return nil, catalogStatusError(err)
	}
	value, err := toProtoPublishedModel(model)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &pb.GetPublishedModelResponse{Model: value}, nil
}

func (s *AssessmentModelCatalogService) ListPublishedModels(ctx context.Context, req *pb.ListPublishedModelsRequest) (*pb.ListPublishedModelsResponse, error) {
	result, err := s.query.ListPublished(ctx, s.actor, catalogListInput(req.GetKind(), req.GetSubKind(), req.GetAlgorithm(), req.GetCategory(), req.GetKeyword(), req.GetQuestionnaireCode(), req.GetQuestionnaireVersion(), req.GetPage(), req.GetPageSize()))
	if err != nil {
		return nil, catalogStatusError(err)
	}
	items := make([]*pb.PublishedAssessmentModel, 0, len(result.Items))
	for index := range result.Items {
		item, err := toProtoPublishedModel(&result.Items[index])
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		items = append(items, item)
	}
	return &pb.ListPublishedModelsResponse{Models: items, Total: result.Total, Page: int32(result.Page), PageSize: int32(result.PageSize)}, nil
}

func (s *AssessmentModelCatalogService) ListHotPublishedModels(ctx context.Context, req *pb.ListHotPublishedModelsRequest) (*pb.ListHotPublishedModelsResponse, error) {
	result, err := s.query.ListHotPublished(ctx, s.actor, modelcatalog.ListModelsDTO{Kind: req.GetKind()}, int(req.GetLimit()), int(req.GetWindowDays()))
	if err != nil {
		return nil, catalogStatusError(err)
	}
	items := make([]*pb.HotPublishedAssessmentModel, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, &pb.HotPublishedAssessmentModel{Summary: toProtoSummary(item.ModelSummary), Rank: int32(item.Rank), SubmissionCount: item.SubmissionCount, HeatScore: item.HeatScore})
	}
	return &pb.ListHotPublishedModelsResponse{Models: items, Total: result.Total, Limit: int32(result.Limit), WindowDays: int32(result.WindowDays)}, nil
}

func (s *AssessmentModelCatalogService) GetCatalogOptions(ctx context.Context, req *pb.GetCatalogOptionsRequest) (*pb.GetCatalogOptionsResponse, error) {
	result, err := s.query.Options(ctx, s.actor, req.GetKind())
	if err != nil {
		return nil, catalogStatusError(err)
	}
	return &pb.GetCatalogOptionsResponse{
		Kinds: optionValues(result.Kinds), ProductChannels: optionValues(result.ProductChannels), AlgorithmFamilies: optionValues(result.AlgorithmFamilies),
		Algorithms: optionValues(result.Algorithms), SubKinds: optionValues(result.SubKinds), Categories: optionValues(result.Categories),
		Stages: optionValues(result.Stages), ApplicableAges: optionValues(result.ApplicableAges), Reporters: optionValues(result.Reporters),
	}, nil
}

func catalogListInput(kind, subKind, algorithm, category, keyword, questionnaireCode, questionnaireVersion string, page, pageSize int32) modelcatalog.ListModelsDTO {
	return modelcatalog.ListModelsDTO{Kind: kind, SubKind: subKind, Algorithm: algorithm, Category: category, Keyword: keyword, QuestionnaireCode: questionnaireCode, QuestionnaireVersion: questionnaireVersion, Page: int(page), PageSize: int(pageSize)}
}

func toProtoSummary(summary modelcatalog.ModelSummary) *pb.CatalogModelSummary {
	return &pb.CatalogModelSummary{
		Code: summary.Code, Kind: summary.Kind, SubKind: summary.SubKind, Algorithm: summary.Algorithm,
		ProductChannel: summary.ProductChannel, Title: summary.Title, Description: summary.Description,
		Status: summary.Status, Category: summary.Category,
		Stages: append([]string(nil), summary.Stages...), ApplicableAges: append([]string(nil), summary.ApplicableAges...),
		Reporters: append([]string(nil), summary.Reporters...), Tags: append([]string(nil), summary.Tags...),
		QuestionnaireCode: summary.QuestionnaireCode, QuestionnaireVersion: summary.QuestionnaireVersion,
		AlgorithmFamily: summary.AlgorithmFamily, DecisionKind: summary.DecisionKind, PayloadFormat: summary.PayloadFormat,
	}
}

func toProtoPublishedModel(model *modelcatalog.PublishedModelDetail) (*pb.PublishedAssessmentModel, error) {
	if model == nil || model.Definition == nil {
		return nil, status.Error(codes.FailedPrecondition, "published model definition_v2 is required")
	}
	definition, err := json.Marshal(model.Definition)
	if err != nil {
		return nil, err
	}
	summary := toProtoSummary(model.ModelSummary)
	if summary != nil {
		if summary.DecisionKind == "" {
			summary.DecisionKind = model.DecisionKind
		}
		if summary.PayloadFormat == "" {
			summary.PayloadFormat = model.PayloadFormat
		}
	}
	return &pb.PublishedAssessmentModel{Summary: summary, Version: model.Version, DefinitionJson: definition}, nil
}

func optionValues(items []modelcatalog.Option) []*pb.CatalogOption {
	result := make([]*pb.CatalogOption, 0, len(items))
	for _, item := range items {
		result = append(result, &pb.CatalogOption{Label: item.Label, Value: item.Value, Disabled: item.Disabled})
	}
	return result
}

func catalogStatusError(err error) error {
	if domain.IsNotFound(err) {
		return status.Error(codes.NotFound, err.Error())
	}
	switch pkgerrors.ParseCoder(err).Code() {
	case errorCode.ErrPermissionDenied, errorCode.ErrForbidden:
		return status.Error(codes.PermissionDenied, err.Error())
	case errorCode.ErrInvalidArgument, errorCode.ErrValidation, errorCode.ErrBind:
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
