package question

// Option 选项
type Option struct {
	code    string
	content string
	score   int
}

// NewOption 创建选项
func NewOption(code, content string, score int) Option {
	return Option{
		code:    code,
		content: content,
		score:   score,
	}
}

// GetCode 获取选项编码
func (o *Option) GetCode() string {
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
