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
	page, pageSize := normalizePagination(dto.Page, dto.PageSize)
	pagination := report.NewPagination(page, pageSize)

	var (
		reports []*report.InterpretReport
		total   int64
		err     error
	)
	switch {
	case dto.TesteeID != 0:
		reports, total, err = s.reportRepo.FindByTesteeID(ctx, testee.NewID(dto.TesteeID), pagination)
	case dto.RestrictToAccessScope:
		testeeIDs := make([]testee.ID, 0, len(dto.AccessibleTesteeIDs))
		for _, rawID := range dto.AccessibleTesteeIDs {
			testeeIDs = append(testeeIDs, testee.NewID(rawID))
		}
		reports, total, err = s.reportRepo.FindByTesteeIDs(ctx, testeeIDs, pagination)
	default:
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "受试者ID不能为空")
	}
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
func (s *reportQueryService) ExportPDF(_ context.Context, _ uint64) ([]byte, error) {
	return nil, errors.WithCode(errorCode.ErrUnsupportedOperation, "PDF导出当前不支持")
}
