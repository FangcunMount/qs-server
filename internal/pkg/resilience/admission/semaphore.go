package admission

import (
	"context"
	"time"
)

// Semaphore 表示有限并发槽位。
type Semaphore interface {
	TryAcquire() bool
	AcquireBlocking()
	AcquireWithWait(ctx context.Context, maxWait time.Duration) (ok bool, waited time.Duration)
	Release()
}

// channelSemaphore 基于 channel 的槽位控制。
type channelSemaphore struct {
	sem chan struct{}
}

// noopSemaphore 永不拒绝的槽位控制。
type noopSemaphore struct{}

// NewChannelSemaphore 创建基于 channel 的槽位控制；max <= 0 时返回永不拒绝的 noop。
func NewChannelSemaphore(max int) Semaphore {
	if max <= 0 {
		return noopSemaphore{}
	}
	return &channelSemaphore{sem: make(chan struct{}, max)}
}

func (noopSemaphore) TryAcquire() bool { return true }

func (noopSemaphore) AcquireBlocking() {}

func (noopSemaphore) AcquireWithWait(context.Context, time.Duration) (bool, time.Duration) {
	return true, 0
}

func (noopSemaphore) Release() {}

// TryAcquire 尝试获取槽位。
func (s *channelSemaphore) TryAcquire() bool {
	if s == nil || s.sem == nil {
		return true
	}
	select {
	case s.sem <- struct{}{}:
		return true
	default:
		return false
	}
}

// AcquireBlocking 阻塞获取槽位。
func (s *channelSemaphore) AcquireBlocking() {
	if s == nil || s.sem == nil {
		return
	}
	s.sem <- struct{}{}
}

// AcquireWithWait 在 maxWait 内等待槽位。
func (s *channelSemaphore) AcquireWithWait(ctx context.Context, maxWait time.Duration) (bool, time.Duration) {
	if s == nil || s.sem == nil {
		return true, 0
	}
	if ctx == nil {
		ctx = context.Background()
	}
	start := time.Now()
	if maxWait <= 0 {
		select {
		case s.sem <- struct{}{}:
			return true, time.Since(start)
		case <-ctx.Done():
			return false, time.Since(start)
		}
	}
	timer := time.NewTimer(maxWait)
	defer timer.Stop()
	select {
	case s.sem <- struct{}{}:
		return true, time.Since(start)
	case <-timer.C:
		return false, time.Since(start)
	case <-ctx.Done():
		return false, time.Since(start)
	}
}

// Release 释放槽位。
func (s *channelSemaphore) Release() {
	if s == nil || s.sem == nil {
		return
	}
	select {
	case <-s.sem:
	default:
	}
}
