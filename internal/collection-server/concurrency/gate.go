package concurrency

import (
	"github.com/gin-gonic/gin"
)

// Gate 基于 channel 的 HTTP 并发槽位控制。
type Gate struct {
	sem chan struct{}
}

func NewGate(max int) *Gate {
	if max <= 0 {
		return nil
	}
	return &Gate{sem: make(chan struct{}, max)}
}

func (g *Gate) TryAcquire() bool {
	if g == nil || g.sem == nil {
		return true
	}
	select {
	case g.sem <- struct{}{}:
		return true
	default:
		return false
	}
}

func (g *Gate) Release() {
	if g == nil || g.sem == nil {
		return
	}
	select {
	case <-g.sem:
	default:
	}
}

// BlockingMiddleware 阻塞等待槽位（用于 general HTTP 池）。
func (g *Gate) BlockingMiddleware() gin.HandlerFunc {
	if g == nil {
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		g.sem <- struct{}{}
		defer g.Release()
		c.Next()
	}
}

// TryMiddleware 槽位满时执行 onReject 并中断请求链（用于 wait-report 过载降级）。
func (g *Gate) TryMiddleware(onReject gin.HandlerFunc) gin.HandlerFunc {
	if g == nil {
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		if !g.TryAcquire() {
			if onReject != nil {
				onReject(c)
			}
			c.Abort()
			return
		}
		defer g.Release()
		c.Next()
	}
}
