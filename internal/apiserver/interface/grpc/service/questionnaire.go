package service

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/questionnaire"
)

// QuestionnaireService 问卷 gRPC 服务 - C端接口
// 提供问卷的查询功能：列表查询、详情查看
type QuestionnaireService struct {
	pb.UnimplementedQuestionnaireServiceServer
	queryService questionnaire.QuestionnaireQueryService
}

// NewQuestionnaireService 创建问卷 gRPC 服务
func NewQuestionnaireService(queryService questionnaire.QuestionnaireQueryService) *QuestionnaireService {
	return &QuestionnaireService{
		queryService: queryService,
	}
}

// RegisterService 注册 gRPC 服务
func (s *QuestionnaireService) RegisterService(server *grpc.Server) {
	pb.RegisterQuestionnaireServiceServer(server, s)
}

// ListQuestionnaires 获取已发布的问卷列表（C端）
// @Description C端用户查看可用的问卷列表，使用轻量级摘要查询（不包含问题详情）
func (s *QuestionnaireService) ListQuestionnaires(ctx context.Context, req *pb.ListQuestionnairesRequest) (*pb.ListQuestionnairesResponse, error) {
	// 构建查询条件
	conditions := make(map[string]interface{})
	if req.Title != "" {
		conditions["title"] = req.Title
	}
	// 仅返回医学量表
	conditions["type"] = domainQuestionnaire.TypeMedicalScale.String()
	// C端只查询已发布的问卷

	dto := questionnaire.ListQuestionnairesDTO{
		Page:       int(req.Page),
		PageSize:   int(req.PageSize),
		Conditions: conditions,
	}

	// 调用应用服务 - 使用轻量级摘要查询
	result, err := s.queryService.ListPublished(ctx, dto)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 转换响应（使用摘要类型，不包含 questions）
	protoQuestionnaires := make([]*pb.QuestionnaireSummary, 0, len(result.Items))
	for _, item := range result.Items {
		protoQuestionnaires = append(protoQuestionnaires, s.toProtoQuestionnaireSummary(item))
	}

	return &pb.ListQuestionnairesResponse{
		Questionnaires: protoQuestionnaires,
		Total:          result.Total,
	}, nil
}

// GetQuestionnaire 获取已发布问卷的详情（C端）
// @Description C端用户查看问卷详情和题目
func (s *QuestionnaireService) GetQuestionnaire(ctx context.Context, req *pb.GetQuestionnaireRequest) (*pb.GetQuestionnaireResponse, error) {
	// 调用应用服务
	result, err := s.queryService.GetPublishedByCode(ctx, req.Code)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if result == nil {
		return nil, status.Error(codes.NotFound, "问卷不存在或未发布")
	}

	// 仅允许访问医学量表
	if result.Type != domainQuestionnaire.TypeMedicalScale.String() {
		return nil, status.Error(codes.NotFound, "问卷不存在或未发布")
	}

	// 转换响应
	return &pb.GetQuestionnaireResponse{
		Questionnaire: s.toProtoQuestionnaire(result),
	}, nil
}

// toProtoQuestionnaire 转换为 protobuf 问卷
func (s *QuestionnaireService) toProtoQuestionnaire(result *questionnaire.QuestionnaireResult) *pb.Questionnaire {
	if result == nil {
		return nil
	}

	// 转换问题列表
	protoQuestions := make([]*pb.Question, 0, len(result.Questions))
	for _, q := range result.Questions {
		protoQuestions = append(protoQuestions, &pb.Question{
			Code:    q.Code,
			Title:   q.Stem,
			Type:    q.Type,
			Tips:    q.Description,
			Options: s.toProtoOptions(q.Options),
		})
	}

	return &pb.Questionnaire{
		Code:        result.Code,
		Version:     result.Version,
		Title:       result.Title,
		Description: result.Description,
		ImgUrl:      result.ImgUrl,
		Status:      result.Status,
		Type:        result.Type,
		Questions:   protoQuestions,
	}
}

// toProtoOptions 转换选项列表
func (s *QuestionnaireService) toProtoOptions(options []questionnaire.OptionResult) []*pb.Option {
	protoOptions := make([]*pb.Option, 0, len(options))
	for _, opt := range options {
		protoOptions = append(protoOptions, &pb.Option{
			Code:    opt.Value,
			Content: opt.Label,
			Score:   int32(opt.Score),
		})
	}
	return protoOptions
}

// toProtoQuestionnaireSummary 转换为 protobuf 问卷摘要（不包含问题详情）
func (s *QuestionnaireService) toProtoQuestionnaireSummary(result *questionnaire.QuestionnaireSummaryResult) *pb.QuestionnaireSummary {
	if result == nil {
		return nil
	}

	return &pb.QuestionnaireSummary{
		Code:          result.Code,
		Version:       result.Version,
		Title:         result.Title,
		Description:   result.Description,
		ImgUrl:        result.ImgUrl,
		Status:        result.Status,
		Type:          result.Type,
		QuestionCount: int32(result.QuestionCount),
	}
}
