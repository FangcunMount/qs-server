package assessment

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// reportQueryService 报告查询服务实现
// 行为者：报告查询者（答题者或管理员）
type reportQueryService struct {
	reportRepo report.ReportRepository
}

// NewReportQueryService 创建报告查询服务
func NewReportQueryService(reportRepo report.ReportRepository) ReportQueryService {
	return &reportQueryService{
		reportRepo: reportRepo,
	}
}

// GetByAssessmentID 根据测评ID获取报告
func (s *reportQueryService) GetByAssessmentID(ctx context.Context, assessmentID uint64) (*ReportResult, error) {
	assessmentIDVO := meta.FromUint64(assessmentID)
	rpt, err := s.reportRepo.FindByAssessmentID(ctx, assessmentIDVO)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInterpretReportNotFound, "报告不存在")
	}

	return toReportResult(rpt), nil
}

// ListByTesteeID 获取受试者的报告列表
func (s *reportQueryService) ListByTesteeID(ctx context.Context, dto ListReportsDTO) (*ReportListResult, error) {
	if dto.TesteeID == 0 {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "受试者ID不能为空")
	}

	page, pageSize := normalizePagination(dto.Page, dto.PageSize)
	testeeID := testee.NewID(dto.TesteeID)
	pagination := report.NewPagination(page, pageSize)

	reports, total, err := s.reportRepo.FindByTesteeID(ctx, testeeID, pagination)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "查询报告列表失败")
	}

	items := make([]*ReportResult, len(reports))
	for i, r := range reports {
		items[i] = toReportResult(r)
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

// ExportPDF 导出PDF报告
func (s *reportQueryService) ExportPDF(ctx context.Context, assessmentID uint64) ([]byte, error) {
	// TODO: 实现PDF导出功能
	// 可以使用第三方库如 go-wkhtmltopdf 或 gofpdf
	return nil, errors.WithCode(errorCode.ErrInvalidArgument, "PDF导出功能暂未实现")
}
