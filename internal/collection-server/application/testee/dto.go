package testee

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// CreateTesteeRequest 创建受试者请求
// 注意：机构ID由系统自动设置，不需要用户传入
type CreateTesteeRequest struct {
	IAMUserID  uint64         `json:"iam_user_id"`                     // IAM用户ID（成人）
	IAMChildID uint64         `json:"iam_child_id" binding:"required"` // IAM儿童ID
	Name       string         `json:"name" binding:"required"`         // 姓名
	Gender     int32          `json:"gender" binding:"required"`       // 性别：1-男，2-女，3-其他
	Birthday   *meta.Birthday `json:"birthday"`                        // 出生日期（格式：YYYY-MM-DD）
	Tags       []string       `json:"tags"`                            // 标签列表
	Source     string         `json:"source"`                          // 来源：online_form/plan/screening/imported
	IsKeyFocus bool           `json:"is_key_focus"`                    // 是否重点关注
}

// UpdateTesteeRequest 更新受试者请求
type UpdateTesteeRequest struct {
	Name       string         `json:"name"`         // 姓名
	Gender     int32          `json:"gender"`       // 性别
	Birthday   *meta.Birthday `json:"birthday"`     // 出生日期（格式：YYYY-MM-DD）
	Tags       []string       `json:"tags"`         // 标签列表
	IsKeyFocus bool           `json:"is_key_focus"` // 是否重点关注
}

// TesteeResponse 受试者响应
type TesteeResponse struct {
	ID         uint64        `json:"id"`           // 受试者ID
	OrgID      uint64        `json:"org_id"`       // 机构ID
	IAMUserID  uint64        `json:"iam_user_id"`  // IAM用户ID
	IAMChildID uint64        `json:"iam_child_id"` // IAM儿童ID
	Name       string        `json:"name"`         // 姓名
	Gender     int32         `json:"gender"`       // 性别
	Birthday   meta.Birthday `json:"birthday"`     // 出生日期（格式：YYYY-MM-DD）
	Tags       []string      `json:"tags"`         // 标签列表
	Source     string        `json:"source"`       // 来源
	IsKeyFocus bool          `json:"is_key_focus"` // 是否重点关注

	// 测评统计信息
	AssessmentStats *AssessmentStatsDTO `json:"assessment_stats,omitempty"`

	CreatedAt time.Time `json:"created_at"` // 创建时间
	UpdatedAt time.Time `json:"updated_at"` // 更新时间
}

// AssessmentStatsDTO 测评统计信息
type AssessmentStatsDTO struct {
	TotalCount       int32     `json:"total_count"`        // 总测评次数
	LastAssessmentAt time.Time `json:"last_assessment_at"` // 最后测评时间
	LastRiskLevel    string    `json:"last_risk_level"`    // 最后风险等级
}

// ListTesteesRequest 查询受试者列表请求
// 注意：collection-server 查询当前用户（监护人）的受试者列表，不需要传入机构ID
type ListTesteesRequest struct {
	Offset int32 `form:"offset"` // 偏移量
	Limit  int32 `form:"limit"`  // 每页数量
}

// ListTesteesResponse 查询受试者列表响应
type ListTesteesResponse struct {
	Items []*TesteeResponse `json:"items"` // 受试者列表
	Total int64             `json:"total"` // 总数
}

// TesteeExistsResponse 受试者是否存在响应
type TesteeExistsResponse struct {
	Exists   bool   `json:"exists"`    // 是否存在
	TesteeID uint64 `json:"testee_id"` // 受试者ID（如果存在）
}
