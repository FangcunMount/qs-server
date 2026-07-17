package admission

import (
	"time"

	"github.com/gin-gonic/gin"
)

// HTTPWaitMiddleware 在 maxWait 内等待槽位，超时执行 onReject 并中断请求链。
func HTTPWaitMiddleware(sem Semaphore, maxWait time.Duration, onReject gin.HandlerFunc, observe func(time.Duration)) gin.HandlerFunc {
	if sem == nil {
		return func(c *gin.Context) { c.Next() }
	}
	strategy := WithWaitObserver(WaitStrategy{Sem: sem, MaxWait: maxWait}, observe)
	return NewHTTPMiddleware(strategy, onReject)
}

// HTTPBlockingMiddleware 阻塞等待槽位（用于 general HTTP 池）。
func HTTPBlockingMiddleware(sem Semaphore, observe func(time.Duration)) gin.HandlerFunc {
	if sem == nil {
		return func(c *gin.Context) { c.Next() }
	}
	strategy := WithWaitObserver(BlockingStrategy{Sem: sem}, observe)
	return NewHTTPMiddleware(strategy, nil)
}

// HTTPTryMiddleware 槽位满时执行 onReject 并中断请求链。
func HTTPTryMiddleware(sem Semaphore, onReject gin.HandlerFunc) gin.HandlerFunc {
	if sem == nil {
		return func(c *gin.Context) { c.Next() }
	}
	strategy := TryStrategy{Sem: sem}
	return NewHTTPMiddleware(strategy, onReject)
}
