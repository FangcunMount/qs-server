package service

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
)

func (s *EvaluationService) GetMyAssessmentV2(ctx context.Context, req *pb.GetMyAssessmentV2Request) (*pb.GetMyAssessmentV2Response, error) {
	if req.TesteeId == 0 || req.AssessmentId == 0 {
		return nil, status.Error(codes.InvalidArgument, "testee_id 和 assessment_id 不能为空")
	}
	if _, err := s.submissionService.GetMyAssessment(ctx, req.TesteeId, req.AssessmentId); err != nil {
		return nil, toAssessmentQueryGRPCError(err)
	}
	result, err := s.loadAssessmentV2Row(ctx, req.AssessmentId)
	if err != nil {
		return nil, err
	}
	return &pb.GetMyAssessmentV2Response{Assessment: toEvaluationProtoAssessmentDetailV2(result)}, nil
}

func (s *EvaluationService) ListMyAssessmentsV2(ctx context.Context, req *pb.ListMyAssessmentsV2Request) (*pb.ListMyAssessmentsV2Response, error) {
	if req.TesteeId == 0 {
		return nil, status.Error(codes.InvalidArgument, "testee_id 不能为空")
	}
	page := int(req.Page)
	pageSize := int(req.PageSize)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	if s.assessmentReader == nil {
		listResult, err := s.submissionService.ListMyAssessments(ctx, assessmentApp.ListMyAssessmentsDTO{
			TesteeID: req.TesteeId, Page: page, PageSize: pageSize, Status: req.Status,
			ScaleCode: req.ScaleCode, RiskLevel: req.RiskLevel,
		})
		if err != nil {
			return nil, toAssessmentQueryGRPCError(err)
		}
		items := make([]*pb.AssessmentSummaryV2, 0, len(listResult.Items))
		for _, item := range listResult.Items {
			items = append(items, toEvaluationProtoAssessmentSummaryV2(legacyAssessmentV2Result(item)))
		}
		return &pb.ListMyAssessmentsV2Response{
			Items: items, Total: int32(listResult.Total), Page: int32(listResult.Page),
			PageSize: int32(listResult.PageSize), TotalPages: int32(listResult.TotalPages),
		}, nil
	}
	testeeID := req.TesteeId
	filter := evaluationreadmodel.AssessmentFilter{
		TesteeID:  &testeeID,
		Statuses:  normalizeGRPCAssessmentStatuses(req.Status),
		ScaleCode: req.ScaleCode,
		RiskLevel: req.RiskLevel,
	}
	if req.DateFrom != "" {
		if parsed, err := time.Parse(time.RFC3339, req.DateFrom); err == nil {
			filter.DateFrom = &parsed
		}
	}
	if req.DateTo != "" {
		if parsed, err := time.Parse(time.RFC3339, req.DateTo); err == nil {
			filter.DateTo = &parsed
		}
	}
	rows, total, err := s.assessmentReader.ListAssessments(ctx, filter, evaluationreadmodel.PageRequest{Page: page, PageSize: pageSize})
	if err != nil {
		return nil, toAssessmentQueryGRPCError(err)
	}
	v2Items, err := assessmentApp.RowsToV2Results(rows)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	items := make([]*pb.AssessmentSummaryV2, 0, len(v2Items))
	for _, item := range v2Items {
		items = append(items, toEvaluationProtoAssessmentSummaryV2(item))
	}
	totalPages := int32((total + int64(pageSize) - 1) / int64(pageSize))
	if totalPages == 0 && total > 0 {
		totalPages = 1
	}
	return &pb.ListMyAssessmentsV2Response{
		Items: items, Total: int32(total), Page: int32(page), PageSize: int32(pageSize), TotalPages: totalPages,
	}, nil
}

func (s *EvaluationService) GetAssessmentReportV2(ctx context.Context, req *pb.GetAssessmentReportV2Request) (*pb.GetAssessmentReportV2Response, error) {
	if req.AssessmentId == 0 {
		return nil, status.Error(codes.InvalidArgument, "assessment_id 不能为空")
	}
	if s.reportQueryService == nil {
		return nil, status.Error(codes.FailedPrecondition, "report query service is not configured")
	}
	result, err := s.reportQueryService.GetV2ByAssessmentID(ctx, req.AssessmentId)
	if err != nil {
		return nil, toAssessmentQueryGRPCError(err)
	}
	if result == nil {
		return nil, status.Error(codes.NotFound, "报告不存在")
	}
	return &pb.GetAssessmentReportV2Response{Report: toEvaluationProtoAssessmentReportV2(result)}, nil
}

func (s *EvaluationService) loadAssessmentV2Row(ctx context.Context, assessmentID uint64) (*assessmentApp.AssessmentV2Result, error) {
	if s.assessmentReader == nil {
		return nil, status.Error(codes.FailedPrecondition, "assessment read model is not configured")
	}
	row, err := s.assessmentReader.GetAssessment(ctx, assessmentID)
	if err != nil {
		return nil, toAssessmentQueryGRPCError(err)
	}
	return assessmentApp.RowToV2Result(*row)
}

func normalizeGRPCAssessmentStatuses(raw string) []string {
	switch raw {
	case "":
		return nil
	case "pending":
		return []string{domainAssessment.StatusPending.String(), domainAssessment.StatusSubmitted.String()}
	case "done":
		return []string{domainAssessment.StatusInterpreted.String()}
	default:
		return []string{raw}
	}
}
