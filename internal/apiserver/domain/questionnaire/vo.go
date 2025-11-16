package questionnaire

import (
	"strconv"
)

// Status 问卷状态
type QuestionnaireStatus uint8

const (
	STATUS_DRAFT     QuestionnaireStatus = 0 // 草稿
	STATUS_PUBLISHED QuestionnaireStatus = 1 // 已发布
	STATUS_ARCHIVED  QuestionnaireStatus = 2 // 已归档
)

// Value 获取状态值
func (s QuestionnaireStatus) Value() uint8 {
	return uint8(s)
}

// String 获取状态字符串
func (s QuestionnaireStatus) String() string {
	switch s {
	case STATUS_DRAFT:
		return "draft"
	case STATUS_PUBLISHED:
		return "published"
	case STATUS_ARCHIVED:
		return "unpublished"
	default:
		return "unknown"
	}
}

// QuestionnaireVersion 问卷版本
type QuestionnaireVersion string

// NewQuestionnaireVersion 创建问卷版本
func NewQuestionnaireVersion(value string) QuestionnaireVersion {
	return QuestionnaireVersion(value)
}

// Value 获取版本值
func (v QuestionnaireVersion) Value() string {
	return string(v)
}

// Increment 增加版本号
func (v QuestionnaireVersion) Increment() QuestionnaireVersion {
	version, err := strconv.Atoi(v.Value())
	if err != nil {
		return v
	}
	return QuestionnaireVersion(strconv.Itoa(version + 1))
}
