package service

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/answersheet"
)

// AnswerSheetService 答卷 gRPC 服务 - C端接口
// 提供答卷的提交、查询功能：提交答卷、查看我的答卷列表、查看我的答卷详情
type AnswerSheetService struct {
	pb.UnimplementedAnswerSheetServiceServer
	submissionService answersheet.AnswerSheetSubmissionService
	managementService answersheet.AnswerSheetManagementService
}

// NewAnswerSheetService 创建答卷 gRPC 服务
func NewAnswerSheetService(
	submissionService answersheet.AnswerSheetSubmissionService,
	managementService answersheet.AnswerSheetManagementService,
) *AnswerSheetService {
	return &AnswerSheetService{
		submissionService: submissionService,
		managementService: managementService,
	}
}

// RegisterService 注册 gRPC 服务
func (s *AnswerSheetService) RegisterService(server *grpc.Server) {
	pb.RegisterAnswerSheetServiceServer(server, s)
}

// SaveAnswerSheet 保存答卷（C端）
// @Description C端用户填写完问卷后提交答案
func (s *AnswerSheetService) SaveAnswerSheet(ctx context.Context, req *pb.SaveAnswerSheetRequest) (*pb.SaveAnswerSheetResponse, error) {
	// 参数校验
	if req.QuestionnaireCode == "" {
		return nil, status.Error(codes.InvalidArgument, "questionnaire_code 不能为空")
	}
	if req.WriterId == 0 {
		return nil, status.Error(codes.InvalidArgument, "writer_id 不能为空")
	}
	if req.TesteeId == 0 {
		return nil, status.Error(codes.InvalidArgument, "testee_id 不能为空")
	}
	if len(req.Answers) == 0 {
		return nil, status.Error(codes.InvalidArgument, "answers 不能为空")
	}

	// 转换请求为 DTO
	answers := make([]answersheet.AnswerDTO, 0, len(req.Answers))
	for _, a := range req.Answers {
		answers = append(answers, answersheet.AnswerDTO{
			QuestionCode: a.QuestionCode,
			QuestionType: a.QuestionType,
			Value:        a.Value, // proto 中使用 JSON 字符串
		})
	}

	// 版本号处理：空字符串表示使用最新版本
	questionnaireVer := req.QuestionnaireVersion
	// 空字符串表示不指定版本，自动使用最新版

	dto := answersheet.SubmitAnswerSheetDTO{
		QuestionnaireCode: req.QuestionnaireCode,
		QuestionnaireVer:  questionnaireVer,
		TesteeID:          req.TesteeId, // 受试者ID（传递给测评层）
		OrgID:             req.OrgId,    // 机构ID（传递给测评层）
		FillerID:          req.WriterId, // proto 中使用 writer_id
		Answers:           answers,
	}

	// 调用应用服务
	result, err := s.submissionService.Submit(ctx, dto)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 转换响应
	return &pb.SaveAnswerSheetResponse{
		Id:      result.ID,
		Message: "答卷提交成功",
	}, nil
}

// GetAnswerSheet 获取答卷详情（C端）
// @Description C端用户查看自己提交的答卷详情
// Note: gRPC 内部调用，不进行权限验证
func (s *AnswerSheetService) GetAnswerSheet(ctx context.Context, req *pb.GetAnswerSheetRequest) (*pb.GetAnswerSheetResponse, error) {
	// 参数校验
	if req.Id == 0 {
		return nil, status.Error(codes.InvalidArgument, "id 不能为空")
	}

	// 直接获取答卷（不验证权限）
	result, err := s.managementService.GetByID(ctx, req.Id)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if result == nil {
		return nil, status.Error(codes.NotFound, "答卷不存在")
	}

	// 转换响应
	return &pb.GetAnswerSheetResponse{
		AnswerSheet: s.toProtoAnswerSheet(result),
	}, nil
}

