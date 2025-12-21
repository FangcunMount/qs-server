package plan

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// ==================== AssessmentPlan 持久化对象 ====================

// AssessmentPlanPO 测评计划持久化对象
type AssessmentPlanPO struct {
	mysql.AuditFields

	// 组织信息
	OrgID int64 `gorm:"column:org_id;not null;index:idx_org_id"`

	// 量表引用
	ScaleCode string `gorm:"column:scale_code;size:100;not null;index:idx_scale_code"`

	// 周期策略
	ScheduleType  string      `gorm:"column:schedule_type;size:50;not null;index:idx_schedule_type"`
	Interval      int         `gorm:"column:interval;not null;default:0"`
	TotalTimes    int         `gorm:"column:total_times;not null"`
	FixedDates    StringSlice `gorm:"column:fixed_dates;type:json"`    // 固定日期列表（JSON）
	RelativeWeeks IntSlice    `gorm:"column:relative_weeks;type:json"` // 相对周次列表（JSON）

	// 状态
	Status string `gorm:"column:status;size:50;not null;default:'active';index:idx_status"`
}

// TableName 指定表名
func (AssessmentPlanPO) TableName() string {
	return "assessment_plan"
}

// BeforeCreate GORM hook，在创建前执行
func (p *AssessmentPlanPO) BeforeCreate() error {
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

// ==================== AssessmentTask 持久化对象 ====================

// AssessmentTaskPO 测评任务持久化对象
type AssessmentTaskPO struct {
	mysql.AuditFields

	// 关联计划
	PlanID uint64 `gorm:"column:plan_id;not null;index:idx_plan_id"`

	// 序号
	Seq int `gorm:"column:seq;not null;index:idx_plan_seq"` // 计划内的序号

	// 组织信息（冗余，用于查询优化和权限控制）
	OrgID int64 `gorm:"column:org_id;not null;index:idx_org_id"`

	// 受试者信息
	TesteeID uint64 `gorm:"column:testee_id;not null;index:idx_testee_id"`

	// 量表引用（冗余，用于查询优化）
	ScaleCode string `gorm:"column:scale_code;size:100;not null;index:idx_scale_code"`

	// 时间点
	PlannedAt   time.Time  `gorm:"column:planned_at;not null;index:idx_planned_at"`
	OpenAt      *time.Time `gorm:"column:open_at;index:idx_open_at"`
	ExpireAt    *time.Time `gorm:"column:expire_at;index:idx_expire_at"`
	CompletedAt *time.Time `gorm:"column:completed_at"`

	// 状态与关联
	Status       string  `gorm:"column:status;size:50;not null;default:'pending';index:idx_status"`
	AssessmentID *uint64 `gorm:"column:assessment_id;index:idx_assessment_id"`

	// 入口信息
	EntryToken string `gorm:"column:entry_token;size:255"`
	EntryURL   string `gorm:"column:entry_url;size:500"`

	// 复合索引：计划ID + 序号（确保同一计划内序号唯一）
	// 复合索引：计划ID + 受试者ID + 序号（用于查询某个受试者在某个计划下的任务）
}

// TableName 指定表名
func (AssessmentTaskPO) TableName() string {
	return "assessment_task"
}

// BeforeCreate GORM hook，在创建前执行
func (p *AssessmentTaskPO) BeforeCreate() error {
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

// ==================== 辅助类型 ====================

// StringSlice 字符串切片列，用于 JSON 存储（存储时间字符串）
type StringSlice []string

// Value 实现 driver.Valuer 接口
func (s StringSlice) Value() (driver.Value, error) {
	if len(s) == 0 {
		return nil, nil
	}
	return json.Marshal(s)
}

// Scan 实现 sql.Scanner 接口
func (s *StringSlice) Scan(value interface{}) error {
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

// IntSlice 整数切片列，用于 JSON 存储
type IntSlice []int

// Value 实现 driver.Valuer 接口
func (s IntSlice) Value() (driver.Value, error) {
	if len(s) == 0 {
		return nil, nil
	}
	return json.Marshal(s)
}

// Scan 实现 sql.Scanner 接口
func (s *IntSlice) Scan(value interface{}) error {
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
