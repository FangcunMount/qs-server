package service

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	participant "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/participant"
)

type ParticipantReportService struct {
	pb.UnimplementedParticipantReportServiceServer
	service participant.Service
}

func NewParticipantReportService(service participant.Service) *ParticipantReportService {
	return &ParticipantReportService{service: service}
}

func (s *ParticipantReportService) RegisterService(server *grpc.Server) {
	pb.RegisterParticipantReportServiceServer(server, s)
}

func (s *ParticipantReportService) GetAssessmentReport(ctx context.Context, req *pb.GetAssessmentReportRequest) (*pb.GetAssessmentReportResponse, error) {
	if req.TesteeId == 0 || req.AssessmentId == 0 {
		return nil, status.Error(codes.InvalidArgument, "testee_id 和 assessment_id 不能为空")
	}
	result, err := s.service.GetMyReport(ctx, participant.Actor{TesteeID: req.TesteeId}, participant.GetQuery{AssessmentID: req.AssessmentId})
	if err != nil {
		return nil, toAssessmentQueryGRPCError(err)
	}
	if result == nil {
		return nil, status.Error(codes.NotFound, "报告不存在")
	}
	return &pb.GetAssessmentReportResponse{Report: toProtoParticipantReport(result)}, nil
}

func (s *ParticipantReportService) ListMyReports(ctx context.Context, req *pb.ListMyReportsRequest) (*pb.ListMyReportsResponse, error) {
	if req.TesteeId == 0 {
		return nil, status.Error(codes.InvalidArgument, "testee_id 不能为空")
	}
	page, pageSize := int(req.Page), int(req.PageSize)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	result, err := s.service.ListMyReports(ctx, participant.Actor{TesteeID: req.TesteeId}, participant.ListQuery{Page: page, PageSize: pageSize})
	if err != nil {
		return nil, toAssessmentQueryGRPCError(err)
	}
	items := make([]*pb.AssessmentReport, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, toProtoParticipantReport(item))
	}
	total, err := protoInt32FromInt("total", result.Total)
	if err != nil {
		return nil, err
	}
	pageOut, err := protoInt32FromInt("page", result.Page)
	if err != nil {
		return nil, err
	}
	pageSizeOut, err := protoInt32FromInt("page_size", result.PageSize)
	if err != nil {
		return nil, err
	}
	totalPages, err := protoInt32FromInt("total_pages", result.TotalPages)
	if err != nil {
		return nil, err
	}
	return &pb.ListMyReportsResponse{Items: items, Total: total, Page: pageOut, PageSize: pageSizeOut, TotalPages: totalPages}, nil
}
