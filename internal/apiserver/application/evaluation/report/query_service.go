package report

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
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
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("获取报告",
		"action", "get_report",
		"resource", "report",
		"report_id", reportID,
	)

	id := meta.FromUint64(reportID)
	report, err := s.reportRepo.FindByID(ctx, id)
	if err != nil {
		l.Errorw("获取报告失败",
			"report_id", reportID,
			"action", "get_report",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrInterpretReportNotFound, "报告不存在")
	}

	duration := time.Since(startTime)
	l.Debugw("获取报告成功",
		"report_id", reportID,
		"result", "success",
		"duration_ms", duration.Milliseconds(),
	)

	return ToReportResult(report), nil
}

// GetByAssessmentID 根据测评ID获取报告
func (s *reportQueryService) GetByAssessmentID(ctx context.Context, assessmentID uint64) (*ReportResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("根据测评获取报告",
		"action", "get_report_by_assessment",
		"resource", "report",
		"assessment_id", assessmentID,
	)

	id := meta.FromUint64(assessmentID)
	report, err := s.reportRepo.FindByAssessmentID(ctx, id)
	if err != nil {
		l.Errorw("根据测评获取报告失败",
			"assessment_id", assessmentID,
			"action", "get_report_by_assessment",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrInterpretReportNotFound, "报告不存在")
	}

	duration := time.Since(startTime)
	l.Debugw("根据测评获取报告成功",
		"assessment_id", assessmentID,
		"result", "success",
		"duration_ms", duration.Milliseconds(),
	)

	return ToReportResult(report), nil
}

// ListByTesteeID 获取受试者的报告列表
func (s *reportQueryService) ListByTesteeID(ctx context.Context, dto ListReportsDTO) (*ReportListResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("查询受试者报告列表",
		"action", "list_reports",
		"testee_id", dto.TesteeID,
		"page", dto.Page,
		"page_size", dto.PageSize,
	)

	if dto.TesteeID == 0 {
		l.Warnw("受试者ID为空",
			"action", "list_reports",
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "受试者ID不能为空")
	}

	page, pageSize := normalizePagination(dto.Page, dto.PageSize)
	testeeID := testee.NewID(dto.TesteeID)
	pagination := domainReport.NewPagination(page, pageSize)

	l.Debugw("开始查询报告列表",
		"testee_id", dto.TesteeID,
		"page", page,
		"page_size", pageSize,
	)

	reports, total, err := s.reportRepo.FindByTesteeID(ctx, testeeID, pagination)
	if err != nil {
		l.Errorw("查询报告列表失败",
			"testee_id", dto.TesteeID,
			"action", "list_reports",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "查询报告列表失败")
	}

	items := make([]*ReportResult, len(reports))
	for i, r := range reports {
		items[i] = ToReportResult(r)
	}

	totalInt := int(total)
	duration := time.Since(startTime)
	l.Debugw("查询受试者报告列表成功",
		"action", "list_reports",
		"result", "success",
		"testee_id", dto.TesteeID,
		"total_count", totalInt,
		"page_count", len(reports),
		"duration_ms", duration.Milliseconds(),
	)

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
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("查询高风险报告列表",
		"action", "list_high_risk_reports",
		"page", dto.Page,
		"page_size", dto.PageSize,
	)

	page, pageSize := normalizePagination(dto.Page, dto.PageSize)

	// 使用查询扩展仓储
	queryRepo, ok := s.reportRepo.(domainReport.ReportQueryRepository)
	if !ok {
		l.Errorw("仓储不支持高风险报告查询",
			"action", "list_high_risk_reports",
			"result", "failed",
		)
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "仓储不支持高风险报告查询")
	}

	offset := (page - 1) * pageSize
	l.Debugw("开始查询高风险报告",
		"page", page,
		"page_size", pageSize,
		"offset", offset,
	)

	reports, err := queryRepo.FindHighRiskReports(ctx, offset, pageSize)
	if err != nil {
		l.Errorw("查询高风险报告失败",
			"action", "list_high_risk_reports",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "查询高风险报告失败")
	}

	items := make([]*ReportResult, len(reports))
	for i, r := range reports {
		items[i] = ToReportResult(r)
	}

	// 统计总数
	l.Debugw("统计高风险报告总数",
		"action", "count",
	)

	spec := domainReport.ReportQuerySpec{HighRiskOnly: true}
	total, err := queryRepo.CountBySpec(ctx, spec)
	if err != nil {
		l.Warnw("统计高风险报告总数失败，使用查询结果数作为总数",
			"error", err.Error(),
		)
		total = int64(len(items))
	}

	totalInt := int(total)
	duration := time.Since(startTime)
	l.Debugw("查询高风险报告列表成功",
		"action", "list_high_risk_reports",
		"result", "success",
		"total_count", totalInt,
		"page_count", len(items),
		"duration_ms", duration.Milliseconds(),
	)

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
