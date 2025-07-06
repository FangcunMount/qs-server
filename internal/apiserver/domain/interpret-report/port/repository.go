package port

import (
	"context"

	interpretreport "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/interpret-report"
)

// InterpretReportRepositoryMongo 解读报告MongoDB仓储接口
type InterpretReportRepositoryMongo interface {
	// Create 创建解读报告
	Create(ctx context.Context, report *interpretreport.InterpretReport) error
	// FindByAnswerSheetId 根据答卷ID查找解读报告
	FindByAnswerSheetId(ctx context.Context, answerSheetId uint64) (*interpretreport.InterpretReport, error)
	// FindList 根据条件查找解读报告列表
	FindList(ctx context.Context, page, pageSize int, conditions map[string]string) ([]*interpretreport.InterpretReport, error)
	// CountWithConditions 根据条件计算解读报告数量
	CountWithConditions(ctx context.Context, conditions map[string]string) (int64, error)
	// Update 更新解读报告
	Update(ctx context.Context, report *interpretreport.InterpretReport) error
	// ExistsByAnswerSheetId 检查答卷ID对应的解读报告是否存在
	ExistsByAnswerSheetId(ctx context.Context, answerSheetId uint64) (bool, error)
}
