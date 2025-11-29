package pipeline

import "context"

// Chain 处理器链
// 封装职责链的构建和执行
type Chain struct {
	head Handler
	tail Handler
}

// NewChain 创建处理器链
func NewChain() *Chain {
	return &Chain{}
}

// AddHandler 添加处理器到链尾
func (c *Chain) AddHandler(handler Handler) *Chain {
	if c.head == nil {
		c.head = handler
		c.tail = handler
	} else {
		c.tail.SetNext(handler)
		c.tail = handler
	}
	return c
}

// Execute 执行处理器链
func (c *Chain) Execute(ctx context.Context, evalCtx *Context) error {
	if c.head == nil {
		return nil
	}
	return c.head.Handle(ctx, evalCtx)
}

// Head 获取链头处理器
func (c *Chain) Head() Handler {
	return c.head
}
