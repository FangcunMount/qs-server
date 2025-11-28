package assessment

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
)

// ==================== Assessment Repository ====================

// Repository 测评仓储接口（出站端口）
type Repository interface {
	// === 基础 CRUD ===

	// Save 保存测评（新增或更新）
	Save(ctx context.Context, assessment *Assessment) error

	// FindByID 根据ID查找
	FindByID(ctx context.Context, id ID) (*Assessment, error)

	// Delete 删除测评
	Delete(ctx context.Context, id ID) error

	// === 按关联查询 ===

	// FindByAnswerSheetID 根据答卷ID查找
	FindByAnswerSheetID(ctx context.Context, answerSheetID AnswerSheetRef) (*Assessment, error)

	// FindByTesteeID 查询受试者的测评列表（支持分页）
	FindByTesteeID(ctx context.Context, testeeID testee.ID, pagination Pagination) ([]*Assessment, int64, error)

	// FindByTesteeIDAndScaleID 查询受试者在某个量表下的测评列表
	FindByTesteeIDAndScaleID(ctx context.Context, testeeID testee.ID, scaleRef MedicalScaleRef, pagination Pagination) ([]*Assessment, int64, error)

	// === 按业务来源查询 ===

	// FindByPlanID 查询计划下的测评列表
	FindByPlanID(ctx context.Context, planID string, pagination Pagination) ([]*Assessment, int64, error)

	// FindByScreeningProjectID 查询筛查项目下的测评列表
	FindByScreeningProjectID(ctx context.Context, screeningProjectID string, pagination Pagination) ([]*Assessment, int64, error)

	// === 统计查询 ===

	// CountByStatus 按状态统计数量
	CountByStatus(ctx context.Context, status Status) (int64, error)

	// CountByTesteeIDAndStatus 按受试者和状态统计
	CountByTesteeIDAndStatus(ctx context.Context, testeeID testee.ID, status Status) (int64, error)

	// CountByOrgIDAndStatus 按组织和状态统计
	CountByOrgIDAndStatus(ctx context.Context, orgID int64, status Status) (int64, error)

	// === 批量查询 ===

	// FindByIDs 批量查询（根据ID列表）
	FindByIDs(ctx context.Context, ids []ID) ([]*Assessment, error)

	// FindPendingSubmission 查找待提交的测评
	FindPendingSubmission(ctx context.Context, pagination Pagination) ([]*Assessment, int64, error)
}

// ==================== AssessmentScore Repository ====================

// ScoreRepository 测评得分仓储接口
type ScoreRepository interface {
	// === 批量保存 ===

	// SaveScores 批量保存得分
	SaveScores(ctx context.Context, scores []*AssessmentScore) error

	// === 基础查询 ===

	// FindByAssessmentID 查询测评的所有得分
	FindByAssessmentID(ctx context.Context, assessmentID ID) ([]*AssessmentScore, error)

	// === 趋势分析查询 ===

	// FindByTesteeIDAndFactorCode 查询受试者在某个因子上的历史得分（用于趋势分析）
	FindByTesteeIDAndFactorCode(ctx context.Context, testeeID testee.ID, factorCode FactorCode, limit int) ([]*AssessmentScore, error)

	// FindLatestByTesteeIDAndScaleID 查询受试者在某个量表下所有因子的最新得分
	FindLatestByTesteeIDAndScaleID(ctx context.Context, testeeID testee.ID, scaleRef MedicalScaleRef) ([]*AssessmentScore, error)

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
