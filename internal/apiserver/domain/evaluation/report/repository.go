package report

import (
	"context"
)

// ==================== ReportRepository 仓储接口 ====================

// ReportRepository 报告仓储接口
// 职责：解读报告的持久化操作
// 存储：MongoDB（灵活的文档结构，适合存储结构化报告）
type ReportRepository interface {
	// Save 保存报告
	Save(ctx context.Context, report *InterpretReport) error

	// FindByID 根据ID查找报告
	FindByID(ctx context.Context, id ID) (*InterpretReport, error)

	// Update 更新报告
	Update(ctx context.Context, report *InterpretReport) error

	// Delete 删除报告
	Delete(ctx context.Context, id ID) error

	// ExistsByID 检查报告是否存在
	ExistsByID(ctx context.Context, id ID) (bool, error)
}
