package report

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// reportQueryService 报告查询服务实现
type reportQueryService struct {
	reportRepo domainReport.ReportRepository
}

// NewReportQueryService 创建报告查询服务
func NewReportQueryService(reportRepo domainReport.ReportRepository) ReportQueryService {
	return &reportQueryService{
		reportRepo: reportRepo,
	}
}

// GetByID 根据报告ID获取报告
func (s *reportQueryService) GetByID(ctx context.Context, reportID uint64) (*ReportResult, error) {
	id := meta.FromUint64(reportID)
	report, err := s.reportRepo.FindByID(ctx, id)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInterpretReportNotFound, "报告不存在")
	}

	return ToReportResult(report), nil
}

// GetByAssessmentID 根据测评ID获取报告
func (s *reportQueryService) GetByAssessmentID(ctx context.Context, assessmentID uint64) (*ReportResult, error) {
	id := meta.FromUint64(assessmentID)
	report, err := s.reportRepo.FindByAssessmentID(ctx, id)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInterpretReportNotFound, "报告不存在")
	}

	return ToReportResult(report), nil
}

// ListByTesteeID 获取受试者的报告列表
func (s *reportQueryService) ListByTesteeID(ctx context.Context, dto ListReportsDTO) (*ReportListResult, error) {
	if dto.TesteeID == 0 {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "受试者ID不能为空")
	}

	page, pageSize := normalizePagination(dto.Page, dto.PageSize)
	testeeID := testee.NewID(dto.TesteeID)
	pagination := domainReport.NewPagination(page, pageSize)

	reports, total, err := s.reportRepo.FindByTesteeID(ctx, testeeID, pagination)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "查询报告列表失败")
	}

	items := make([]*ReportResult, len(reports))
	for i, r := range reports {
		items[i] = ToReportResult(r)
	}

	totalInt := int(total)
	return &ReportListResult{
		Items:      items,
		Total:      totalInt,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: (totalInt + pageSize - 1) / pageSize,
	}, nil
}

// ListHighRiskReports 获取高风险报告列表
func (s *reportQueryService) ListHighRiskReports(ctx context.Context, dto ListHighRiskReportsDTO) (*ReportListResult, error) {
	page, pageSize := normalizePagination(dto.Page, dto.PageSize)

	// 使用查询扩展仓储
	queryRepo, ok := s.reportRepo.(domainReport.ReportQueryRepository)
	if !ok {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "仓储不支持高风险报告查询")
	}

	offset := (page - 1) * pageSize
	reports, err := queryRepo.FindHighRiskReports(ctx, offset, pageSize)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "查询高风险报告失败")
	}

	items := make([]*ReportResult, len(reports))
	for i, r := range reports {
		items[i] = ToReportResult(r)
	}

	// 统计总数
	spec := domainReport.ReportQuerySpec{HighRiskOnly: true}
	total, err := queryRepo.CountBySpec(ctx, spec)
	if err != nil {
		total = int64(len(items))
	}

	totalInt := int(total)
	return &ReportListResult{
		Items:      items,
		Total:      totalInt,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: (totalInt + pageSize - 1) / pageSize,
	}, nil
}

// normalizePagination 规范化分页参数
func normalizePagination(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}
