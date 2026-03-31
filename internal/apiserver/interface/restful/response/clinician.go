package response

import "time"

// ClinicianResponse 从业者响应。
type ClinicianResponse struct {
	ID            string  `json:"id"`
	OrgID         string  `json:"org_id"`
	OperatorID    *string `json:"operator_id,omitempty"`
	Name          string  `json:"name"`
	Department    string  `json:"department,omitempty"`
	Title         string  `json:"title,omitempty"`
	ClinicianType string  `json:"clinician_type"`
	EmployeeCode  string  `json:"employee_code,omitempty"`
	IsActive      bool    `json:"is_active"`
}

// ClinicianListResponse 从业者列表响应。
type ClinicianListResponse struct {
	Items      []*ClinicianResponse `json:"items"`
	Total      int64                `json:"total"`
	Page       int                  `json:"page"`
	PageSize   int                  `json:"page_size"`
	TotalPages int                  `json:"total_pages"`
}

// AssessmentEntryResponse 测评入口响应。
type AssessmentEntryResponse struct {
	ID            string     `json:"id"`
	OrgID         string     `json:"org_id"`
	ClinicianID   string     `json:"clinician_id"`
	Token         string     `json:"token"`
	TargetType    string     `json:"target_type"`
	TargetCode    string     `json:"target_code"`
	TargetVersion string     `json:"target_version,omitempty"`
	IsActive      bool       `json:"is_active"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	QRCodeURL     string     `json:"qrcode_url,omitempty"`
}

// AssessmentEntryListResponse 测评入口列表响应。
type AssessmentEntryListResponse struct {
	Items      []*AssessmentEntryResponse `json:"items"`
	Total      int64                      `json:"total"`
	Page       int                        `json:"page"`
	PageSize   int                        `json:"page_size"`
	TotalPages int                        `json:"total_pages"`
}

// ClinicianSummaryResponse 从业者摘要响应。
type ClinicianSummaryResponse struct {
	ID            string  `json:"id"`
	OperatorID    *string `json:"operator_id,omitempty"`
	Name          string  `json:"name"`
	Department    string  `json:"department,omitempty"`
	Title         string  `json:"title,omitempty"`
	ClinicianType string  `json:"clinician_type"`
}

// AssessmentEntryResolvedResponse 测评入口解析响应。
type AssessmentEntryResolvedResponse struct {
	Entry     *AssessmentEntryResponse  `json:"entry"`
	Clinician *ClinicianSummaryResponse `json:"clinician"`
}

// AssessmentEntryIntakeResponse 测评入口 intake 响应。
type AssessmentEntryIntakeResponse struct {
	Entry      *AssessmentEntryResponse  `json:"entry"`
	Clinician  *ClinicianSummaryResponse `json:"clinician"`
	Testee     *TesteeResponse           `json:"testee"`
	Relation   *RelationResponse         `json:"relation,omitempty"`
	Assignment *RelationResponse         `json:"assignment,omitempty"`
}

// RelationResponse 从业者关系响应。
type RelationResponse struct {
	ID           string    `json:"id"`
	OrgID        string    `json:"org_id"`
	ClinicianID  string    `json:"clinician_id"`
	TesteeID     string    `json:"testee_id"`
	RelationType string    `json:"relation_type"`
	SourceType   string    `json:"source_type"`
	SourceID     *string   `json:"source_id,omitempty"`
	IsActive     bool      `json:"is_active"`
	BoundAt      time.Time `json:"bound_at"`
}
