package request

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type FlexibleTime struct {
	time.Time
}

func (t *FlexibleTime) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}

	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}

	var parsed time.Time
	var err error
	for _, layout := range layouts {
		parsed, err = time.Parse(layout, raw)
		if err == nil {
			t.Time = parsed
			return nil
		}
	}

	return err
}

// CreateClinicianRequest 创建从业者请求。
type CreateClinicianRequest struct {
	OrgID         int64    `json:"org_id"`
	OperatorID    *meta.ID `json:"operator_id"`
	Name          string   `json:"name" binding:"required"`
	Department    string   `json:"department"`
	Title         string   `json:"title"`
	ClinicianType string   `json:"clinician_type" binding:"required"`
	EmployeeCode  string   `json:"employee_code"`
	IsActive      bool     `json:"is_active"`
}

// UpdateClinicianRequest 更新从业者请求。
type UpdateClinicianRequest struct {
	Name          string `json:"name" binding:"required"`
	Department    string `json:"department"`
	Title         string `json:"title"`
	ClinicianType string `json:"clinician_type" binding:"required"`
	EmployeeCode  string `json:"employee_code"`
}

// BindClinicianOperatorRequest 绑定从业者与后台操作者。
type BindClinicianOperatorRequest struct {
	OperatorID meta.ID `json:"operator_id" binding:"required"`
}

// ListClinicianRequest 从业者列表请求。
type ListClinicianRequest struct {
	OrgID    int64 `form:"org_id"`
	Page     int   `form:"page" binding:"omitempty,min=1"`
	PageSize int   `form:"page_size" binding:"omitempty,min=1,max=100"`
}

// CreateAssessmentEntryRequest 创建测评入口请求。
type CreateAssessmentEntryRequest struct {
	TargetType    string        `json:"target_type" binding:"required"`
	TargetCode    string        `json:"target_code" binding:"required"`
	TargetVersion string        `json:"target_version"`
	ExpiresAt     *FlexibleTime `json:"expires_at"`
}

// ListAssessmentEntryRequest 测评入口列表请求。
type ListAssessmentEntryRequest struct {
	Page     int `form:"page" binding:"omitempty,min=1"`
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=100"`
}

// AssignClinicianTesteeRequest 分配受试者给从业者。
type AssignClinicianTesteeRequest struct {
	OrgID        int64    `json:"org_id"`
	ClinicianID  meta.ID  `json:"clinician_id" binding:"required"`
	TesteeID     meta.ID  `json:"testee_id" binding:"required"`
	RelationType string   `json:"relation_type"`
	SourceType   string   `json:"source_type"`
	SourceID     *meta.ID `json:"source_id"`
}

// TransferPrimaryClinicianRequest 转移主责从业者请求。
type TransferPrimaryClinicianRequest struct {
	OrgID         int64    `json:"org_id"`
	ToClinicianID meta.ID  `json:"to_clinician_id" binding:"required"`
	TesteeID      meta.ID  `json:"testee_id" binding:"required"`
	SourceType    string   `json:"source_type"`
	SourceID      *meta.ID `json:"source_id"`
}

// IntakeByAssessmentEntryRequest 扫码 intake 请求。
type IntakeByAssessmentEntryRequest struct {
	ProfileID *uint64    `json:"profile_id"`
	Name      string     `json:"name" binding:"required"`
	Gender    string     `json:"gender"`
	Birthday  *time.Time `json:"birthday"`
}
