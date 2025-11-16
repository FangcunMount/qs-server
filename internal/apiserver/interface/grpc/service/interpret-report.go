package service

import (
	"context"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/dto"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpret-report/port"
	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/interpret-report"
	"github.com/FangcunMount/qs-server/pkg/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// InterpretReportService 解读报告 gRPC 服务
type InterpretReportService struct {
	pb.UnimplementedInterpretReportServiceServer
	interpretReportCreator port.InterpretReportCreator
	interpretReportQueryer port.InterpretReportQueryer
}

// NewInterpretReportService 创建解读报告服务
func NewInterpretReportService(creator port.InterpretReportCreator, queryer port.InterpretReportQueryer) *InterpretReportService {
	return &InterpretReportService{
		interpretReportCreator: creator,
		interpretReportQueryer: queryer,
	}
}

// RegisterService 注册 GRPC 服务
func (s *InterpretReportService) RegisterService(server *grpc.Server) {
	pb.RegisterInterpretReportServiceServer(server, s)
}

// SaveInterpretReport 保存解读报告
func (s *InterpretReportService) SaveInterpretReport(ctx context.Context, req *pb.SaveInterpretReportRequest) (*pb.SaveInterpretReportResponse, error) {
	if req.AnswerSheetId == 0 {
		return nil, status.Error(codes.InvalidArgument, "答卷ID不能为空")
	}
	if req.MedicalScaleCode == "" {
		return nil, status.Error(codes.InvalidArgument, "医学量表代码不能为空")
	}
	if req.Title == "" {
		return nil, status.Error(codes.InvalidArgument, "标题不能为空")
	}
	if len(req.InterpretItems) == 0 {
		return nil, status.Error(codes.InvalidArgument, "解读项列表不能为空")
	}

	log.Infof("保存解读报告，答卷ID: %d", req.AnswerSheetId)

	// 转换请求为 DTO
	interpretReportDTO := &dto.InterpretReportDTO{
		AnswerSheetId:    req.AnswerSheetId,
		MedicalScaleCode: req.MedicalScaleCode,
		Title:            req.Title,
		Description:      req.Description,
		InterpretItems:   make([]dto.InterpretItemDTO, 0, len(req.InterpretItems)),
	}

	// 转换解读项
	for _, item := range req.InterpretItems {
		interpretReportDTO.InterpretItems = append(interpretReportDTO.InterpretItems, dto.InterpretItemDTO{
			FactorCode: item.FactorCode,
			Title:      item.Title,
			Score:      item.Score,
			Content:    item.Content,
		})
	}

	// 保存解读报告
	savedReport, err := s.interpretReportCreator.CreateInterpretReport(ctx, interpretReportDTO)
	if err != nil {
		log.Errorf("保存解读报告失败: %v", err)
		return nil, status.Error(codes.Internal, fmt.Sprintf("保存解读报告失败: %v", err))
	}

	return &pb.SaveInterpretReportResponse{
		Id:      savedReport.ID,
		Message: "解读报告保存成功",
	}, nil
}

// GetInterpretReportByAnswerSheetID 根据答卷ID获取解读报告
func (s *InterpretReportService) GetInterpretReportByAnswerSheetID(ctx context.Context, req *pb.GetInterpretReportByAnswerSheetIDRequest) (*pb.GetInterpretReportByAnswerSheetIDResponse, error) {
	if req.AnswerSheetId == 0 {
		return nil, status.Error(codes.InvalidArgument, "答卷ID不能为空")
	}

	log.Infof("获取解读报告，答卷ID: %d", req.AnswerSheetId)

	// 查询解读报告
	report, err := s.interpretReportQueryer.GetInterpretReportByAnswerSheetId(ctx, req.AnswerSheetId)
	if err != nil {
		log.Errorf("获取解读报告失败: %v", err)
		return nil, status.Error(codes.Internal, fmt.Sprintf("获取解读报告失败: %v", err))
	}

	if report == nil {
		return nil, status.Error(codes.NotFound, "解读报告不存在")
	}

	// 转换为 gRPC 响应
	response := &pb.GetInterpretReportByAnswerSheetIDResponse{
		InterpretReport: convertInterpretReportToProto(report),
	}

	return response, nil
}

// convertInterpretReportToProto 将 DTO 转换为 Proto 消息
func convertInterpretReportToProto(report *dto.InterpretReportDTO) *pb.InterpretReport {
	if report == nil {
		return nil
	}

	// 转换解读项列表
	interpretItems := make([]*pb.InterpretItem, 0, len(report.InterpretItems))
	for _, item := range report.InterpretItems {
		interpretItems = append(interpretItems, &pb.InterpretItem{
			FactorCode: item.FactorCode,
			Title:      item.Title,
			Score:      item.Score,
			Content:    item.Content,
		})
	}

	return &pb.InterpretReport{
		Id:               report.ID,
		AnswerSheetId:    report.AnswerSheetId,
		MedicalScaleCode: report.MedicalScaleCode,
		Title:            report.Title,
		Description:      report.Description,
		InterpretItems:   interpretItems,
		CreatedAt:        "", // DTO 中没有时间字段，暂时为空
		UpdatedAt:        "", // DTO 中没有时间字段，暂时为空
	}
}
