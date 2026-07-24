package redisops

import (
	"context"
	"errors"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
	redis "github.com/redis/go-redis/v9"
)

const (
	defaultSubmitLeaseTTL       = 5 * time.Minute
	defaultSubmitWaitTimeout    = 500 * time.Millisecond
	defaultSubmitPollInterval   = 20 * time.Millisecond
	defaultSubmitSignalTTL      = 5 * time.Minute
	submitSignalWriteTimeout    = 100 * time.Millisecond
	submitCompletionSignalValue = "committed"
)

var errSubmitSignalUnavailable = errors.New("submit completion signal Redis is unavailable")

// SubmitCoalescerConfig bounds how long a contender waits for the current
// owner. The completion signal is only a wake-up hint; it is never returned as
// the durable submission result.
type SubmitCoalescerConfig struct {
	WaitTimeout  time.Duration
	PollInterval time.Duration
	SignalTTL    time.Duration
}

func DefaultSubmitCoalescerConfig() SubmitCoalescerConfig {
	return SubmitCoalescerConfig{
		WaitTimeout:  defaultSubmitWaitTimeout,
		PollInterval: defaultSubmitPollInterval,
		SignalTTL:    defaultSubmitSignalTTL,
	}
}

func (c SubmitCoalescerConfig) normalized() SubmitCoalescerConfig {
	defaults := DefaultSubmitCoalescerConfig()
	if c.WaitTimeout <= 0 {
		c.WaitTimeout = defaults.WaitTimeout
	}
	if c.PollInterval <= 0 {
		c.PollInterval = defaults.PollInterval
	}
	if c.PollInterval > c.WaitTimeout {
		c.PollInterval = c.WaitTimeout
	}
	if c.SignalTTL <= 0 {
		c.SignalTTL = defaults.SignalTTL
	}
	return c
}

// SubmitCoalescer collapses the expensive part of concurrent, writer-scoped
// duplicate submissions across collection-server instances.
//
// The owner performs the normal durable submit while holding a Redis lease.
// Contenders wait for a short-lived completion signal and then execute the
// readback closure, which must consult the durable store. Redis failure and a
// stale lease degrade to that same durable path.
type SubmitCoalescer struct {
	opsHandle *redisruntime.Handle
	runner    locklease.Runner
	config    SubmitCoalescerConfig
	observer  resilience.Observer
}

func NewSubmitCoalescer(
	opsHandle *redisruntime.Handle,
	runner locklease.Runner,
	config SubmitCoalescerConfig,
) *SubmitCoalescer {
	return NewSubmitCoalescerWithObserver(opsHandle, runner, config, nil)
}

func NewSubmitCoalescerWithObserver(
	opsHandle *redisruntime.Handle,
	runner locklease.Runner,
	config SubmitCoalescerConfig,
	observer resilience.Observer,
) *SubmitCoalescer {
	return &SubmitCoalescer{
		opsHandle: opsHandle,
		runner:    runner,
		config:    config.normalized(),
		observer:  defaultObserver(observer),
	}
}

