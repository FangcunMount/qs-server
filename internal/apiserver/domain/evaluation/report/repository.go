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

// ==================== 分页参数 ====================

// Pagination 分页参数值对象
type Pagination struct {
	page     int
	pageSize int
}

// NewPagination 创建分页参数
func NewPagination(page, pageSize int) Pagination {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return Pagination{
		page:     page,
		pageSize: pageSize,
	}
}

// DefaultPagination 默认分页参数
func DefaultPagination() Pagination {
	return NewPagination(1, 10)
}

// Page 获取页码
func (p Pagination) Page() int {
	return p.page
}

// PageSize 获取每页数量
func (p Pagination) PageSize() int {
	return p.pageSize
}

// Offset 获取偏移量（用于 SQL OFFSET）
func (p Pagination) Offset() int {
	return (p.page - 1) * p.pageSize
}

// Limit 获取限制数量（用于 SQL LIMIT）
func (p Pagination) Limit() int {
	return p.pageSize
}
