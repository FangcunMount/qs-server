package report

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// suggestionService 建议服务实现
type suggestionService struct {
	reportRepo          domainReport.ReportRepository
	suggestionGenerator domainReport.SuggestionGenerator
}

// NewSuggestionService 创建建议服务
func NewSuggestionService(
	reportRepo domainReport.ReportRepository,
	suggestionGenerator domainReport.SuggestionGenerator,
) SuggestionService {
	return &suggestionService{
		reportRepo:          reportRepo,
		suggestionGenerator: suggestionGenerator,
	}
}

// GenerateSuggestions 生成建议
func (s *suggestionService) GenerateSuggestions(ctx context.Context, reportID uint64) ([]SuggestionDTO, error) {
	id := meta.FromUint64(reportID)
	report, err := s.reportRepo.FindByID(ctx, id)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInterpretReportNotFound, "报告不存在")
	}

	// 生成建议
	suggestions, err := s.suggestionGenerator.Generate(ctx, report)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInterpretReportGenerationFailed, "生成建议失败")
	}

	// 更新报告
	report.UpdateSuggestions(suggestions)
	if err := s.reportRepo.Update(ctx, report); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "更新报告失败")
	}

	// 转换为 DTO
	return toSuggestionDTOs(suggestions), nil
}

// UpdateSuggestions 更新报告的建议
func (s *suggestionService) UpdateSuggestions(ctx context.Context, reportID uint64, suggestions []SuggestionDTO) error {
	id := meta.FromUint64(reportID)
	report, err := s.reportRepo.FindByID(ctx, id)
	if err != nil {
		return errors.WrapC(err, errorCode.ErrInterpretReportNotFound, "报告不存在")
	}

	// 转换 DTO 为领域模型
	domainSuggestions := fromSuggestionDTOs(suggestions)

	// 更新建议
	report.UpdateSuggestions(domainSuggestions)
	if err := s.reportRepo.Update(ctx, report); err != nil {
		return errors.WrapC(err, errorCode.ErrDatabase, "更新报告失败")
	}

	return nil
}

// fromSuggestionDTOs 将 DTO 转换为领域模型
func fromSuggestionDTOs(dtos []SuggestionDTO) []domainReport.Suggestion {
	if len(dtos) == 0 {
		return nil
	}
	result := make([]domainReport.Suggestion, len(dtos))
	for i, dto := range dtos {
		var fc *domainReport.FactorCode
		if dto.FactorCode != nil {
			code := domainReport.NewFactorCode(*dto.FactorCode)
			fc = &code
		}
		result[i] = domainReport.Suggestion{
			Category:   domainReport.SuggestionCategory(dto.Category),
			Content:    dto.Content,
			FactorCode: fc,
		}
	}
	return result
}
