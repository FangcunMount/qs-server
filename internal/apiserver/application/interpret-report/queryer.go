package interpretreport

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/dto"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/mapper"
	interpretport "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/interpret-report/port"
	errCode "github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
)

// Queryer 解读报告查询器
type Queryer struct {
	repo   interpretport.InterpretReportRepositoryMongo
	mapper *mapper.InterpretReportMapper
}

// NewQueryer 创建解读报告查询器
func NewQueryer(repo interpretport.InterpretReportRepositoryMongo) *Queryer {
	return &Queryer{
		repo:   repo,
		mapper: mapper.NewInterpretReportMapper(),
	}
}

// 确保实现了接口
var _ interpretport.InterpretReportQueryer = (*Queryer)(nil)

// GetInterpretReportByAnswerSheetId 根据答卷ID获取解读报告
func (q *Queryer) GetInterpretReportByAnswerSheetId(ctx context.Context, answerSheetId uint64) (*dto.InterpretReportDTO, error) {
	// 验证参数
	if err := q.validateAnswerSheetId(answerSheetId); err != nil {
		return nil, err
	}

	// 查询解读报告
	report, err := q.repo.FindByAnswerSheetId(ctx, answerSheetId)
	if err != nil {
		return nil, errors.WithCode(errCode.ErrInterpretReportNotFound, "解读报告不存在: %v", err)
	}

	// 转换为DTO
	return q.mapper.ToDTO(report), nil
}

// validateAnswerSheetId 验证答卷ID
func (q *Queryer) validateAnswerSheetId(answerSheetId uint64) error {
	if answerSheetId == 0 {
		return errors.WithCode(errCode.ErrInvalidArgument, "答卷ID不能为空")
	}
	return nil
}
