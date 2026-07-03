package concurrency

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/admission"
	"github.com/gin-gonic/gin"
)

// Gate 基于 channel 的 HTTP 并发槽位控制。
type Gate struct {
	sem admission.Semaphore
}

func NewGate(max int) *Gate {
	if max <= 0 {
		return nil
	}
	return &Gate{sem: admission.NewChannelSemaphore(max)}
}

// Semaphore 暴露底层准入槽位，供 gRPC 等路径复用。
func (g *Gate) Semaphore() admission.Semaphore {
	if g == nil {
		return nil
	}
	return g.sem
}

func (g *Gate) TryAcquire() bool {
	if g == nil || g.sem == nil {
		return true
	}
	return g.sem.TryAcquire()
}

func (g *Gate) Release() {
	if g == nil || g.sem == nil {
		return
	}
	g.sem.Release()
}

// AcquireWithWait 在 maxWait 内等待槽位；超时返回 false。
func (g *Gate) AcquireWithWait(ctx context.Context, maxWait time.Duration) (acquired bool, waited time.Duration) {
	if g == nil || g.sem == nil {
		return true, 0
	}
	return g.sem.AcquireWithWait(ctx, maxWait)
}

// WaitMiddleware 在 maxWait 内等待槽位，超时执行 onReject 并中断请求链。
func (g *Gate) WaitMiddleware(maxWait time.Duration, onReject gin.HandlerFunc) gin.HandlerFunc {
	return admission.HTTPWaitMiddleware(g.Semaphore(), maxWait, onReject, admission.ObserveHTTPGateWait)
}

// BlockingMiddleware 阻塞等待槽位（用于 general HTTP 池）。
func (g *Gate) BlockingMiddleware() gin.HandlerFunc {
	return admission.HTTPBlockingMiddleware(g.Semaphore(), admission.ObserveHTTPGateWait)
}

// TryMiddleware 槽位满时执行 onReject 并中断请求链（用于 wait-report 过载降级）。
func (g *Gate) TryMiddleware(onReject gin.HandlerFunc) gin.HandlerFunc {
	return admission.HTTPTryMiddleware(g.Semaphore(), onReject)
}
