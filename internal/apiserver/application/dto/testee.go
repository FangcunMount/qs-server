package dto

import "time"

// TesteeDTO 受试者数据传输对象
type TesteeDTO struct {
	ID         uint64     `json:"id"`
	OrgID      int64      `json:"org_id"`
	IAMUserID  *int64     `json:"iam_user_id,omitempty"`
	IAMChildID *int64     `json:"iam_child_id,omitempty"`
	Name       string     `json:"name"`
	Gender     int8       `json:"gender"`
	Birthday   *time.Time `json:"birthday,omitempty"`
	Age        int        `json:"age"`
	Grade      *string    `json:"grade,omitempty"`
	ClassName  *string    `json:"class_name,omitempty"`
	SchoolName *string    `json:"school_name,omitempty"`
	Tags       []string   `json:"tags,omitempty"`
	Source     string     `json:"source"`
	IsKeyFocus bool       `json:"is_key_focus"`

	// 统计信息
	LastAssessmentAt *time.Time `json:"last_assessment_at,omitempty"`
	TotalAssessments int        `json:"total_assessments"`
	LastRiskLevel    *string    `json:"last_risk_level,omitempty"`

	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

// CreateTesteeRequest 创建受试者请求
type CreateTesteeRequest struct {
	OrgID      int64      `json:"org_id" binding:"required"`
	IAMUserID  *int64     `json:"iam_user_id,omitempty"`
	IAMChildID *int64     `json:"iam_child_id,omitempty"`
	Name       string     `json:"name" binding:"required"`
	Gender     int8       `json:"gender"`
	Birthday   *time.Time `json:"birthday,omitempty"`
	Grade      *string    `json:"grade,omitempty"`
	ClassName  *string    `json:"class_name,omitempty"`
	SchoolName *string    `json:"school_name,omitempty"`
	Source     string     `json:"source"`
}

// UpdateTesteeRequest 更新受试者请求
type UpdateTesteeRequest struct {
	Name       string     `json:"name,omitempty"`
	Gender     int8       `json:"gender"`
	Birthday   *time.Time `json:"birthday,omitempty"`
	Grade      *string    `json:"grade,omitempty"`
	ClassName  *string    `json:"class_name,omitempty"`
	SchoolName *string    `json:"school_name,omitempty"`
}

// TesteeListRequest 受试者列表请求
type TesteeListRequest struct {
	OrgID    int64    `form:"org_id" binding:"required"`
	Name     string   `form:"name"`
	Tags     []string `form:"tags"`
	KeyFocus *bool    `form:"key_focus"`
	Offset   int      `form:"offset"`
	Limit    int      `form:"limit" binding:"required,min=1,max=100"`
}

// StaffDTO 员工数据传输对象
type StaffDTO struct {
	ID        uint64   `json:"id"`
	OrgID     int64    `json:"org_id"`
	IAMUserID int64    `json:"iam_user_id"`
	Roles     []string `json:"roles"`
	Name      string   `json:"name"`
	Email     string   `json:"email"`
	Phone     string   `json:"phone"`
	IsActive  bool     `json:"is_active"`
}

// CreateStaffRequest 创建员工请求
type CreateStaffRequest struct {
	OrgID     int64    `json:"org_id" binding:"required"`
	IAMUserID int64    `json:"iam_user_id" binding:"required"`
	Name      string   `json:"name" binding:"required"`
	Email     string   `json:"email"`
	Phone     string   `json:"phone"`
	Roles     []string `json:"roles"`
}

// UpdateStaffRequest 更新员工请求
type UpdateStaffRequest struct {
	Email string   `json:"email,omitempty"`
	Phone string   `json:"phone,omitempty"`
	Roles []string `json:"roles,omitempty"`
}

// FillerDTO 填写人数据传输对象
type FillerDTO struct {
	UserID     int64  `json:"user_id"`
	FillerType string `json:"filler_type"`
}

// TesteeRefDTO 受试者引用数据传输对象
type TesteeRefDTO struct {
	TesteeID   uint64 `json:"testee_id"`
	IAMUserID  *int64 `json:"iam_user_id,omitempty"`
	IAMChildID *int64 `json:"iam_child_id,omitempty"`
}
