package answersheet

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/answersheet"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/answersheet/port"
)

// Queryer 答卷查询器
type Queryer struct {
	aRepoMongo port.AnswerSheetRepositoryMongo
}

// NewQueryer 创建答卷查询器
func NewQueryer(aRepoMongo port.AnswerSheetRepositoryMongo) *Queryer {
	return &Queryer{aRepoMongo: aRepoMongo}
}

// GetAnswerSheetByID 根据ID获取答卷
func (q *Queryer) GetAnswerSheetByID(ctx context.Context, id uint64) (*answersheet.AnswerSheet, error) {
	return q.aRepoMongo.FindByID(ctx, id)
}

// GetAnswerSheetListByWriter 根据答卷者获取答卷列表
func (q *Queryer) GetAnswerSheetListByWriter(ctx context.Context, writerID uint64, page, pageSize int) ([]*answersheet.AnswerSheet, error) {
	return q.aRepoMongo.FindListByWriter(ctx, writerID, page, pageSize)
}

// GetAnswerSheetListByTestee 根据被试者获取答卷列表
func (q *Queryer) GetAnswerSheetListByTestee(ctx context.Context, testeeID uint64, page, pageSize int) ([]*answersheet.AnswerSheet, error) {
	return q.aRepoMongo.FindListByTestee(ctx, testeeID, page, pageSize)
}

// GetAnswerSheetListWithPagination 根据条件获取答卷列表（带分页）
func (q *Queryer) GetAnswerSheetListWithPagination(ctx context.Context, page, pageSize int, conditions map[string]interface{}) ([]*answersheet.AnswerSheet, int64, error) {
	// 获取答卷列表 - 这里需要根据具体的查询条件来决定使用哪个方法
	// 由于当前接口限制，我们使用 CountWithConditions 来获取总数
	total, err := q.aRepoMongo.CountWithConditions(ctx, conditions)
	if err != nil {
		return nil, 0, err
	}

	// 这里暂时返回空列表，实际应该根据条件查询
	// 可以考虑在未来扩展 repository 接口来支持通用条件查询
	return []*answersheet.AnswerSheet{}, total, nil
}

// CountAnswerSheetsByWriter 统计答卷者的答卷数量
func (q *Queryer) CountAnswerSheetsByWriter(ctx context.Context, writerID uint64) (int64, error) {
	conditions := map[string]interface{}{
		"writer.id": writerID,
	}
	return q.aRepoMongo.CountWithConditions(ctx, conditions)
}

// CountAnswerSheetsByTestee 统计被试者的答卷数量
func (q *Queryer) CountAnswerSheetsByTestee(ctx context.Context, testeeID uint64) (int64, error) {
	conditions := map[string]interface{}{
		"testee.id": testeeID,
	}
	return q.aRepoMongo.CountWithConditions(ctx, conditions)
}

// CountAnswerSheetsByQuestionnaireCode 统计指定问卷的答卷数量
func (q *Queryer) CountAnswerSheetsByQuestionnaireCode(ctx context.Context, questionnaireCode string) (int64, error) {
	conditions := map[string]interface{}{
		"questionnaire_code": questionnaireCode,
	}
	return q.aRepoMongo.CountWithConditions(ctx, conditions)
}
