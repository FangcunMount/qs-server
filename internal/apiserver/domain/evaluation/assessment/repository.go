package assessment

import (
	"context"

	"github.com/FangcunMount/qs-server/pkg/event"
)

// ==================== Assessment Repository ====================

// Repository 测评仓储接口（出站端口）
type Repository interface {
	// === 基础 CRUD ===

	// Save 保存测评（新增或更新）
	Save(ctx context.Context, assessment *Assessment) error

	// SaveWithEvents 保存测评并将聚合上的领域事件暂存到可靠出站存储。
	// Deprecated: 新应用用例应使用应用层 UoW + outbox stager 组合显式表达事务边界。
	SaveWithEvents(ctx context.Context, assessment *Assessment) error

	// SaveWithAdditionalEvents 保存测评，并在同一事务里额外暂存补充事件。
	// Deprecated: 新应用用例应使用应用层 UoW + outbox stager 组合显式表达事务边界。
	SaveWithAdditionalEvents(ctx context.Context, assessment *Assessment, additional []event.DomainEvent) error

	// FindByID 根据ID查找
	FindByID(ctx context.Context, id ID) (*Assessment, error)

	// Delete 删除测评
	Delete(ctx context.Context, id ID) error

	// === 按关联查询 ===

	// FindByAnswerSheetID 根据答卷ID查找
	FindByAnswerSheetID(ctx context.Context, answerSheetID AnswerSheetRef) (*Assessment, error)
}

// ==================== AssessmentScore Repository ====================

// ScoreRepository 测评得分仓储接口
type ScoreRepository interface {
	// === 批量保存 ===

	// SaveScores 批量保存得分（已废弃，使用 SaveScoresWithContext）
	SaveScores(ctx context.Context, scores []*AssessmentScore) error

	// SaveScoresWithContext 带上下文保存得分（包含受试者和量表信息）
	// 需要传入 Assessment 对象来获取必要的辅助信息（testeeID, scaleID 等）
	SaveScoresWithContext(ctx context.Context, assessmentDomain *Assessment, score *AssessmentScore) error

	// === 基础查询 ===

	// === 删除 ===

	// DeleteByAssessmentID 删除测评的所有得分
	DeleteByAssessmentID(ctx context.Context, assessmentID ID) error
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
