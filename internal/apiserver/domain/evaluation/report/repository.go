package report

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
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

	// FindByAssessmentID 根据测评ID查找报告
	// 由于 Report.ID == Assessment.ID，此方法等价于 FindByID
	FindByAssessmentID(ctx context.Context, assessmentID AssessmentID) (*InterpretReport, error)

	// FindByTesteeID 查询受试者的报告列表
	FindByTesteeID(ctx context.Context, testeeID testee.ID, pagination Pagination) ([]*InterpretReport, int64, error)

	// Update 更新报告
	Update(ctx context.Context, report *InterpretReport) error

	// Delete 删除报告
	Delete(ctx context.Context, id ID) error

	// ExistsByID 检查报告是否存在
	ExistsByID(ctx context.Context, id ID) (bool, error)
}

// ==================== 查询规格 ====================

// ReportQuerySpec 报告查询规格
type ReportQuerySpec struct {
	// 量表编码过滤
	ScaleCode string

	// 风险等级过滤
	RiskLevel *RiskLevel

	// 仅高风险
	HighRiskOnly bool

	// 分页
	Offset int
	Limit  int
}

// ReportQueryRepository 报告查询仓储接口（扩展查询能力）
type ReportQueryRepository interface {
	ReportRepository

	// FindBySpec 根据规格查询报告
	FindBySpec(ctx context.Context, spec ReportQuerySpec) ([]*InterpretReport, error)

	// CountBySpec 根据规格统计报告数量
	CountBySpec(ctx context.Context, spec ReportQuerySpec) (int64, error)

	// FindHighRiskReports 查找高风险报告
	FindHighRiskReports(ctx context.Context, offset, limit int) ([]*InterpretReport, error)
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
