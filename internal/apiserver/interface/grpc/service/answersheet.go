package service

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/dto"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/answersheet/port"
	pb "github.com/yshujie/questionnaire-scale/internal/apiserver/interface/grpc/proto/answersheet"
)

// AnswerSheetService 答卷 GRPC 服务
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
		// ... 其他字段转换
	}

	// 调用领域服务
	savedDTO, err := s.saver.SaveOriginalAnswerSheet(ctx, *dto)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 转换响应
	return &pb.SaveAnswerSheetResponse{
		Id: savedDTO.ID,
	}, nil
}

// GetAnswerSheet 获取答卷
func (s *AnswerSheetService) GetAnswerSheet(ctx context.Context, req *pb.GetAnswerSheetRequest) (*pb.GetAnswerSheetResponse, error) {
	// 调用领域服务
	detail, err := s.queryer.GetAnswerSheetByID(ctx, req.Id)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 转换响应
	return &pb.GetAnswerSheetResponse{
		AnswerSheet: &pb.AnswerSheet{
			Id:                   detail.AnswerSheet.ID,
			QuestionnaireCode:    detail.AnswerSheet.QuestionnaireCode,
			QuestionnaireVersion: detail.AnswerSheet.QuestionnaireVersion,
			Title:                detail.AnswerSheet.Title,
			Score:                uint32(detail.AnswerSheet.Score),
			WriterId:             detail.AnswerSheet.WriterID,
			WriterName:           detail.WriterName,
			TesteeId:             detail.AnswerSheet.TesteeID,
			TesteeName:           detail.TesteeName,
			// ... 其他字段转换
		},
	}, nil
}

// ListAnswerSheets 获取答卷列表
func (s *AnswerSheetService) ListAnswerSheets(ctx context.Context, req *pb.ListAnswerSheetsRequest) (*pb.ListAnswerSheetsResponse, error) {
	// 构建过滤条件
	filter := &dto.AnswerSheetDTO{
		QuestionnaireCode:    req.QuestionnaireCode,
		QuestionnaireVersion: req.QuestionnaireVersion,
		WriterID:             req.WriterId,
		TesteeID:             req.TesteeId,
	}

	// 调用领域服务
	sheets, total, err := s.queryer.GetAnswerSheetList(ctx, *filter, int(req.Page), int(req.PageSize))
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 转换响应
	items := make([]*pb.AnswerSheet, len(sheets))
	for i, sheet := range sheets {
		items[i] = &pb.AnswerSheet{
			Id:                   sheet.ID,
			QuestionnaireCode:    sheet.QuestionnaireCode,
			QuestionnaireVersion: sheet.QuestionnaireVersion,
			Title:                sheet.Title,
			Score:                uint32(sheet.Score),
			WriterId:             sheet.WriterID,
			TesteeId:             sheet.TesteeID,
			// ... 其他字段转换
		}
	}

	return &pb.ListAnswerSheetsResponse{
		Total: total,
		Items: items,
	}, nil
}
