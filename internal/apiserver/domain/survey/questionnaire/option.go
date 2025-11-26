package questionnaire

import (
	"fmt"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// Option 问题选项（值对象）
// 表示问题的一个可选答案，由编码、内容和分数三元组定义
// 作为值对象，Option 是不可变的，通过相等性而非身份识别
type Option struct {
	code    meta.Code // 选项编码（唯一标识）
	content string    // 选项文本内容
	score   float64   // 选项分数（支持小数，如心理量表的 4.5 分）
}

// NewOption 创建选项
// 参数验证确保创建的选项是有效的
func NewOption(codeVal meta.Code, content string, score float64) (Option, error) {
	if codeVal.Value() == "" {
		return Option{}, errors.WithCode(code.ErrQuestionnaireInvalidQuestion, "option code cannot be empty")
	}
	if content == "" {
		return Option{}, errors.WithCode(code.ErrQuestionnaireInvalidQuestion, "option content cannot be empty")
	}

	return Option{
		code:    codeVal,
		content: content,
		score:   score,
	}, nil
}

// NewOptionWithStringCode 使用字符串编码创建选项（便捷方法）
func NewOptionWithStringCode(codeStr, content string, score float64) (Option, error) {
	return NewOption(meta.NewCode(codeStr), content, score)
}

// GetCode 获取选项编码
func (o Option) GetCode() meta.Code {
	return o.code
}

// GetContent 获取选项内容
func (o Option) GetContent() string {
	return o.content
}

// GetScore 获取选项分数
func (o Option) GetScore() float64 {
	return o.score
}

// Equals 判断两个选项是否相等
// 值对象的相等性：当所有字段值都相同时认为相等
func (o Option) Equals(other Option) bool {
	return o.code.Equals(other.code) &&
		o.content == other.content &&
		o.score == other.score
}

// String 字符串表示（便于调试和日志）
func (o Option) String() string {
	return fmt.Sprintf("Option[%s: %s (%.1f)]",
		o.code.Value(), o.content, o.score)
}