// ListAnswerSheets 获取答卷列表（C端）
// @Description C端用户查看自己提交的所有答卷（返回摘要，不含 answers）
func (s *AnswerSheetService) ListAnswerSheets(ctx context.Context, req *pb.ListAnswerSheetsRequest) (*pb.ListAnswerSheetsResponse, error) {
	// 参数校验
	if req.WriterId == 0 {
		return nil, status.Error(codes.InvalidArgument, "writer_id 不能为空")
	}
	if req.Page < 1 {
		req.Page = 1 // 默认第一页
	}
	if req.PageSize < 1 || req.PageSize > 100 {
		req.PageSize = 20 // 默认每页20条，最大100条
	}

	dto := answersheet.ListMyAnswerSheetsDTO{
		FillerID:          req.WriterId, // proto 中使用 WriterId
		QuestionnaireCode: req.QuestionnaireCode,
		Page:              int(req.Page),
		PageSize:          int(req.PageSize),
	}

	// 调用应用服务
	result, err := s.submissionService.ListMyAnswerSheets(ctx, dto)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 转换响应（使用摘要类型，不含 answers）
	protoAnswerSheets := make([]*pb.AnswerSheetSummary, 0, len(result.Items))
	for _, item := range result.Items {
		protoAnswerSheets = append(protoAnswerSheets, &pb.AnswerSheetSummary{
			Id:                item.ID,
			QuestionnaireCode: item.QuestionnaireCode,
			Title:             item.QuestionnaireTitle,
			Score:             item.Score,
			WriterId:          item.FillerID,
			AnswerCount:       int32(item.AnswerCount),
		})
	}

	return &pb.ListAnswerSheetsResponse{
		AnswerSheets: protoAnswerSheets,
		Total:        result.Total,
	}, nil
}

// SaveAnswerSheetScores 保存答卷分数（内部接口）
// @Description 评分系统保存答卷分数
// Note: 这个接口应该由评分系统调用，不是 C 端接口
func (s *AnswerSheetService) SaveAnswerSheetScores(ctx context.Context, req *pb.SaveAnswerSheetScoresRequest) (*pb.SaveAnswerSheetScoresResponse, error) {
	// TODO: 实现评分功能
	// 这里暂时返回未实现的错误
	return nil, status.Error(codes.Unimplemented, "评分功能暂未实现")
}

// toProtoAnswerSheet 转换为 protobuf 答卷
func (s *AnswerSheetService) toProtoAnswerSheet(result *answersheet.AnswerSheetResult) *pb.AnswerSheet {
	if result == nil {
		return nil
	}

	// 转换答案列表
	protoAnswers := make([]*pb.Answer, 0, len(result.Answers))
	for _, a := range result.Answers {
		// 将答案值转换为 JSON 字符串（proto 中使用 string）
		valueStr := s.valueToString(a.Value)
		protoAnswers = append(protoAnswers, &pb.Answer{
			QuestionCode: a.QuestionCode,
			QuestionType: a.QuestionType,
			Value:        valueStr,
			Score:        uint32(a.Score),
		})
	}

	return &pb.AnswerSheet{
		Id:                result.ID,
		QuestionnaireCode: result.QuestionnaireCode,
		Title:             result.QuestionnaireTitle,
		Score:             result.Score,
		WriterId:          result.FillerID,
		WriterName:        result.FillerName,
		Answers:           protoAnswers,
		CreatedAt:         result.FilledAt.Format("2006-01-02 15:04:05"),
	}
}

// valueToString 将答案值转换为字符串（JSON 格式）
func (s *AnswerSheetService) valueToString(value interface{}) string {
	if value == nil {
		return ""
	}

	switch v := value.(type) {
	case string:
		return v
	case float64:
		return fmt.Sprintf("%f", v)
	case int:
		return fmt.Sprintf("%d", v)
	default:
		// 对于复杂类型，可以使用 JSON 序列化
		return fmt.Sprintf("%v", v)
	}
}
