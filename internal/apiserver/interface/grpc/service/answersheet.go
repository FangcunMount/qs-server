package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/dto"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/answersheet/port"
	pb "github.com/yshujie/questionnaire-scale/internal/apiserver/interface/grpc/proto/answersheet"
	"github.com/yshujie/questionnaire-scale/pkg/log"
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
		Id:      savedDTO.ID.Value(),
		Message: "答卷保存成功",
	}, nil
}

// GetAnswerSheet 获取答卷详情
func (s *AnswerSheetService) GetAnswerSheet(ctx context.Context, req *pb.GetAnswerSheetRequest) (*pb.GetAnswerSheetResponse, error) {
	log.Infof("---- in grpc GetAnswerSheet: %d", req.Id)

	// 调用领域服务
	detail, err := s.queryer.GetAnswerSheetByID(ctx, req.Id)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 检查答卷是否存在
	if detail == nil {
		return nil, status.Error(codes.NotFound, "答卷不存在")
	}

	// 转换响应
	protoAnswerSheet := s.toProtoAnswerSheet(detail)
	if protoAnswerSheet == nil {
		return nil, status.Error(codes.Internal, "转换答卷数据失败")
	}

	return &pb.GetAnswerSheetResponse{
		AnswerSheet: protoAnswerSheet,
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
			Id:                   sheet.ID.Value(),
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

// SaveAnswerSheetScores 保存答卷答案和分数
func (s *AnswerSheetService) SaveAnswerSheetScores(ctx context.Context, req *pb.SaveAnswerSheetScoresRequest) (*pb.SaveAnswerSheetScoresResponse, error) {
	log.Infof("保存答卷答案和分数，答卷ID: %d, 总分: %d", req.AnswerSheetId, req.TotalScore)

	// 转换答案列表
	answers := s.fromProtoAnswers(req.Answers)

	// 调用领域服务保存分数
	savedDTO, err := s.saver.SaveAnswerSheetScores(ctx, req.AnswerSheetId, uint16(req.TotalScore), answers)
	if err != nil {
		log.Errorf("保存答卷分数失败: %v", err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 转换响应
	return &pb.SaveAnswerSheetScoresResponse{
		AnswerSheetId: savedDTO.ID.Value(),
		TotalScore:    uint32(savedDTO.Score),
		Message:       "答卷分数保存成功",
	}, nil
}

// toProtoAnswerSheet 转换为 protobuf 答卷（详细信息）
func (s *AnswerSheetService) toProtoAnswerSheet(detail *dto.AnswerSheetDetailDTO) *pb.AnswerSheet {
	if detail == nil {
		log.Warnf("AnswerSheetDetailDTO is nil")
		return nil
	}

	// 安全地获取答案列表
	var answers []*pb.Answer
	if detail.AnswerSheet.Answers != nil {
		answers = s.toProtoAnswers(detail.AnswerSheet.Answers)
	} else {
		answers = []*pb.Answer{} // 空切片而不是 nil
	}

	return &pb.AnswerSheet{
		Id:                   detail.AnswerSheet.ID.Value(),
		QuestionnaireCode:    detail.AnswerSheet.QuestionnaireCode,
		QuestionnaireVersion: detail.AnswerSheet.QuestionnaireVersion,
		Title:                detail.AnswerSheet.Title,
		Score:                uint32(detail.AnswerSheet.Score),
		WriterId:             detail.AnswerSheet.WriterID,
		WriterName:           detail.WriterName,
		TesteeId:             detail.AnswerSheet.TesteeID,
		TesteeName:           detail.TesteeName,
		Answers:              answers,
		CreatedAt:            detail.CreatedAt,
		UpdatedAt:            detail.UpdatedAt,
	}
}

// toProtoAnswers 转换为 protobuf 答案列表
func (s *AnswerSheetService) toProtoAnswers(answers []dto.AnswerDTO) []*pb.Answer {
	if answers == nil {
		return []*pb.Answer{} // 返回空切片而不是 nil
	}

	protoAnswers := make([]*pb.Answer, len(answers))
	for i, answer := range answers {
		// 根据问题类型处理答案值
		var valueStr string

		switch answer.QuestionType {
		case "single_choice":
			// 单选题答案应该是字符串
			if str, ok := answer.Value.(string); ok {
				valueStr = str
			} else {
				log.Errorf("Invalid single choice answer type: %T", answer.Value)
				valueStr = fmt.Sprintf("%v", answer.Value)
			}
		case "multiple_choice":
			// 多选题答案应该是字符串数组
			if valueBytes, err := json.Marshal(answer.Value); err == nil {
				valueStr = string(valueBytes)
			} else {
				log.Errorf("Failed to marshal multiple choice answer: %v", err)
				valueStr = fmt.Sprintf("%v", answer.Value)
			}
		case "text", "textarea":
			// 文本类答案直接转换为字符串
			if str, ok := answer.Value.(string); ok {
				valueStr = str
			} else {
				log.Errorf("Invalid text answer type: %T", answer.Value)
				valueStr = fmt.Sprintf("%v", answer.Value)
			}
		case "number", "rating":
			// 数值类答案需要转换为字符串
			switch v := answer.Value.(type) {
			case float64:
				valueStr = fmt.Sprintf("%f", v)
			case int:
				valueStr = fmt.Sprintf("%d", v)
			case string:
				valueStr = v
			default:
				log.Errorf("Invalid number answer type: %T", answer.Value)
				valueStr = fmt.Sprintf("%v", answer.Value)
			}
		default:
			// 其他类型答案统一转换为JSON
			if valueBytes, err := json.Marshal(answer.Value); err == nil {
				valueStr = string(valueBytes)
			} else {
				log.Errorf("Failed to marshal answer value: %v", err)
				valueStr = fmt.Sprintf("%v", answer.Value)
			}
		}

		protoAnswers[i] = &pb.Answer{
			QuestionCode: answer.QuestionCode,
			QuestionType: answer.QuestionType,
			Score:        uint32(answer.Score),
			Value:        valueStr,
		}
	}
	return protoAnswers
}

// fromProtoAnswers 从 protobuf 转换答案列表
func (s *AnswerSheetService) fromProtoAnswers(protoAnswers []*pb.Answer) []dto.AnswerDTO {
	answers := make([]dto.AnswerDTO, len(protoAnswers))
	for i, protoAnswer := range protoAnswers {
		// 根据问题类型处理答案值
		var value interface{}
		var err error

		// 设置默认问题类型（如果为空）
		questionType := protoAnswer.QuestionType
		if questionType == "" {
			questionType = "Radio" // 默认为单选题
		}

		switch questionType {
		case "single_choice":
			// 单选题答案应该是字符串
			value = protoAnswer.Value
		case "multiple_choice":
			// 多选题答案应该是字符串数组
			var options []string
			if err = json.Unmarshal([]byte(protoAnswer.Value), &options); err != nil {
				log.Errorf("Failed to unmarshal multiple choice answer: %v", err)
				value = protoAnswer.Value // 保持原始值
			} else {
				value = options
			}
		case "text", "textarea":
			// 文本类答案直接使用字符串
			value = protoAnswer.Value
		case "number", "rating":
			// 数值类答案需要转换为数值
			if num, err := strconv.ParseFloat(protoAnswer.Value, 64); err == nil {
				value = num
			} else {
				log.Errorf("Failed to parse number answer: %v", err)
				value = protoAnswer.Value // 保持原始值
			}
		default:
			// 其他类型答案尝试解析JSON
			if err = json.Unmarshal([]byte(protoAnswer.Value), &value); err != nil {
				log.Errorf("Failed to unmarshal answer value: %v", err)
				value = protoAnswer.Value // 保持原始值
			}
		}

		answers[i] = dto.AnswerDTO{
			QuestionCode: protoAnswer.QuestionCode,
			QuestionType: questionType,
			Score:        uint16(protoAnswer.Score),
			Value:        value,
		}
	}
	return answers
}
