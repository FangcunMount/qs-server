package actor

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
)

// TesteePO 受试者持久化对象
type TesteePO struct {
	mysql.AuditFields

	OrgID      int64          `gorm:"column:org_id;not null;index:idx_org_id"`
	IAMUserID  *int64         `gorm:"column:iam_user_id;index:idx_iam_user_id"`
	IAMChildID *int64         `gorm:"column:iam_child_id;index:idx_iam_child_id"`
	Name       string         `gorm:"column:name;size:100;not null;index:idx_name"`
	Gender     int8           `gorm:"column:gender;not null"`
	Birthday   *time.Time     `gorm:"column:birthday"`
	Tags       StringSliceCol `gorm:"column:tags;type:json"`
	Source     string         `gorm:"column:source;size:50;not null;default:unknown"`
	IsKeyFocus bool           `gorm:"column:is_key_focus;not null;default:false;index:idx_is_key_focus"`

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

// StaffPO 员工持久化对象
type StaffPO struct {
	mysql.AuditFields

	OrgID     int64          `gorm:"column:org_id;not null;index:idx_org_id"`
	IAMUserID int64          `gorm:"column:iam_user_id;not null;index:idx_iam_user_id"`
	Roles     StringSliceCol `gorm:"column:roles;type:json;not null"`
	Name      string         `gorm:"column:name;size:100;not null"`
	Email     string         `gorm:"column:email;size:255"`
	Phone     string         `gorm:"column:phone;size:20"`
	IsActive  bool           `gorm:"column:is_active;not null;default:true;index:idx_is_active"`
}

// TableName 指定表名
func (StaffPO) TableName() string {
	return "staff"
}

// BeforeCreate GORM hook，在创建前执行
func (p *TesteePO) BeforeCreate() error {
	if p.Version == 0 {
		p.Version = mysql.InitialVersion
	}
	return nil
}

// BeforeCreate GORM hook，在创建前执行
func (p *StaffPO) BeforeCreate() error {
	if p.Version == 0 {
		p.Version = mysql.InitialVersion
	}
	return nil
}