// Run chooses one lease owner and makes contenders wait before consulting the
// durable idempotency path. Both closures may use the same collection
// orchestration when it always performs explicit durable readback first and
// executes the full acceptance path only after a confirmed miss.
func (c *SubmitCoalescer) Run(
	ctx context.Context,
	key string,
	owner func(context.Context) (string, error),
	readback func(context.Context) (string, error),
) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if c == nil || key == "" || c.runner == nil {
		if c != nil {
			c.observe(ctx, resilience.OutcomeDegradedOpen)
		}
		return callSubmitPath(ctx, owner)
	}

	var (
		ownerValue       string
		ownerErr         error
		ownerCalled      bool
		decisionObserved bool
	)
	leaseStarted := time.Now()
	result, runErr := c.runner.Run(
		ctx,
		locklease.WorkloadCollectionSubmit,
		submitLeaseKey(key),
		defaultSubmitLeaseTTL,
		func(runCtx context.Context) error {
			decisionObserved = true
			resilience.ObserveAnswerSheetSubmitCoalescerRedis("lease_acquire", "acquired", time.Since(leaseStarted))
			ownerCalled = true
			ownerValue, ownerErr = callSubmitPath(runCtx, owner)
			if ownerErr == nil && ownerValue != "" {
				if signalErr := c.writeCompletionSignal(runCtx, key); signalErr != nil {
					if context.Cause(runCtx) == nil {
						c.observe(runCtx, resilience.OutcomeLockError)
						resilience.ObserveAnswerSheetSubmitCoalescer("signal_error")
					}
				}
			}
			return ownerErr
		},
	)

	if ownerCalled {
		c.observe(ctx, resilience.OutcomeLockAcquired)
		resilience.ObserveAnswerSheetSubmitCoalescer("owner")
		// Once the durable closure succeeded, Redis renewal/release failures
		// cannot invalidate that result. The lower LockLease metrics retain
		// release and renewal failures for operations.
		if ownerErr == nil {
			return ownerValue, nil
		}
		return ownerValue, ownerErr
	}

	if !decisionObserved {
		resultLabel := "contention"
		if runErr != nil {
			resultLabel = "error"
		}
		resilience.ObserveAnswerSheetSubmitCoalescerRedis("lease_acquire", resultLabel, time.Since(leaseStarted))
	}
	if runErr != nil {
		if cause := context.Cause(ctx); cause != nil {
			resilience.ObserveAnswerSheetSubmitCoalescer("canceled")
			return "", cause
		}
		c.observe(ctx, resilience.OutcomeDegradedOpen)
		resilience.ObserveAnswerSheetSubmitCoalescer("degraded_open")
		return callSubmitPath(ctx, owner)
	}
	if result.Acquired {
		// A Runner must execute the non-nil body after admission. Treat a
		// broken implementation as unavailable and keep Mongo as final truth.
		c.observe(ctx, resilience.OutcomeDegradedOpen)
		resilience.ObserveAnswerSheetSubmitCoalescer("degraded_open")
		return callSubmitPath(ctx, owner)
	}

	c.observe(ctx, resilience.OutcomeLockContention)
	waitStarted := time.Now()
	signaled, waitErr := c.waitForCompletionSignal(ctx, key)
	if waitErr != nil {
		if cause := context.Cause(ctx); cause != nil {
			resilience.ObserveAnswerSheetSubmitCoalescerWait("canceled", time.Since(waitStarted))
			resilience.ObserveAnswerSheetSubmitCoalescer("canceled")
			return "", cause
		}
		c.observe(ctx, resilience.OutcomeDegradedOpen)
		resilience.ObserveAnswerSheetSubmitCoalescerWait("redis_error", time.Since(waitStarted))
		resilience.ObserveAnswerSheetSubmitCoalescer("degraded_open")
	} else if signaled {
		resilience.ObserveAnswerSheetSubmitCoalescerWait("signaled", time.Since(waitStarted))
		resilience.ObserveAnswerSheetSubmitCoalescer("contender_signaled")
	} else {
		resilience.ObserveAnswerSheetSubmitCoalescerWait("timeout", time.Since(waitStarted))
		resilience.ObserveAnswerSheetSubmitCoalescer("contender_timeout")
	}

	value, err := callSubmitPath(ctx, firstNonNilSubmitPath(readback, owner))
	if err != nil {
		resilience.ObserveAnswerSheetSubmitCoalescer("readback_error")
		return value, err
	}
	resilience.ObserveAnswerSheetSubmitCoalescer("readback_ok")
	return value, nil
}

func (c *SubmitCoalescer) writeCompletionSignal(parent context.Context, key string) error {
	if c == nil || c.opsHandle == nil || c.opsHandle.Client == nil {
		return errSubmitSignalUnavailable
	}
	writeCtx, cancel := context.WithTimeout(parent, submitSignalWriteTimeout)
	defer cancel()

	started := time.Now()
	err := c.opsHandle.Client.Set(
		writeCtx,
		c.completionSignalKey(key),
		submitCompletionSignalValue,
		c.config.SignalTTL,
	).Err()
	outcome := "ok"
	if err != nil {
		outcome = "error"
		if context.Cause(writeCtx) != nil {
			outcome = "canceled"
		}
	}
	resilience.ObserveAnswerSheetSubmitCoalescerRedis("signal_write", outcome, time.Since(started))
	return err
}

