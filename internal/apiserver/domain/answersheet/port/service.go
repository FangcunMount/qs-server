package port

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/answersheet"
)

// AnswerSheetSaver 答卷保存器
type AnswerSheetSaver interface {
	SaveAnswerSheet(ctx context.Context, aDomain *answersheet.AnswerSheet) (*answersheet.AnswerSheet, error)
}

// AnswerSheetQueryer 答卷查询器
type AnswerSheetQueryer interface {
	GetAnswerSheetByID(ctx context.Context, id uint64) (*answersheet.AnswerSheet, error)
	GetAnswerSheetListByWriter(ctx context.Context, writerID uint64, page, pageSize int) ([]*answersheet.AnswerSheet, error)
	GetAnswerSheetListByTestee(ctx context.Context, testeeID uint64, page, pageSize int) ([]*answersheet.AnswerSheet, error)
}
