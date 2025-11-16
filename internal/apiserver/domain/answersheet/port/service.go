package port

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/dto"
)

// AnswerSheetSaver 答卷保存器
// 专注于答卷的保存操作
type AnswerSheetSaver interface {
	// SaveAnswerSheet 保存答卷（包括新建和更新）
	SaveOriginalAnswerSheet(ctx context.Context, answerSheet dto.AnswerSheetDTO) (*dto.AnswerSheetDTO, error)

	// SaveAnswerSheetScores 保存答卷分数
	SaveAnswerSheetScores(ctx context.Context, id uint64, totalScore float64, answers []dto.AnswerDTO) (*dto.AnswerSheetDTO, error)
}

// AnswerSheetQueryer 答卷查询器
// 专注于答卷的查询操作
type AnswerSheetQueryer interface {
	// GetAnswerSheetByID 根据ID获取答卷详情
	GetAnswerSheetByID(ctx context.Context, id uint64) (*dto.AnswerSheetDetailDTO, error)

	// GetAnswerSheetList 获取答卷列表
	GetAnswerSheetList(ctx context.Context, filter dto.AnswerSheetDTO, page, pageSize int) ([]dto.AnswerSheetDTO, int64, error)
}
