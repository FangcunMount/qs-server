package admission

import (
	"context"
	"time"
)

// Strategy 定义槽位获取语义。
type Strategy interface {
	Acquire(ctx context.Context) (release func(), waited time.Duration, err error)
}

// BlockingStrategy 阻塞直到获取槽位或 context 取消。
type BlockingStrategy struct {
	Sem Semaphore
}

// Acquire 获取槽位。
func (s BlockingStrategy) Acquire(ctx context.Context) (func(), time.Duration, error) {
	sem := s.Sem
	if sem == nil {
		return func() {}, 0, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	start := time.Now()
	ok, waited := sem.AcquireWithWait(ctx, 0)
	if !ok {
		if err := ctx.Err(); err != nil {
			return nil, waited, err
		}
		return nil, waited, ErrWaitTimeout
	}
	if waited == 0 {
		waited = time.Since(start)
	}
	return sem.Release, waited, nil
}

// WaitStrategy 在 MaxWait 内等待槽位。
type WaitStrategy struct {
	Sem     Semaphore
	MaxWait time.Duration
}

// Acquire 获取槽位。
func (s WaitStrategy) Acquire(ctx context.Context) (func(), time.Duration, error) {
	sem := s.Sem
	if sem == nil {
		return func() {}, 0, nil
	}
	ok, waited := sem.AcquireWithWait(ctx, s.MaxWait)
	if !ok {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return nil, waited, err
			}
		}
		return nil, waited, ErrWaitTimeout
	}
	return sem.Release, waited, nil
}

// TryStrategy 槽位满时立即拒绝。
type TryStrategy struct {
	Sem Semaphore
}

// Acquire 获取槽位。
func (s TryStrategy) Acquire(context.Context) (func(), time.Duration, error) {
	sem := s.Sem
	if sem == nil {
		return func() {}, 0, nil
	}
	if !sem.TryAcquire() {
		return nil, 0, ErrTryRejected
	}
	return sem.Release, 0, nil
}

// observeStrategy 在准入后记录等待时长。
type observeStrategy struct {
	inner    Strategy
	observer func(time.Duration)
}

// WithWaitObserver 包装策略并在 Acquire 后上报等待时长。
func WithWaitObserver(strategy Strategy, observer func(time.Duration)) Strategy {
	if strategy == nil || observer == nil {
		return strategy
	}
	return observeStrategy{inner: strategy, observer: observer}
}

// Acquire 获取槽位。
func (s observeStrategy) Acquire(ctx context.Context) (func(), time.Duration, error) {
	release, waited, err := s.inner.Acquire(ctx)
	if s.observer != nil && waited >= 0 {
		s.observer(waited)
	}
	return release, waited, err
}
