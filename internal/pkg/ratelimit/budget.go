package ratelimit

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

var ErrBudgetVersionConflict = errors.New("rate limit budget version conflict")

type BudgetID string

type BudgetPolicy struct {
	Global RateLimitPolicy `json:"global"`
	User   RateLimitPolicy `json:"user"`
}

func (p BudgetPolicy) Valid() bool { return p.Global.Valid() && p.User.Valid() }

type BudgetSnapshot struct {
	ID        BudgetID     `json:"id"`
	Version   uint64       `json:"version"`
	Policy    BudgetPolicy `json:"policy"`
	Source    string       `json:"source"`
	ExpiresAt time.Time    `json:"expires_at,omitempty"`
}

type LimiterFactory func(RateLimitPolicy) RateLimiter

type BudgetOptions struct {
	GlobalFactory          LimiterFactory
	UserFactory            LimiterFactory
	ConservativeTransition time.Duration
	Now                    func() time.Time
}

type RateBudgetProvider interface {
	Budget(BudgetID) (RateBudget, bool)
}

type RateBudget struct {
	Global RateLimiter
	User   RateLimiter
}

type Budget struct {
	id       BudgetID
	baseline BudgetPolicy
	opts     BudgetOptions
	mu       sync.Mutex
	state    atomic.Pointer[budgetState]
}

type budgetState struct {
	snapshot       BudgetSnapshot
	global         RateLimiter
	user           RateLimiter
	previousGlobal RateLimiter
	previousUser   RateLimiter
	transitionEnds time.Time
}

type budgetLimiter struct {
	budget *Budget
	user   bool
}

func NewBudget(id BudgetID, baseline BudgetPolicy, opts BudgetOptions) *Budget {
	if opts.GlobalFactory == nil {
		opts.GlobalFactory = func(policy RateLimitPolicy) RateLimiter { return NewLocalLimiter(policy) }
	}
	if opts.UserFactory == nil {
		opts.UserFactory = func(policy RateLimitPolicy) RateLimiter { return NewKeyedLocalLimiter(policy) }
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	b := &Budget{id: id, baseline: baseline, opts: opts}
	b.state.Store(b.buildState(1, baseline, "config", time.Time{}, nil))
	return b
}

func (b *Budget) Limiters() RateBudget {
	if b == nil {
		return RateBudget{}
	}
	return RateBudget{Global: budgetLimiter{budget: b}, User: budgetLimiter{budget: b, user: true}}
}

func (b *Budget) Snapshot() BudgetSnapshot {
	if b == nil {
		return BudgetSnapshot{}
	}
	b.expireIfNeeded()
	return b.state.Load().snapshot
}

func (b *Budget) Apply(expectedVersion uint64, policy BudgetPolicy, source string, ttl time.Duration) (BudgetSnapshot, error) {
	if b == nil || !policy.Valid() {
		return BudgetSnapshot{}, errors.New("invalid rate limit budget policy")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.expireLocked()
	current := b.state.Load()
	if current.snapshot.Version != expectedVersion {
		return current.snapshot, ErrBudgetVersionConflict
	}
	expiresAt := time.Time{}
	if ttl > 0 {
		expiresAt = b.opts.Now().Add(ttl)
	}
	next := b.buildState(expectedVersion+1, policy, source, expiresAt, current)
	b.state.Store(next)
	return next.snapshot, nil
}

func (b *Budget) Reset(expectedVersion uint64) (BudgetSnapshot, error) {
	if b == nil {
		return BudgetSnapshot{}, errors.New("rate limit budget is nil")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.expireLocked()
	current := b.state.Load()
	if current.snapshot.Version != expectedVersion {
		return current.snapshot, ErrBudgetVersionConflict
	}
	next := b.buildState(expectedVersion+1, b.baseline, "config", time.Time{}, current)
	b.state.Store(next)
	return next.snapshot, nil
}

// Reconcile installs an authoritative control-plane version. It is used by
// process agents after restart and never by request-path callers.
func (b *Budget) Reconcile(version uint64, policy BudgetPolicy, source string, expiresAt time.Time) (BudgetSnapshot, error) {
	if b == nil || version == 0 || !policy.Valid() {
		return BudgetSnapshot{}, errors.New("invalid reconciled rate limit budget")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	current := b.state.Load()
	if current != nil && version <= current.snapshot.Version {
		return current.snapshot, nil
	}
	next := b.buildState(version, policy, source, expiresAt, current)
	b.state.Store(next)
	return next.snapshot, nil
}

func (b *Budget) ReconcileBaseline(version uint64) (BudgetSnapshot, error) {
	if b == nil || version == 0 {
		return BudgetSnapshot{}, errors.New("invalid reconciled baseline version")
	}
	return b.Reconcile(version, b.baseline, "config", time.Time{})
}

func (b *Budget) buildState(version uint64, policy BudgetPolicy, source string, expiresAt time.Time, previous *budgetState) *budgetState {
	now := b.opts.Now()
	state := &budgetState{
		snapshot: BudgetSnapshot{ID: b.id, Version: version, Policy: policy, Source: source, ExpiresAt: expiresAt},
		global:   b.opts.GlobalFactory(policy.Global),
		user:     b.opts.UserFactory(policy.User),
	}
	if previous != nil && b.opts.ConservativeTransition > 0 {
		state.previousGlobal = previous.global
		state.previousUser = previous.user
		state.transitionEnds = now.Add(b.opts.ConservativeTransition)
	}
	return state
}

func (b *Budget) expireIfNeeded() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.expireLocked()
}

func (b *Budget) expireLocked() {
	current := b.state.Load()
	if current == nil || current.snapshot.ExpiresAt.IsZero() || b.opts.Now().Before(current.snapshot.ExpiresAt) {
		return
	}
	b.state.Store(b.buildState(current.snapshot.Version+1, b.baseline, "config", time.Time{}, current))
}

func (l budgetLimiter) Decide(ctx context.Context, key string) RateLimitDecision {
	if l.budget == nil {
		return limitedDecision(RateLimitPolicy{}, time.Second, 1)
	}
	l.budget.expireIfNeeded()
	state := l.budget.state.Load()
	current, previous := state.global, state.previousGlobal
	if l.user {
		current, previous = state.user, state.previousUser
	}
	decision := current.Decide(ctx, key)
	if !decision.Allowed || previous == nil || !l.budget.opts.Now().Before(state.transitionEnds) {
		return decision
	}
	previousDecision := previous.Decide(ctx, key)
	if previousDecision.Allowed {
		return decision
	}
	previousDecision.Subject = decision.Subject
	return previousDecision
}
