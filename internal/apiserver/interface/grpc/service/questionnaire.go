package service

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/fangcun-mount/qs-server/internal/apiserver/application/dto"
	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/questionnaire/port"
	pb "github.com/fangcun-mount/qs-server/internal/apiserver/interface/grpc/proto/questionnaire"
)

// QuestionnaireService 问卷 GRPC 服务 - 对外提供查询功能
type QuestionnaireService struct {
	pb.UnimplementedQuestionnaireServiceServer
	queryer port.QuestionnaireQueryer
}

// NewQuestionnaireService 创建问卷 GRPC 服务
func NewQuestionnaireService(queryer port.QuestionnaireQueryer) *QuestionnaireService {
	return &QuestionnaireService{
		queryer: queryer,
	}
}

// RegisterService 注册 GRPC 服务
func (s *QuestionnaireService) RegisterService(server *grpc.Server) {
	pb.RegisterQuestionnaireServiceServer(server, s)
}

// GetQuestionnaire 获取问卷详情
func (s *QuestionnaireService) GetQuestionnaire(ctx context.Context, req *pb.GetQuestionnaireRequest) (*pb.GetQuestionnaireResponse, error) {
	// 调用领域服务
	result, err := s.queryer.GetQuestionnaireByCode(ctx, req.Code)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 转换响应
	return &pb.GetQuestionnaireResponse{
		Questionnaire: s.toProtoQuestionnaire(result),
	}, nil
}

// ListQuestionnaires 获取问卷列表
func (s *QuestionnaireService) ListQuestionnaires(ctx context.Context, req *pb.ListQuestionnairesRequest) (*pb.ListQuestionnairesResponse, error) {
	// 构建查询条件
	conditions := make(map[string]string)
	if req.Status != "" {
		conditions["status"] = req.Status
	}
	if req.Title != "" {
		conditions["title"] = req.Title
	}

	// 调用领域服务
	questionnaires, total, err := s.queryer.ListQuestionnaires(ctx, int(req.Page), int(req.PageSize), conditions)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 转换响应
	protoQuestionnaires := make([]*pb.Questionnaire, len(questionnaires))
	for i, q := range questionnaires {
		protoQuestionnaires[i] = s.toProtoQuestionnaire(q)
	}

	return &pb.ListQuestionnairesResponse{
		Questionnaires: protoQuestionnaires,
		Total:          total,
	}, nil
}

// toProtoQuestionnaire 转换为 protobuf 问卷
func (s *QuestionnaireService) toProtoQuestionnaire(dto *dto.QuestionnaireDTO) *pb.Questionnaire {
	if dto == nil {
		return nil
	}

	return &pb.Questionnaire{
		Code:        dto.Code,
		Title:       dto.Title,
		Description: dto.Description,
		ImgUrl:      dto.ImgUrl,
		Status:      dto.Status,
		Version:     dto.Version,
		Questions:   s.toProtoQuestions(dto.Questions),
		// TODO: 添加 CreatedAt 和 UpdatedAt 字段到 DTO
		CreatedAt: "",
		UpdatedAt: "",
	}
}

// toProtoQuestions 转换为 protobuf 问题列表
func (s *QuestionnaireService) toProtoQuestions(questions []dto.QuestionDTO) []*pb.Question {
	protoQuestions := make([]*pb.Question, len(questions))
	for i, q := range questions {
		protoQuestions[i] = s.toProtoQuestion(&q)
	}
	return protoQuestions
}

// toProtoQuestion 转换为 protobuf 问题
func (s *QuestionnaireService) toProtoQuestion(dto *dto.QuestionDTO) *pb.Question {
	if dto == nil {
		return nil
	}

	return &pb.Question{
		Code:            dto.Code,
		Type:            dto.Type,
		Title:           dto.Title,
		Tips:            dto.Tips,
		Placeholder:     dto.Placeholder,
		Options:         s.toProtoOptions(dto.Options),
		ValidationRules: s.toProtoValidationRules(dto.ValidationRules),
		CalculationRule: s.toProtoCalculationRule(dto.CalculationRule),
	}
}

// toProtoOptions 转换为 protobuf 选项列表
func (s *QuestionnaireService) toProtoOptions(options []dto.OptionDTO) []*pb.Option {
	protoOptions := make([]*pb.Option, len(options))
	for i, o := range options {
		protoOptions[i] = &pb.Option{
			Code:    o.Code,
			Content: o.Content,
			Score:   int32(o.Score),
		}
	}
	return protoOptions
}

// toProtoValidationRules 转换为 protobuf 验证规则列表
func (s *QuestionnaireService) toProtoValidationRules(rules []dto.ValidationRuleDTO) []*pb.ValidationRule {
	protoRules := make([]*pb.ValidationRule, len(rules))
	for i, r := range rules {
		protoRules[i] = &pb.ValidationRule{
			RuleType:    r.RuleType,
			TargetValue: r.TargetValue,
		}
	}
	return protoRules
}

// toProtoCalculationRule 转换为 protobuf 计算规则
func (s *QuestionnaireService) toProtoCalculationRule(rule *dto.CalculationRuleDTO) *pb.CalculationRule {
	if rule == nil {
		return nil
	}

	return &pb.CalculationRule{
		FormulaType: rule.FormulaType,
	}
}