func (c *SubmitCoalescer) waitForCompletionSignal(ctx context.Context, key string) (bool, error) {
	if c == nil || c.opsHandle == nil || c.opsHandle.Client == nil {
		return false, errSubmitSignalUnavailable
	}
	waitTimeout := c.boundedWaitTimeout(ctx)
	if waitTimeout <= 0 {
		return false, nil
	}
	waitCtx, cancel := context.WithTimeout(ctx, waitTimeout)
	defer cancel()

	ticker := time.NewTicker(c.config.PollInterval)
	defer ticker.Stop()
	for {
		signaled, err := c.readCompletionSignal(waitCtx, key)
		if err != nil {
			if cause := context.Cause(ctx); cause != nil {
				return false, cause
			}
			if context.Cause(waitCtx) != nil {
				return false, nil
			}
			return false, err
		}
		if signaled {
			return true, nil
		}
		select {
		case <-waitCtx.Done():
			if cause := context.Cause(ctx); cause != nil {
				return false, cause
			}
			return false, nil
		case <-ticker.C:
		}
	}
}

func (c *SubmitCoalescer) readCompletionSignal(ctx context.Context, key string) (bool, error) {
	started := time.Now()
	_, err := c.opsHandle.Client.Get(ctx, c.completionSignalKey(key)).Result()
	outcome := "hit"
	switch {
	case err == nil:
	case errors.Is(err, redis.Nil):
		outcome = "miss"
		err = nil
	default:
		outcome = "error"
	}
	resilience.ObserveAnswerSheetSubmitCoalescerRedis("signal_read", outcome, time.Since(started))
	return outcome == "hit", err
}

func (c *SubmitCoalescer) boundedWaitTimeout(ctx context.Context) time.Duration {
	waitTimeout := c.config.WaitTimeout
	deadline, ok := ctx.Deadline()
	if !ok {
		return waitTimeout
	}
	remaining := time.Until(deadline)
	if remaining <= c.config.PollInterval {
		return 0
	}
	// Preserve most of a nearly exhausted request budget for the mandatory
	// durable readback rather than spending it all waiting on Redis.
	if waitTimeout*2 > remaining {
		waitTimeout = remaining / 3
	}
	if waitTimeout < c.config.PollInterval {
		return 0
	}
	return waitTimeout
}

func (c *SubmitCoalescer) completionSignalKey(raw string) string {
	namespace := ""
	if c != nil && c.opsHandle != nil {
		namespace = c.opsHandle.Namespace
		if namespace == "" && c.opsHandle.Builder != nil {
			namespace = c.opsHandle.Builder.Namespace()
		}
	}
	return keyspace.NewOpsKeyspace(namespace).SubmitCompletionSignal(raw)
}

func submitLeaseKey(key string) string {
	return "submit:idempotency:" + key + ":lock"
}

func callSubmitPath(
	ctx context.Context,
	path func(context.Context) (string, error),
) (string, error) {
	if path == nil {
		return "", nil
	}
	return path(ctx)
}

func firstNonNilSubmitPath(
	primary func(context.Context) (string, error),
	fallback func(context.Context) (string, error),
) func(context.Context) (string, error) {
	if primary != nil {
		return primary
	}
	return fallback
}

func (c *SubmitCoalescer) observe(ctx context.Context, outcome resilience.Outcome) {
	observer := resilience.DefaultObserver()
	if c != nil && c.observer != nil {
		observer = c.observer
	}
	resilience.Observe(ctx, observer, resilience.ProtectionDuplicateSuppression, resilience.Subject{
		Component: "collection-server",
		Scope:     "answersheet_submit",
		Resource:  "submit_coalescer",
		Strategy:  "redis_lease_durable_readback",
	}, outcome)
}

func defaultObserver(observer resilience.Observer) resilience.Observer {
	if observer != nil {
		return observer
	}
	return resilience.DefaultObserver()
}
