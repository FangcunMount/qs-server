package request

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// CreateTesteeRequest 创建受试者请求
type CreateTesteeRequest struct {
	OrgID      int64      `json:"org_id" binding:"required"` // 机构ID
	ProfileID  *uint64    `json:"profile_id"`                // 用户档案ID（新字段）
	IAMChildID *int64     `json:"iam_child_id"`              // IAM儿童ID（已废弃，向后兼容）
	Name       string     `json:"name" binding:"required"`   // 姓名
	Gender     string     `json:"gender"`                    // 性别
	Birthday   *time.Time `json:"birthday"`                  // 出生日期
	Tags       []string   `json:"tags"`                      // 标签
	Source     string     `json:"source"`                    // 来源
	IsKeyFocus bool       `json:"is_key_focus"`              // 是否重点关注
}

// UpdateTesteeRequest 更新受试者请求
type UpdateTesteeRequest struct {
	Name       *string    `json:"name"`         // 姓名
	Gender     *string    `json:"gender"`       // 性别
	Birthday   *time.Time `json:"birthday"`     // 出生日期
	Tags       []string   `json:"tags"`         // Deprecated: 当前更新接口不消费；标签仅作为辅助标签，不是风险/队列事实
	IsKeyFocus *bool      `json:"is_key_focus"` // 是否重点关注
}

// ListTesteeRequest 查询受试者列表请求
type ListTesteeRequest struct {
	OrgID            int64    `form:"org_id"`                                      // 兼容字段：机构ID
	Name             string   `form:"name"`                                        // 姓名（模糊匹配）
	Tags             []string `form:"tags"`                                        // 标签筛选
	IsKeyFocus       *bool    `form:"is_key_focus"`                                // 是否重点关注
	ProfileID        string   `form:"profile_id"`                                  // 档案ID（ProfileID）
	ClinicianID      *uint64  `form:"clinician_id"`                                // Clinician 过滤
	CreatedStartDate string   `form:"created_start_date"`                          // 报到开始日期（YYYY-MM-DD）
	CreatedEndDate   string   `form:"created_end_date"`                            // 报到结束日期（YYYY-MM-DD）
	Page             int      `form:"page" binding:"omitempty,min=1"`              // 页码
	PageSize         int      `form:"page_size" binding:"omitempty,min=1,max=100"` // 每页数量
}

// GetTesteeByProfileIDRequest 根据 profile_id 查询受试者请求
type GetTesteeByProfileIDRequest struct {
	OrgID      int64  `form:"org_id"`       // 兼容字段：机构ID
	ProfileID  string `form:"profile_id"`   // 用户档案ID（ProfileID）
	IAMChildID string `form:"iam_child_id"` // 兼容字段：IAM儿童ID
}

// CreateStaffRequest 创建员工请求
type CreateStaffRequest struct {
	OrgID    int64    `json:"org_id"`                          // 兼容字段：机构ID
	UserID   meta.ID  `json:"user_id,omitempty"`               // IAM用户ID（优先使用）
	Roles    []string `json:"roles"`                           // 期望授予的角色列表（IAM 启用时转成 assignment）
	Name     string   `json:"name" binding:"required"`         // 姓名
	Email    string   `json:"email" binding:"omitempty,email"` // 邮箱
	Phone    string   `json:"phone"`                           // 电话
	Password string   `json:"password"`                        // 新建 IAM 运营账号时的初始密码
	IsActive *bool    `json:"is_active"`                       // 是否激活
}

// UpdateStaffRequest 更新员工请求
type UpdateStaffRequest struct {
	Roles    []string `json:"roles"`                           // 角色列表
	Name     *string  `json:"name"`                            // 姓名
	Email    *string  `json:"email" binding:"omitempty,email"` // 邮箱
	Phone    *string  `json:"phone"`                           // 电话
	IsActive *bool    `json:"is_active"`                       // 是否激活
}

// ListStaffRequest 查询员工列表请求
type ListStaffRequest struct {
	OrgID    int64  `form:"org_id"`                                      // 兼容字段：机构ID
	Role     string `form:"role"`                                        // 角色筛选
	Page     int    `form:"page" binding:"omitempty,min=1"`              // 页码
	PageSize int    `form:"page_size" binding:"omitempty,min=1,max=100"` // 每页数量
}
