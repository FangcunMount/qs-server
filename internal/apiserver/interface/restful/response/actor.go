package response

import "time"

// TesteeResponse 受试者响应
type TesteeResponse struct {
	ID              uint64                   `json:"id"`                         // ID
	OrgID           int64                    `json:"org_id"`                     // 机构ID
	ProfileID       *uint64                  `json:"profile_id,omitempty"`       // 用户档案ID（新字段）
	IAMChildID      *uint64                  `json:"iam_child_id,omitempty"`     // IAM儿童ID（已废弃，向后兼容，等同于ProfileID）
	Name            string                   `json:"name"`                       // 姓名
	Gender          string                   `json:"gender,omitempty"`           // 性别
	Birthday        *time.Time               `json:"birthday,omitempty"`         // 出生日期
	Tags            []string                 `json:"tags"`                       // 标签
	Source          string                   `json:"source,omitempty"`           // 来源
	IsKeyFocus      bool                     `json:"is_key_focus"`               // 是否重点关注
	AssessmentStats *AssessmentStatsResponse `json:"assessment_stats,omitempty"` // 测评统计
	Guardians       []GuardianResponse       `json:"guardians,omitempty"`        // 监护人信息列表
	CreatedAt       time.Time                `json:"created_at"`                 // 创建时间
	UpdatedAt       time.Time                `json:"updated_at"`                 // 更新时间
}

// GuardianResponse 监护人信息响应
type GuardianResponse struct {
	Name     string `json:"name"`
	Relation string `json:"relation"`
	Phone    string `json:"phone"`
}

// AssessmentStatsResponse 测评统计响应
type AssessmentStatsResponse struct {
	TotalCount       int        `json:"total_count"`                  // 总次数
	LastAssessmentAt *time.Time `json:"last_assessment_at,omitempty"` // 最后测评时间
	LastRiskLevel    string     `json:"last_risk_level,omitempty"`    // 最后风险等级
}

// TesteeListResponse 受试者列表响应
type TesteeListResponse struct {
	Items      []*TesteeResponse `json:"items"`       // 列表数据
	Total      int64             `json:"total"`       // 总数
	Page       int               `json:"page"`        // 当前页码
	PageSize   int               `json:"page_size"`   // 每页数量
	TotalPages int               `json:"total_pages"` // 总页数
}

// StaffResponse 员工响应
type StaffResponse struct {
	ID        uint64    `json:"id"`              // ID
	OrgID     int64     `json:"org_id"`          // 机构ID
	UserID    int64     `json:"user_id"`         // 用户ID
	Roles     []string  `json:"roles"`           // 角色列表
	Name      string    `json:"name"`            // 姓名
	Email     string    `json:"email,omitempty"` // 邮箱
	Phone     string    `json:"phone,omitempty"` // 电话
	IsActive  bool      `json:"is_active"`       // 是否激活
	CreatedAt time.Time `json:"created_at"`      // 创建时间
	UpdatedAt time.Time `json:"updated_at"`      // 更新时间
}

// StaffListResponse 员工列表响应
type StaffListResponse struct {
	Items      []*StaffResponse `json:"items"`       // 列表数据
	Total      int64            `json:"total"`       // 总数
	Page       int              `json:"page"`        // 当前页码
	PageSize   int              `json:"page_size"`   // 每页数量
	TotalPages int              `json:"total_pages"` // 总页数
}
