package assessmententry

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
)

// AssessmentEntry 测评入口聚合根。
type AssessmentEntry struct {
	id            ID
	orgID         int64
	clinicianID   clinician.ID
	token         string
	targetType    TargetType
	targetCode    string
	targetVersion string
	isActive      bool
	expiresAt     *time.Time
}

// NewAssessmentEntry 创建测评入口。
func NewAssessmentEntry(
	orgID int64,
	clinicianID clinician.ID,
	token string,
	targetType TargetType,
	targetCode string,
	targetVersion string,
	isActive bool,
	expiresAt *time.Time,
) *AssessmentEntry {
	return &AssessmentEntry{
		orgID:         orgID,
		clinicianID:   clinicianID,
		token:         token,
		targetType:    targetType,
		targetCode:    targetCode,
		targetVersion: targetVersion,
		isActive:      isActive,
		expiresAt:     expiresAt,
	}
}

// ID 获取入口ID。
func (e *AssessmentEntry) ID() ID {
	return e.id
}

// OrgID 获取机构ID。
func (e *AssessmentEntry) OrgID() int64 {
	return e.orgID
}

// ClinicianID 获取从业者ID。
func (e *AssessmentEntry) ClinicianID() clinician.ID {
	return e.clinicianID
}

// Token 获取入口令牌。
func (e *AssessmentEntry) Token() string {
	return e.token
}

// TargetType 获取目标类型。
func (e *AssessmentEntry) TargetType() TargetType {
	return e.targetType
}

// TargetCode 获取目标编码。
func (e *AssessmentEntry) TargetCode() string {
	return e.targetCode
}

// TargetVersion 获取目标版本。
func (e *AssessmentEntry) TargetVersion() string {
	return e.targetVersion
}

// IsActive 是否激活。
func (e *AssessmentEntry) IsActive() bool {
	return e.isActive
}

// ExpiresAt 获取过期时间。
func (e *AssessmentEntry) ExpiresAt() *time.Time {
	return e.expiresAt
}

// IsExpired 判断是否已过期。
func (e *AssessmentEntry) IsExpired(now time.Time) bool {
	return e.expiresAt != nil && now.After(*e.expiresAt)
}

// CanResolve 判断是否可解析。
func (e *AssessmentEntry) CanResolve(now time.Time) bool {
	return e.isActive && !e.IsExpired(now)
}

// SetID 设置ID。
func (e *AssessmentEntry) SetID(id ID) {
	e.id = id
}

// Deactivate 停用入口。
func (e *AssessmentEntry) Deactivate() {
	e.isActive = false
}

// Reactivate 重新启用入口。
func (e *AssessmentEntry) Reactivate() {
	e.isActive = true
}
