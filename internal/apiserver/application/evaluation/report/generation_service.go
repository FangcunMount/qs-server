package report

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// reportGenerationService 报告生成服务实现
type reportGenerationService struct {
	reportRepo domainReport.ReportRepository
}

// NewReportGenerationService 创建报告生成服务
func NewReportGenerationService(reportRepo domainReport.ReportRepository) ReportGenerationService {
	return &reportGenerationService{
		reportRepo: reportRepo,
	}
}

// GenerateFromEvaluation 根据评估结果生成报告
func (s *reportGenerationService) GenerateFromEvaluation(ctx context.Context, dto GenerateReportDTO) (*ReportResult, error) {
	if dto.AssessmentID == 0 {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "测评ID不能为空")
	}

	// 转换维度
	dimensions := make([]domainReport.DimensionInterpret, len(dto.Dimensions))
	for i, d := range dto.Dimensions {
		dimensions[i] = domainReport.NewDimensionInterpret(
			domainReport.FactorCode(d.FactorCode),
			d.FactorName,
			d.RawScore,
			d.MaxScore,
			domainReport.RiskLevel(d.RiskLevel),
			d.Description,
		)
	}

	// 创建报告
	reportID := meta.FromUint64(dto.AssessmentID)
	report := domainReport.NewInterpretReport(
		reportID,
		dto.ScaleName,
		dto.ScaleCode,
		dto.TotalScore,
		domainReport.RiskLevel(dto.RiskLevel),
		dto.Conclusion,
		dimensions,
		dto.Suggestions,
	)

	// 保存报告
	if err := s.reportRepo.Save(ctx, report); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存报告失败")
	}

	return ToReportResult(report), nil
}
