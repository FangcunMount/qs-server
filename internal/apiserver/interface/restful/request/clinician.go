package request

import "time"

// CreateClinicianRequest 创建从业者请求。
type CreateClinicianRequest struct {
	OrgID         int64   `json:"org_id"`
	OperatorID    *uint64 `json:"operator_id"`
	Name          string  `json:"name" binding:"required"`
	Department    string  `json:"department"`
	Title         string  `json:"title"`
	ClinicianType string  `json:"clinician_type" binding:"required"`
	EmployeeCode  string  `json:"employee_code"`
	IsActive      bool    `json:"is_active"`
}

// ListClinicianRequest 从业者列表请求。
type ListClinicianRequest struct {
	OrgID    int64 `form:"org_id"`
	Page     int   `form:"page" binding:"min=1"`
	PageSize int   `form:"page_size" binding:"min=1,max=100"`
}

// CreateAssessmentEntryRequest 创建测评入口请求。
type CreateAssessmentEntryRequest struct {
	TargetType    string     `json:"target_type" binding:"required"`
	TargetCode    string     `json:"target_code" binding:"required"`
	TargetVersion string     `json:"target_version"`
	ExpiresAt     *time.Time `json:"expires_at"`
}

// ListAssessmentEntryRequest 测评入口列表请求。
type ListAssessmentEntryRequest struct {
	Page     int `form:"page" binding:"min=1"`
	PageSize int `form:"page_size" binding:"min=1,max=100"`
}

// IntakeByAssessmentEntryRequest 扫码 intake 请求。
type IntakeByAssessmentEntryRequest struct {
	ProfileID *uint64    `json:"profile_id"`
	Name      string     `json:"name" binding:"required"`
	Gender    string     `json:"gender"`
	Birthday  *time.Time `json:"birthday"`
}
