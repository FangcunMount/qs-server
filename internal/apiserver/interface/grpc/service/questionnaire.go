package service

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/dto"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/port"
	pb "github.com/yshujie/questionnaire-scale/internal/apiserver/interface/grpc/proto/questionnaire"
)

// QuestionnaireService 问卷 GRPC 服务
type QuestionnaireService struct {
	pb.UnimplementedQuestionnaireServiceServer
	creator   port.QuestionnaireCreator
	editor    port.QuestionnaireEditor
	publisher port.QuestionnairePublisher
	queryer   port.QuestionnaireQueryer
}

// NewQuestionnaireService 创建问卷 GRPC 服务
func NewQuestionnaireService(
	creator port.QuestionnaireCreator,
	editor port.QuestionnaireEditor,
	publisher port.QuestionnairePublisher,
	queryer port.QuestionnaireQueryer,
) *QuestionnaireService {
	return &QuestionnaireService{
		creator:   creator,
		editor:    editor,
		publisher: publisher,
		queryer:   queryer,
	}
}

// RegisterService 注册 GRPC 服务
func (s *QuestionnaireService) RegisterService(server *grpc.Server) {
	pb.RegisterQuestionnaireServiceServer(server, s)
}

// CreateQuestionnaire 创建问卷
func (s *QuestionnaireService) CreateQuestionnaire(ctx context.Context, req *pb.CreateQuestionnaireRequest) (*pb.CreateQuestionnaireResponse, error) {
	// 转换请求为 DTO
	dto := &dto.QuestionnaireDTO{
		Title:       req.Title,
		Description: req.Description,
		ImgUrl:      req.ImgUrl,
	}

	// 调用领域服务
	result, err := s.creator.CreateQuestionnaire(ctx, dto)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 转换响应
	return &pb.CreateQuestionnaireResponse{
		Questionnaire: s.toProtoQuestionnaire(result),
	}, nil
}

// GetQuestionnaire 获取问卷
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

// EditBasicInfo 编辑问卷基本信息
func (s *QuestionnaireService) EditBasicInfo(ctx context.Context, req *pb.EditBasicInfoRequest) (*pb.EditBasicInfoResponse, error) {
	// 转换请求为 DTO
	dto := &dto.QuestionnaireDTO{
		Code:        req.Code,
		Title:       req.Title,
		Description: req.Description,
		ImgUrl:      req.ImgUrl,
	}

	// 调用领域服务
	result, err := s.editor.EditBasicInfo(ctx, dto)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 转换响应
	return &pb.EditBasicInfoResponse{
		Questionnaire: s.toProtoQuestionnaire(result),
	}, nil
}

// UpdateQuestions 更新问卷问题
func (s *QuestionnaireService) UpdateQuestions(ctx context.Context, req *pb.UpdateQuestionsRequest) (*pb.UpdateQuestionsResponse, error) {
	// 转换问题
	questions := s.fromProtoQuestions(req.Questions)

	// 调用领域服务
	result, err := s.editor.UpdateQuestions(ctx, req.Code, questions)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 转换响应
	return &pb.UpdateQuestionsResponse{
		Questionnaire: s.toProtoQuestionnaire(result),
	}, nil
}

// PublishQuestionnaire 发布问卷
func (s *QuestionnaireService) PublishQuestionnaire(ctx context.Context, req *pb.PublishQuestionnaireRequest) (*pb.PublishQuestionnaireResponse, error) {
	// 调用领域服务
	result, err := s.publisher.Publish(ctx, req.Code)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 转换响应
	return &pb.PublishQuestionnaireResponse{
		Questionnaire: s.toProtoQuestionnaire(result),
	}, nil
}

// UnpublishQuestionnaire 下架问卷
func (s *QuestionnaireService) UnpublishQuestionnaire(ctx context.Context, req *pb.UnpublishQuestionnaireRequest) (*pb.UnpublishQuestionnaireResponse, error) {
	// 调用领域服务
	result, err := s.publisher.Unpublish(ctx, req.Code)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 转换响应
	return &pb.UnpublishQuestionnaireResponse{
		Questionnaire: s.toProtoQuestionnaire(result),
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
		Id:          dto.Code,
		Type:        dto.Type,
		Title:       dto.Title,
		Description: dto.Tips,
		Required:    false, // TODO: 添加 Required 字段到 DTO
		Options:     s.toProtoOptions(dto.Options),
		// TODO: 添加验证规则和计算规则的转换
	}
}

// toProtoOptions 转换为 protobuf 选项列表
func (s *QuestionnaireService) toProtoOptions(options []dto.OptionDTO) []*pb.Option {
	protoOptions := make([]*pb.Option, len(options))
	for i, o := range options {
		protoOptions[i] = &pb.Option{
			Id:    o.Code,
			Text:  o.Content,
			Score: int32(o.Score),
		}
	}
	return protoOptions
}

// fromProtoQuestions 从 protobuf 转换问题列表
func (s *QuestionnaireService) fromProtoQuestions(protoQuestions []*pb.Question) []dto.QuestionDTO {
	questions := make([]dto.QuestionDTO, len(protoQuestions))
	for i, pq := range protoQuestions {
		questions[i] = *s.fromProtoQuestion(pq)
	}
	return questions
}

// fromProtoQuestion 从 protobuf 转换问题
func (s *QuestionnaireService) fromProtoQuestion(proto *pb.Question) *dto.QuestionDTO {
	if proto == nil {
		return nil
	}

	return &dto.QuestionDTO{
		Code:    proto.Id,
		Type:    proto.Type,
		Title:   proto.Title,
		Tips:    proto.Description,
		Options: s.fromProtoOptions(proto.Options),
		// TODO: 添加验证规则和计算规则的转换
	}
}

// fromProtoOptions 从 protobuf 转换选项列表
func (s *QuestionnaireService) fromProtoOptions(protoOptions []*pb.Option) []dto.OptionDTO {
	options := make([]dto.OptionDTO, len(protoOptions))
	for i, po := range protoOptions {
		options[i] = dto.OptionDTO{
			Code:    po.Id,
			Content: po.Text,
			Score:   int(po.Score),
		}
	}
	return options
}
