package answersheet

import (
	"errors"
	"strings"
)

// QuestionnaireRef 问卷引用值对象
type QuestionnaireRef struct {
	code    string
	version string
	title   string
}

// NewQuestionnaireRef 创建问卷引用
func NewQuestionnaireRef(code, version, title string) QuestionnaireRef {
	return QuestionnaireRef{
		code:    code,
		version: version,
		title:   title,
	}
}

// Code 获取问卷编码
func (r QuestionnaireRef) Code() string {
	return r.code
}

// Version 获取问卷版本
func (r QuestionnaireRef) Version() string {
	return r.version
}

// Title 获取问卷标题
func (r QuestionnaireRef) Title() string {
	return r.title
}

// Validate 验证问卷引用
func (r QuestionnaireRef) Validate() error {
	if strings.TrimSpace(r.code) == "" {
		return errors.New("questionnaire code cannot be empty")
	}
	if strings.TrimSpace(r.version) == "" {
		return errors.New("questionnaire version cannot be empty")
	}
	return nil
}

// IsEmpty 是否为空引用
func (r QuestionnaireRef) IsEmpty() bool {
	return r.code == "" && r.version == ""
}
