package actor

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	clinicianDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"gorm.io/gorm"
)

// TesteePO 受试者持久化对象
type TesteePO struct {
	mysql.AuditFields

	OrgID      int64          `gorm:"column:org_id;not null"`
	ProfileID  *uint64        `gorm:"column:profile_id;type:bigint unsigned;index:idx_profile_id"`
	Name       string         `gorm:"column:name;size:100;not null;index:idx_name"`
	Gender     int8           `gorm:"column:gender;not null"`
	Birthday   *time.Time     `gorm:"column:birthday"`
	Tags       StringSliceCol `gorm:"column:tags;type:json"`
	Source     string         `gorm:"column:source;size:50;not null;default:unknown"`
	IsKeyFocus bool           `gorm:"column:is_key_focus;not null;default:false"`

	// 测评统计字段
	TotalAssessments int        `gorm:"column:total_assessments;not null;default:0"`
	LastAssessmentAt *time.Time `gorm:"column:last_assessment_at"`
	LastRiskLevel    *string    `gorm:"column:last_risk_level;size:50"`
}

// TableName 指定表名
func (TesteePO) TableName() string {
	return "testee"
}

// StringSliceCol 字符串切片列，用于 JSON 存储
type StringSliceCol []string

// Value 实现 driver.Valuer 接口，将 Go 值转换为数据库可存储的值
func (s StringSliceCol) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	return json.Marshal(s)
}

// Scan 实现 sql.Scanner 接口，从数据库读取值
func (s *StringSliceCol) Scan(value interface{}) error {
	if value == nil {
		*s = nil
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}

	return json.Unmarshal(bytes, s)
}

// AssessmentStatsPO 测评统计持久化对象（嵌入在 TesteePO 中）
type AssessmentStatsPO struct {
	TotalAssessments int        `gorm:"column:total_assessments"`
	LastAssessmentAt *time.Time `gorm:"column:last_assessment_at"`
	LastRiskLevel    *string    `gorm:"column:last_risk_level"`
}

// OperatorPO 后台操作者持久化对象
// 兼容说明：底层表名仍为 `staff`，后续迁移前先保持存储结构稳定。
type OperatorPO struct {
	mysql.AuditFields

	OrgID    int64          `gorm:"column:org_id;not null;index:idx_org_id;uniqueIndex:uk_staff_org_user,priority:1"`
	UserID   int64          `gorm:"column:user_id;not null;index:idx_user_id;uniqueIndex:uk_staff_org_user,priority:2"`
	Roles    StringSliceCol `gorm:"column:roles;type:json;not null"`
	Name     string         `gorm:"column:name;size:100;not null"`
	Email    string         `gorm:"column:email;size:255"`
	Phone    string         `gorm:"column:phone;size:20"`
	IsActive bool           `gorm:"column:is_active;not null;default:true;index:idx_is_active"`
}

// TableName 指定表名
func (OperatorPO) TableName() string {
	return "staff"
}

// ClinicianPO 业务从业者持久化对象。
type ClinicianPO struct {
	mysql.AuditFields

	OrgID         int64   `gorm:"column:org_id;not null;index:idx_org_id"`
	OperatorID    *uint64 `gorm:"column:operator_id;type:bigint unsigned;index:idx_operator_id"`
	Name          string  `gorm:"column:name;size:100;not null;index:idx_name"`
	Department    string  `gorm:"column:department;size:100"`
	Title         string  `gorm:"column:title;size:100"`
	ClinicianType string  `gorm:"column:clinician_type;size:50;not null;index:idx_clinician_type"`
	EmployeeCode  *string `gorm:"column:employee_code;size:50"`
	IsActive      bool    `gorm:"column:is_active;not null;default:true;index:idx_is_active"`
}

// TableName 指定表名。
func (ClinicianPO) TableName() string {
	return "clinician"
}

// ClinicianRelationPO 从业者与受试者关系持久化对象。
type ClinicianRelationPO struct {
	mysql.AuditFields

	OrgID        int64              `gorm:"column:org_id;not null;index:idx_org_id"`
	ClinicianID  clinicianDomain.ID `gorm:"column:clinician_id;type:bigint unsigned;not null;index:idx_clinician_id"`
	TesteeID     meta.ID            `gorm:"column:testee_id;type:bigint unsigned;not null;index:idx_testee_id"`
	RelationType string             `gorm:"column:relation_type;size:50;not null;index:idx_relation_type"`
	SourceType   string             `gorm:"column:source_type;size:50;not null;index:idx_source_type"`
	SourceID     *uint64            `gorm:"column:source_id;type:bigint unsigned;index:idx_source_id"`
	IsActive     bool               `gorm:"column:is_active;not null;default:true;index:idx_is_active"`
	BoundAt      time.Time          `gorm:"column:bound_at;not null"`
	UnboundAt    *time.Time         `gorm:"column:unbound_at"`
}

// TableName 指定表名。
func (ClinicianRelationPO) TableName() string {
	return "clinician_relation"
}

// AssessmentEntryPO 测评入口持久化对象。
type AssessmentEntryPO struct {
	mysql.AuditFields

	OrgID         int64              `gorm:"column:org_id;not null;index:idx_org_id"`
	ClinicianID   clinicianDomain.ID `gorm:"column:clinician_id;type:bigint unsigned;not null;index:idx_clinician_id"`
	Token         string             `gorm:"column:token;size:32;not null;uniqueIndex:uk_token"`
	TargetType    string             `gorm:"column:target_type;size:50;not null;index:idx_target_type"`
	TargetCode    string             `gorm:"column:target_code;size:100;not null"`
	TargetVersion *string            `gorm:"column:target_version;size:50"`
	IsActive      bool               `gorm:"column:is_active;not null;default:true;index:idx_is_active"`
	ExpiresAt     *time.Time         `gorm:"column:expires_at;index:idx_expires_at"`
}

// TableName 指定表名。
func (AssessmentEntryPO) TableName() string {
	return "assessment_entry"
}

// BeforeCreate GORM hook，在创建前执行
func (p *TesteePO) BeforeCreate(_ *gorm.DB) error {
	// 如果 ID 为 0，使用 ID 生成器生成 ID
	if p.ID == 0 {
		p.ID = meta.New()
	}
	// 设置默认版本号
	if p.Version == 0 {
		p.Version = mysql.InitialVersion
	}
	return nil
}

// BeforeCreate GORM hook，在创建前执行
func (p *OperatorPO) BeforeCreate(_ *gorm.DB) error {
	// 如果 ID 为 0，使用 ID 生成器生成 ID
	if p.ID == 0 {
		p.ID = meta.New()
	}
	// 设置默认版本号
	if p.Version == 0 {
		p.Version = mysql.InitialVersion
	}
	return nil
}

// BeforeCreate GORM hook，在创建前执行。
func (p *ClinicianPO) BeforeCreate(_ *gorm.DB) error {
	if p.ID == 0 {
		p.ID = meta.New()
	}
	if p.Version == 0 {
		p.Version = mysql.InitialVersion
	}
	return nil
}

// BeforeCreate GORM hook，在创建前执行。
func (p *ClinicianRelationPO) BeforeCreate(_ *gorm.DB) error {
	if p.ID == 0 {
		p.ID = meta.New()
	}
	if p.Version == 0 {
		p.Version = mysql.InitialVersion
	}
	return nil
}

// BeforeCreate GORM hook，在创建前执行。
func (p *AssessmentEntryPO) BeforeCreate(_ *gorm.DB) error {
	if p.ID == 0 {
		p.ID = meta.New()
	}
	if p.Version == 0 {
		p.Version = mysql.InitialVersion
	}
	return nil
}
