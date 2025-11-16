package question

import "github.com/FangcunMount/qs-server/internal/pkg/meta"

// Option 选项
type Option struct {
	code    meta.Code
	content string
	score   int
}

// NewOption 创建选项
func NewOption(code meta.Code, content string, score int) Option {
	return Option{
		code:    code,
		content: content,
		score:   score,
	}
}

// NewOptionWithStringCode 使用字符串编码创建选项
func NewOptionWithStringCode(code string, content string, score int) Option {
	return Option{
		code:    meta.NewCode(code),
		content: content,
		score:   score,
	}
}

// GetCode 获取选项编码
func (o *Option) GetCode() meta.Code {
	return o.code
}

// GetContent 获取选项内容
func (o *Option) GetContent() string {
	return o.content
}

// GetScore 获取选项分数
func (o *Option) GetScore() int {
	return o.score
}
