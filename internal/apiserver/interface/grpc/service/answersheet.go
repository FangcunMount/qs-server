package service

import (
	"context"
	"encoding/json"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/dto"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/answersheet/port"
	pb "github.com/yshujie/questionnaire-scale/internal/apiserver/interface/grpc/proto/answersheet"
)

// AnswerSheetService 答卷 GRPC 服务 - 对外提供答卷管理功能
type AnswerSheetService struct {
	pb.UnimplementedAnswerSheetServiceServer
	saver   port.AnswerSheetSaver
	queryer port.AnswerSheetQueryer
}

// NewAnswerSheetService 创建答卷 GRPC 服务
func NewAnswerSheetService(saver port.AnswerSheetSaver, queryer port.AnswerSheetQueryer) *AnswerSheetService {
	return &AnswerSheetService{
		saver:   saver,
		queryer: queryer,
	}
}

// RegisterService 注册 GRPC 服务
func (s *AnswerSheetService) RegisterService(server *grpc.Server) {
	pb.RegisterAnswerSheetServiceServer(server, s)
}

// SaveAnswerSheet 保存答卷
func (s *AnswerSheetService) SaveAnswerSheet(ctx context.Context, req *pb.SaveAnswerSheetRequest) (*pb.SaveAnswerSheetResponse, error) {
	// 转换请求为 DTO
	dto := &dto.AnswerSheetDTO{
		QuestionnaireCode:    req.QuestionnaireCode,
		QuestionnaireVersion: req.QuestionnaireVersion,
		Title:                req.Title,
		WriterID:             req.WriterId,
		TesteeID:             req.TesteeId,
		Answers:              s.fromProtoAnswers(req.Answers),
	}

	// 调用领域服务
	savedDTO, err := s.saver.SaveOriginalAnswerSheet(ctx, *dto)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 转换响应
	return &pb.SaveAnswerSheetResponse{
		Id:      savedDTO.ID,
		Message: "答卷保存成功",
	}, nil
}

// GetAnswerSheet 获取答卷详情
func (s *AnswerSheetService) GetAnswerSheet(ctx context.Context, req *pb.GetAnswerSheetRequest) (*pb.GetAnswerSheetResponse, error) {
	// 调用领域服务
	detail, err := s.queryer.GetAnswerSheetByID(ctx, req.Id)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 转换响应
	return &pb.GetAnswerSheetResponse{
		AnswerSheet: s.toProtoAnswerSheet(detail),
	}, nil
}

// ListAnswerSheets 获取答卷列表
func (s *AnswerSheetService) ListAnswerSheets(ctx context.Context, req *pb.ListAnswerSheetsRequest) (*pb.ListAnswerSheetsResponse, error) {
	// 构建过滤条件
	filter := dto.AnswerSheetDTO{
		QuestionnaireCode:    req.QuestionnaireCode,
		QuestionnaireVersion: req.QuestionnaireVersion,
		WriterID:             req.WriterId,
		TesteeID:             req.TesteeId,
	}

	// 调用领域服务
	sheets, total, err := s.queryer.GetAnswerSheetList(ctx, filter, int(req.Page), int(req.PageSize))
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 转换响应
	protoSheets := make([]*pb.AnswerSheet, len(sheets))
	for i, sheet := range sheets {
		// 简化的答卷信息，不包含详细答案
		protoSheets[i] = &pb.AnswerSheet{
			Id:                   sheet.ID,
			QuestionnaireCode:    sheet.QuestionnaireCode,
			QuestionnaireVersion: sheet.QuestionnaireVersion,
			Title:                sheet.Title,
			Score:                uint32(sheet.Score),
			WriterId:             sheet.WriterID,
			TesteeId:             sheet.TesteeID,
			// 列表中不返回具体答案，减少数据传输量
			Answers:   nil,
			CreatedAt: "", // TODO: 添加时间字段
			UpdatedAt: "", // TODO: 添加时间字段
		}
	}

	return &pb.ListAnswerSheetsResponse{
		AnswerSheets: protoSheets,
		Total:        total,
	}, nil
}

// toProtoAnswerSheet 转换为 protobuf 答卷（详细信息）
func (s *AnswerSheetService) toProtoAnswerSheet(detail *dto.AnswerSheetDetailDTO) *pb.AnswerSheet {
	if detail == nil {
		return nil
	}

	return &pb.AnswerSheet{
		Id:                   detail.AnswerSheet.ID,
		QuestionnaireCode:    detail.AnswerSheet.QuestionnaireCode,
		QuestionnaireVersion: detail.AnswerSheet.QuestionnaireVersion,
		Title:                detail.AnswerSheet.Title,
		Score:                uint32(detail.AnswerSheet.Score),
		WriterId:             detail.AnswerSheet.WriterID,
		WriterName:           detail.WriterName,
		TesteeId:             detail.AnswerSheet.TesteeID,
		TesteeName:           detail.TesteeName,
		Answers:              s.toProtoAnswers(detail.AnswerSheet.Answers),
		CreatedAt:            detail.CreatedAt,
		UpdatedAt:            detail.UpdatedAt,
	}
}

// toProtoAnswers 转换为 protobuf 答案列表
func (s *AnswerSheetService) toProtoAnswers(answers []dto.AnswerDTO) []*pb.Answer {
	protoAnswers := make([]*pb.Answer, len(answers))
	for i, answer := range answers {
		// 将答案值转换为 JSON 字符串
		valueJSON, _ := json.Marshal(answer.Value)

		protoAnswers[i] = &pb.Answer{
			QuestionCode: answer.QuestionCode,
			QuestionType: answer.QuestionType,
			Score:        uint32(answer.Score),
			Value:        string(valueJSON),
		}
	}
	return protoAnswers
}

// fromProtoAnswers 从 protobuf 转换答案列表
func (s *AnswerSheetService) fromProtoAnswers(protoAnswers []*pb.Answer) []dto.AnswerDTO {
	answers := make([]dto.AnswerDTO, len(protoAnswers))
	for i, protoAnswer := range protoAnswers {
		// 从 JSON 字符串解析答案值
		var value interface{}
		if err := json.Unmarshal([]byte(protoAnswer.Value), &value); err != nil {
			// 如果解析失败，直接使用字符串值
			value = protoAnswer.Value
		}

		answers[i] = dto.AnswerDTO{
			QuestionCode: protoAnswer.QuestionCode,
			QuestionType: protoAnswer.QuestionType,
			Score:        uint16(protoAnswer.Score),
			Value:        value,
		}
	}
	return answers
}
