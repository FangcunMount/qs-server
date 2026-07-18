// Package retrygovernance owns provider-neutral retry decisions shared by
// business execution, durable outbox publishing, and operations projections.
package retrygovernance

import (
	"fmt"
	"hash/fnv"
	"strconv"
	"sync/atomic"
	"time"
)

// Disposition describes who, if anyone, may schedule the next attempt.
type Disposition string

const (
	DispositionAutomatic      Disposition = "automatic"
	DispositionManualRequired Disposition = "manual_required"
	DispositionTerminal       Disposition = "terminal"
)

func (d Disposition) IsValid() bool {
	switch d {
	case DispositionAutomatic, DispositionManualRequired, DispositionTerminal:
		return true
	default:
		return false
	}
}

// AttemptOrigin records why an execution attempt was admitted.
type AttemptOrigin string

const (
	AttemptOriginInitial       AttemptOrigin = "initial"
	AttemptOriginAutomatic     AttemptOrigin = "automatic"
	AttemptOriginManual        AttemptOrigin = "manual"
	AttemptOriginForce         AttemptOrigin = "force"
	AttemptOriginLeaseRecovery AttemptOrigin = "lease_recovery"
)

func (o AttemptOrigin) IsValid() bool {
	switch o {
	case AttemptOriginInitial, AttemptOriginAutomatic, AttemptOriginManual, AttemptOriginForce, AttemptOriginLeaseRecovery:
		return true
	default:
		return false
	}
}

// Decision is the durable result of classifying one failed attempt.
type Decision struct {
	Disposition                Disposition
	Attempt                    int
	MaxAutomaticAttempts       int
	RemainingAutomaticAttempts int
	NextAttemptAt              *time.Time
	PolicyVersion              string
	RetryEventID               string
	ActionRequestID            string
}

// Policy bounds automatic attempts and calculates their deterministic base
// delay. Provider adapters may add bounded jitter when scheduling the event.
type Policy struct {
	Version              string
	MaxAutomaticAttempts int
	BaseDelay            time.Duration
	MaxDelay             time.Duration
	JitterFraction       float64
}

// DefaultBusinessPolicy is the v1 Evaluation and Interpretation policy.
var DefaultBusinessPolicy = Policy{
	Version:              "business-retry/v1",
	MaxAutomaticAttempts: 3,
	BaseDelay:            30 * time.Second,
	MaxDelay:             5 * time.Minute,
}

// DefaultOutboxPolicy bounds automatic durable publish attempts.
var DefaultOutboxPolicy = Policy{
	Version:              "outbox-publish-retry/v1",
	MaxAutomaticAttempts: 30,
	BaseDelay:            10 * time.Second,
	MaxDelay:             time.Hour,
	JitterFraction:       0.20,
}

var businessPolicy atomic.Pointer[Policy]
var outboxPolicy atomic.Pointer[Policy]

func init() {
	setPolicy(&businessPolicy, DefaultBusinessPolicy)
	setPolicy(&outboxPolicy, DefaultOutboxPolicy)
}

// BusinessPolicy returns the immutable process policy snapshot used for new
// Evaluation and Interpretation decisions.
func BusinessPolicy() Policy { return *businessPolicy.Load() }

// OutboxPolicy returns the immutable process policy snapshot used by relays.
func OutboxPolicy() Policy { return *outboxPolicy.Load() }

// ConfigurePolicies atomically replaces both policy snapshots during process
// bootstrap. Persisted decisions retain their policy version and max-attempt
// snapshot, so a later configuration change cannot rewrite history.
func ConfigurePolicies(business, outbox Policy) error {
	if err := business.Validate(); err != nil {
		return fmt.Errorf("business retry policy: %w", err)
	}
	if err := outbox.Validate(); err != nil {
		return fmt.Errorf("outbox retry policy: %w", err)
	}
	setPolicy(&businessPolicy, business)
	setPolicy(&outboxPolicy, outbox)
	return nil
}

func setPolicy(target *atomic.Pointer[Policy], value Policy) {
	copy := value
	target.Store(&copy)
}

func (p Policy) Validate() error {
	if p.Version == "" {
		return fmt.Errorf("retry policy version is required")
	}
	if p.MaxAutomaticAttempts < 1 {
		return fmt.Errorf("retry max automatic attempts must be positive")
	}
	if p.BaseDelay <= 0 {
		return fmt.Errorf("retry base delay must be positive")
	}
	if p.MaxDelay < p.BaseDelay {
		return fmt.Errorf("retry max delay must be greater than or equal to base delay")
	}
	if p.JitterFraction < 0 || p.JitterFraction > 1 {
		return fmt.Errorf("retry jitter fraction must be between 0 and 1")
	}
	return nil
}

// DecideFailure classifies one completed failed attempt. Invalid attempt
// numbers are treated as already exhausted so malformed legacy data cannot
// create an unbounded automatic retry loop.
func (p Policy) DecideFailure(retryable bool, attempt int, now time.Time) Decision {
	return p.DecideFailureForKey(retryable, attempt, now, "")
}

func (p Policy) DecideFailureForKey(retryable bool, attempt int, now time.Time, key string) Decision {
	remaining := p.MaxAutomaticAttempts - attempt
	if remaining < 0 {
		remaining = 0
	}
	decision := Decision{
		Attempt:                    attempt,
		MaxAutomaticAttempts:       p.MaxAutomaticAttempts,
		RemainingAutomaticAttempts: remaining,
		PolicyVersion:              p.Version,
	}
	if !retryable {
		decision.Disposition = DispositionTerminal
		return decision
	}
	if attempt < 1 || attempt >= p.MaxAutomaticAttempts {
		decision.Disposition = DispositionManualRequired
		return decision
	}
	delay := p.jitteredDelay(p.delayAfter(attempt), key, attempt)
	next := now.Add(delay)
	decision.Disposition = DispositionAutomatic
	decision.NextAttemptAt = &next
	return decision
}

func (p Policy) jitteredDelay(delay time.Duration, key string, attempt int) time.Duration {
	if p.JitterFraction <= 0 || delay <= 0 {
		return delay
	}
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(key + ":" + strconv.Itoa(attempt)))
	unit := float64(hash.Sum32()%10001)/5000 - 1
	return delay + time.Duration(float64(delay)*p.JitterFraction*unit)
}

func (p Policy) delayAfter(attempt int) time.Duration {
	delay := p.BaseDelay
	for step := 1; step < attempt && delay < p.MaxDelay; step++ {
		if delay > p.MaxDelay/2 {
			return p.MaxDelay
		}
		delay *= 2
	}
	if delay > p.MaxDelay {
		return p.MaxDelay
	}
	return delay
}
