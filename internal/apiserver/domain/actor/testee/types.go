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

// DisplayName 返回性别的中文展示名称。
func (g Gender) DisplayName() string {
	switch g {
	case GenderMale:
		return "男"
	case GenderFemale:
		return "女"
	default:
		return "未知"
	}
}

// Tag 标签类型
type Tag string

const (
	TagKeyFocus Tag = "key_focus"
)

func (t Tag) String() string {
	return string(t)
}

// DisplayName 返回标签的中文展示名称。
func (t Tag) DisplayName() string {
	switch t {
	case "risk_high":
		return "高风险"
	case "risk_medium":
		return "中风险"
	case "risk_low":
		return "低风险"
	case "risk_severe":
		return "严重风险"
	case TagKeyFocus:
		return "重点关注"
	case "daily_simulation":
		return "日常模拟"
	case "seeddata":
		return "种子数据"
	default:
		return string(t)
	}
}

func (t Tag) IsValid() bool {
	// 规则：只允许字母、数字、下划线、中文
	return regexp.MustCompile(`^[\w\p{Han}]+$`).MatchString(string(t))
}

// Source 数据来源类型。
type Source string

const (
	SourceUnknown         Source = "unknown"
	SourceManual          Source = "manual"
	SourceImport          Source = "import"
	SourceAssessmentEntry Source = "assessment_entry"
	SourceRegistration    Source = "registration"
	SourceSelfRegistered  Source = "self_registered"
	SourceSelfRegister    Source = "self_register"
	SourceIntake          Source = "intake"
	SourceScreening       Source = "screening"
	SourceWechat          Source = "wechat"
	SourceWX              Source = "wx"
	SourceDailySimulation Source = "daily_simulation"
	SourceSeeddata        Source = "seeddata"
	SourceProfile         Source = "profile"
)

func (s Source) String() string {
	return string(s)
}

// DisplayName 返回数据来源的中文展示名称。
func (s Source) DisplayName() string {
	switch s {
	case SourceManual:
		return "手动创建"
	case SourceImport:
		return "导入"
	case SourceAssessmentEntry:
		return "测评入口"
	case SourceRegistration:
		return "用户注册"
	case SourceSelfRegistered, SourceSelfRegister:
		return "自主注册"
	case SourceIntake:
		return "接入流程"
	case SourceScreening:
		return "筛查"
	case SourceWechat, SourceWX:
		return "微信"
	case SourceDailySimulation:
		return "日常模拟"
	case SourceSeeddata:
		return "种子数据"
	case SourceProfile:
		return "档案导入"
	case SourceUnknown:
		return "未知来源"
	default:
		return string(s)
	}
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
