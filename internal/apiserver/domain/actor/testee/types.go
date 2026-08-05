package testee

import "github.com/FangcunMount/qs-server/internal/pkg/meta"

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

func (t Tag) String() string {
	return string(t)
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

// IsSeeddataMockSource 判断是否为 seeddata / 日常模拟等 mock 受试者。
func IsSeeddataMockSource(source Source) bool {
	switch source {
	case SourceSeeddata, SourceDailySimulation:
		return true
	default:
		return false
	}
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
