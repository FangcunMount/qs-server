package question

import "github.com/FangcunMount/qs-server/pkg/util/codeutil"

// OptionCode 选项编码
type OptionCode = codeutil.Code

// NewOptionCode 创建选项编码
func NewOptionCode(value string) OptionCode {
	return codeutil.NewCode(value)
}

// GenerateOptionCode 生成新的选项编码
func GenerateOptionCode() (OptionCode, error) {
	return codeutil.GenerateNewCode()
}

// Option 选项
type Option struct {
	code    OptionCode
	content string
	score   int
}

// NewOption 创建选项
func NewOption(code OptionCode, content string, score int) Option {
	return Option{
		code:    code,
		content: content,
		score:   score,
	}
}

// NewOptionWithStringCode 使用字符串编码创建选项
func NewOptionWithStringCode(code string, content string, score int) Option {
	return Option{
		code:    NewOptionCode(code),
		content: content,
		score:   score,
	}
}

// GetCode 获取选项编码
func (o *Option) GetCode() OptionCode {
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
