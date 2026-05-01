package report

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

// reportQueryService 报告查询服务实现
type reportQueryService struct {
	reportRepo domainReport.ReportRepository
	reader     evaluationreadmodel.ReportReader
}

// NewReportQueryService 创建报告查询服务
func NewReportQueryService(reportRepo domainReport.ReportRepository) ReportQueryService {
	return &reportQueryService{
		reportRepo: reportRepo,
	}
}

func NewReportQueryServiceWithReadModel(reportRepo domainReport.ReportRepository, reader evaluationreadmodel.ReportReader) ReportQueryService {
	return &reportQueryService{
		reportRepo: reportRepo,
		reader:     reader,
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

	result, err := s.getReportByID(ctx, reportID)
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

	return result, nil
}

func (s *reportQueryService) getReportByID(ctx context.Context, reportID uint64) (*ReportResult, error) {
	if s.reader != nil {
		row, err := s.reader.GetReportByID(ctx, reportID)
		if err != nil {
			return nil, err
		}
		return reportRowToResult(*row), nil
	}
	if s.reportRepo == nil {
		return nil, errors.WithCode(errorCode.ErrModuleInitializationFailed, "report repository is not configured")
	}
	report, err := s.reportRepo.FindByID(ctx, meta.FromUint64(reportID))
	if err != nil {
		return nil, err
	}
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

	if s.reader == nil {
		return nil, errors.WithCode(errorCode.ErrModuleInitializationFailed, "report read model is not configured")
	}
	row, err := s.reader.GetReportByAssessmentID(ctx, assessmentID)
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

	return reportRowToResult(*row), nil
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
	if s.reader == nil {
		return nil, errors.WithCode(errorCode.ErrModuleInitializationFailed, "report read model is not configured")
	}

	page, pageSize := normalizePagination(dto.Page, dto.PageSize)
	l.Debugw("开始查询报告列表",
		"testee_id", dto.TesteeID,
		"page", page,
		"page_size", pageSize,
	)

	filter := evaluationreadmodel.ReportFilter{TesteeID: &dto.TesteeID}
	rows, total, err := s.reader.ListReports(ctx, filter, evaluationreadmodel.PageRequest{Page: page, PageSize: pageSize})
	if err != nil {
		l.Errorw("查询报告列表失败",
			"testee_id", dto.TesteeID,
			"action", "list_reports",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "查询报告列表失败")
	}

	items := reportRowsToResults(rows)
	totalInt, err := safeconv.Int64ToInt(total)
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrDatabase, "报告总数超出安全范围")
	}
	duration := time.Since(startTime)
	l.Debugw("查询受试者报告列表成功",
		"action", "list_reports",
		"result", "success",
		"testee_id", dto.TesteeID,
		"total_count", totalInt,
		"page_count", len(rows),
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

	if s.reader == nil {
		return nil, errors.WithCode(errorCode.ErrModuleInitializationFailed, "report read model is not configured")
	}
	page, pageSize := normalizePagination(dto.Page, dto.PageSize)

	offset := (page - 1) * pageSize
	l.Debugw("开始查询高风险报告",
		"page", page,
		"page_size", pageSize,
		"offset", offset,
	)

	rows, total, err := s.reader.ListReports(ctx, evaluationreadmodel.ReportFilter{HighRiskOnly: true}, evaluationreadmodel.PageRequest{Page: page, PageSize: pageSize})
	if err != nil {
		l.Errorw("查询高风险报告失败",
			"action", "list_high_risk_reports",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "查询高风险报告失败")
	}

	items := reportRowsToResults(rows)
	totalInt, err := safeconv.Int64ToInt(total)
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrDatabase, "高风险报告总数超出安全范围")
	}
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

func reportRowsToResults(rows []evaluationreadmodel.ReportRow) []*ReportResult {
	items := make([]*ReportResult, len(rows))
	for i, row := range rows {
		items[i] = reportRowToResult(row)
	}
	return items
}

func reportRowToResult(row evaluationreadmodel.ReportRow) *ReportResult {
	dimensions := make([]DimensionResult, len(row.Dimensions))
	for i, d := range row.Dimensions {
		dimensions[i] = DimensionResult{
			FactorCode:  d.FactorCode,
			FactorName:  d.FactorName,
			RawScore:    d.RawScore,
			MaxScore:    d.MaxScore,
			RiskLevel:   d.RiskLevel,
			Description: d.Description,
			Suggestion:  d.Suggestion,
		}
	}

	suggestions := make([]SuggestionDTO, len(row.Suggestions))
	for i, s := range row.Suggestions {
		suggestions[i] = SuggestionDTO{
			Category:   s.Category,
			Content:    s.Content,
			FactorCode: s.FactorCode,
		}
	}

	return &ReportResult{
		ID:          row.AssessmentID,
		ScaleName:   row.ScaleName,
		ScaleCode:   row.ScaleCode,
		TotalScore:  row.TotalScore,
		RiskLevel:   row.RiskLevel,
		Conclusion:  row.Conclusion,
		Dimensions:  dimensions,
		Suggestions: suggestions,
		CreatedAt:   row.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}
