package testee

import (
	"regexp"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// ID 受试者ID类型
type ID = meta.ID

// NewID 创建受试者ID
func NewID(id uint64) ID {
	return meta.FromUint64(id)
}

// Gender 性别枚举
type Gender int8

const (
	GenderUnknown Gender = 0 // 未知
	GenderMale    Gender = 1 // 男
	GenderFemale  Gender = 2 // 女
)

// String 返回性别的字符串表示
func (g Gender) String() string {
	switch g {
	case GenderMale:
		return "male"
	case GenderFemale:
		return "female"
	default:
		return "unknown"
	}
}

// Tag 标签类型
type Tag string

func (t Tag) String() string {
	return string(t)
}

func (t Tag) IsValid() bool {
	// 规则：只允许字母、数字、下划线、中文
	return regexp.MustCompile(`^[\w\p{Han}]+$`).MatchString(string(t))
}

// AssessmentStats 测评统计快照（值对象）
// 通过领域事件异步更新，不应直接修改
type AssessmentStats struct {
	lastAssessmentAt time.Time // 最近一次测评完成时间
	totalCount       int       // 总测评次数
	lastRiskLevel    string    // 最近一次测评的风险等级
}

// NewAssessmentStats 创建测评统计快照
func NewAssessmentStats(lastAssessmentAt time.Time, totalCount int, lastRiskLevel string) *AssessmentStats {
	return &AssessmentStats{
		lastAssessmentAt: lastAssessmentAt,
		totalCount:       totalCount,
		lastRiskLevel:    lastRiskLevel,
	}
}

// LastAssessmentAt 获取最近测评时间
func (s *AssessmentStats) LastAssessmentAt() time.Time {
	return s.lastAssessmentAt
}

// TotalCount 获取总测评次数
func (s *AssessmentStats) TotalCount() int {
	return s.totalCount
}

// LastRiskLevel 获取最近风险等级
func (s *AssessmentStats) LastRiskLevel() string {
	return s.lastRiskLevel
}
