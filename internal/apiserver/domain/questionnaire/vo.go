package questionnaire

// QuestionnaireID 问卷唯一标识
type QuestionnaireID struct {
	value uint64
}

// NewQuestionnaireID 创建问卷ID
func NewQuestionnaireID(value uint64) QuestionnaireID {
	return QuestionnaireID{value: value}
}

// Value 获取ID值
func (id QuestionnaireID) Value() uint64 {
	return id.value
}

// Code 问卷编码
type Code string

// NewCode 创建问卷编码
func NewCode(value string) Code {
	return Code(value)
}

// Value 获取编码值
func (c Code) Value() string {
	return string(c)
}

// Status 问卷状态
type Status uint8

const (
	StatusInit     Status = 0 // 草稿
	StatusActive   Status = 1 // 已发布
	StatusInactive Status = 2 // 已下架
)

// Value 获取状态值
func (s Status) Value() uint8 {
	return uint8(s)
}

// String 获取状态字符串
func (s Status) String() string {
	switch s {
	case StatusInit:
		return "draft"
	case StatusActive:
		return "published"
	case StatusInactive:
		return "unpublished"
	default:
		return "unknown"
	}
}
