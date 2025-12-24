package statistics

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
)

// ==================== StatisticsDaily 持久化对象 ====================

// StatisticsDailyPO 每日统计持久化对象
type StatisticsDailyPO struct {
	mysql.AuditFields

	OrgID         int64     `gorm:"column:org_id;not null;index:idx_org_date"`
	StatisticType string    `gorm:"column:statistic_type;size:50;not null"`
	StatisticKey  string    `gorm:"column:statistic_key;size:255;not null"`
	StatDate      time.Time `gorm:"column:stat_date;type:date;not null"`

	SubmissionCount int64     `gorm:"column:submission_count;not null;default:0"`
	CompletionCount int64     `gorm:"column:completion_count;not null;default:0"`
	ExtraMetrics    JSONField `gorm:"column:extra_metrics;type:json"`

	CreatedAt time.Time `gorm:"column:created_at;type:timestamp;default:CURRENT_TIMESTAMP"`
	UpdatedAt time.Time `gorm:"column:updated_at;type:timestamp;default:CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP"`
}

// TableName 指定表名
func (StatisticsDailyPO) TableName() string {
	return "statistics_daily"
}

// ==================== StatisticsAccumulated 持久化对象 ====================

// StatisticsAccumulatedPO 累计统计持久化对象
type StatisticsAccumulatedPO struct {
	mysql.AuditFields

	OrgID         int64  `gorm:"column:org_id;not null;index:idx_org_type"`
	StatisticType string `gorm:"column:statistic_type;size:50;not null"`
	StatisticKey  string `gorm:"column:statistic_key;size:255;not null"`

	TotalSubmissions int64 `gorm:"column:total_submissions;not null;default:0"`
	TotalCompletions int64 `gorm:"column:total_completions;not null;default:0"`

	Last7dSubmissions  int64 `gorm:"column:last7d_submissions;not null;default:0"`
	Last15dSubmissions int64 `gorm:"column:last15d_submissions;not null;default:0"`
	Last30dSubmissions int64 `gorm:"column:last30d_submissions;not null;default:0"`

	Distribution JSONField `gorm:"column:distribution;type:json"`

	FirstOccurredAt *time.Time `gorm:"column:first_occurred_at;type:timestamp"`
	LastOccurredAt  *time.Time `gorm:"column:last_occurred_at;type:timestamp"`

	LastUpdatedAt time.Time `gorm:"column:last_updated_at;type:timestamp;default:CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP"`
}

// TableName 指定表名
func (StatisticsAccumulatedPO) TableName() string {
	return "statistics_accumulated"
}

// ==================== StatisticsPlan 持久化对象 ====================

// StatisticsPlanPO 计划统计持久化对象
type StatisticsPlanPO struct {
	mysql.AuditFields

	OrgID  int64  `gorm:"column:org_id;not null;index:idx_org_id"`
	PlanID uint64 `gorm:"column:plan_id;not null"`

	TotalTasks     int64 `gorm:"column:total_tasks;not null;default:0"`
	CompletedTasks int64 `gorm:"column:completed_tasks;not null;default:0"`
	PendingTasks   int64 `gorm:"column:pending_tasks;not null;default:0"`
	ExpiredTasks   int64 `gorm:"column:expired_tasks;not null;default:0"`

	EnrolledTestees int64 `gorm:"column:enrolled_testees;not null;default:0"`
	ActiveTestees   int64 `gorm:"column:active_testees;not null;default:0"`

	LastUpdatedAt time.Time `gorm:"column:last_updated_at;type:timestamp;default:CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP"`
}

// TableName 指定表名
func (StatisticsPlanPO) TableName() string {
	return "statistics_plan"
}

// ==================== JSONField 辅助类型 ====================

// JSONField JSON字段类型
type JSONField map[string]interface{}

// Value 实现 driver.Valuer 接口
func (j JSONField) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan 实现 sql.Scanner 接口
func (j *JSONField) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return json.Unmarshal([]byte(value.(string)), j)
	}

	return json.Unmarshal(bytes, j)
}
