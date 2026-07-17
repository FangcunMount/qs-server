package redisadapter

import (
	"context"
	"fmt"
	"strings"
	"time"

	baseredisadapter "github.com/FangcunMount/component-base/pkg/locklease/redisadapter"
	cacheobserve "github.com/FangcunMount/qs-server/internal/pkg/cache/observe"
	lockkeyspace "github.com/FangcunMount/qs-server/internal/pkg/locklease/keyspace"
	lockobserve "github.com/FangcunMount/qs-server/internal/pkg/locklease/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
)

// Manager 是构建在 cache runtime 之上的 Redis 锁租约 adapter。
type Manager struct {
	component string
	name      string
	handle    *redisruntime.Handle
	observer  resilienceplane.Observer
	family    cacheobserve.FamilyObserver
}

// NewManager 为一个进程级锁工作负载创建分布式锁管理器。
func NewManager(component, name string, handle *redisruntime.Handle) *Manager {
	return NewManagerWithObserver(component, name, handle, nil)
}

// NewManagerWithObserver 创建带显式 resilience observer 的锁管理器。
func NewManagerWithObserver(component, name string, handle *redisruntime.Handle, observer resilienceplane.Observer) *Manager {
	return NewManagerWithObservers(component, name, handle, observer, nil)
}

// NewManagerWithObservers creates a lock manager with explicit resilience and
// Redis-family health observers.
func NewManagerWithObservers(component, name string, handle *redisruntime.Handle, observer resilienceplane.Observer, family cacheobserve.FamilyObserver) *Manager {
	return &Manager{
		component: component,
		name:      name,
		handle:    handle,
		observer:  defaultObserver(observer),
		family:    family,
	}
}

// Acquire 尝试获取一个锁租约。
func (m *Manager) Acquire(ctx context.Context, identity Identity, ttl time.Duration) (*Lease, bool, error) {
	lockName := m.metricName(identity)
	key, err := m.lockKey(identity)
	if err != nil {
		observability.ObserveLockAcquire(lockName, "error")
		lockobserve.ObserveOperation(m.componentName(), lockName, "acquire", "error")
		m.observe(ctx, identity, resilienceplane.OutcomeLockError)
		return nil, false, err
	}
	if cause := cancellationCause(ctx); cause != nil {
		m.observeCanceled("acquire", lockName)
		return nil, false, cause
	}
	if m == nil || m.handle == nil || m.handle.Client == nil {
		observability.ObserveLockDegraded(lockName, "redis_unavailable")
		lockobserve.ObserveOperation(m.componentName(), lockName, "acquire", "unavailable")
		m.observe(ctx, identity, resilienceplane.OutcomeLockDegraded)
		err := fmt.Errorf("lock redis handle is unavailable")
		m.observeFamilyFailure(err)
		return nil, false, err
	}
	lease, acquired, err := baseredisadapter.NewManager(m.handle.Client, nil).Acquire(ctx, Identity{
		Name: identity.Name,
		Key:  key,
	}, ttl)
	if err != nil {
		if cause := cancellationCause(ctx); cause != nil {
			m.observeCanceled("acquire", lockName)
			return nil, false, cause
		}
		observability.ObserveLockAcquire(lockName, "error")
		lockobserve.ObserveOperation(m.componentName(), lockName, "acquire", "error")
		m.observe(ctx, identity, resilienceplane.OutcomeLockError)
		m.observeFamilyFailure(err)
		return nil, false, err
	}
	if !acquired {
		observability.ObserveLockAcquire(lockName, "contention")
		lockobserve.ObserveOperation(m.componentName(), lockName, "acquire", "contention")
		m.observe(ctx, identity, resilienceplane.OutcomeLockContention)
		m.observeFamilySuccess()
		return nil, false, nil
	}
	observability.ObserveLockAcquire(lockName, "ok")
	lockobserve.ObserveOperation(m.componentName(), lockName, "acquire", "ok")
	m.observe(ctx, identity, resilienceplane.OutcomeLockAcquired)
	m.observeFamilySuccess()
	return lease, true, nil
}

// AcquireSpec 按锁规格获取锁租约。
func (m *Manager) AcquireSpec(ctx context.Context, spec Spec, key string, ttlOverride ...time.Duration) (*Lease, bool, error) {
	ttl := spec.DefaultTTL
	if len(ttlOverride) > 0 && ttlOverride[0] > 0 {
		ttl = ttlOverride[0]
	}
	if spec.Name == "" {
		return nil, false, fmt.Errorf("lock spec name is empty")
	}
	if ttl <= 0 {
		return nil, false, fmt.Errorf("lock spec ttl must be greater than 0")
	}
	return m.Acquire(ctx, spec.Identity(key), ttl)
}

// RenewSpec 按锁规格以 compare-and-expire 语义续租当前租约。
func (m *Manager) RenewSpec(ctx context.Context, spec Spec, key string, lease *Lease, ttlOverride ...time.Duration) (bool, error) {
	lockName := m.metricName(spec.Identity(key))
	ttl := spec.DefaultTTL
	if len(ttlOverride) > 0 && ttlOverride[0] > 0 {
		ttl = ttlOverride[0]
	}
	if spec.Name == "" {
		lockobserve.ObserveOperation(m.componentName(), lockName, "renew", "error")
		return false, fmt.Errorf("lock spec name is empty")
	}
	if ttl <= 0 {
		lockobserve.ObserveOperation(m.componentName(), lockName, "renew", "error")
		return false, fmt.Errorf("lock spec ttl must be greater than 0")
	}
	if cause := cancellationCause(ctx); cause != nil {
		m.observeCanceled("renew", lockName)
		return false, cause
	}
	if m == nil || m.handle == nil || m.handle.Client == nil {
		lockobserve.ObserveOperation(m.componentName(), lockName, "renew", "unavailable")
		m.observe(ctx, spec.Identity(key), resilienceplane.OutcomeLockRenewError)
		err := fmt.Errorf("lock redis handle is unavailable")
		m.observeFamilyFailure(err)
		return false, err
	}

	owned, err := baseredisadapter.NewManager(m.handle.Client, nil).RenewSpec(ctx, spec, key, lease, ttl)
	if err != nil {
		if cause := cancellationCause(ctx); cause != nil {
			m.observeCanceled("renew", lockName)
			return false, cause
		}
		lockobserve.ObserveOperation(m.componentName(), lockName, "renew", "error")
		m.observe(ctx, spec.Identity(key), resilienceplane.OutcomeLockRenewError)
		m.observeFamilyFailure(err)
		return false, err
	}
	if !owned {
		lockobserve.ObserveOperation(m.componentName(), lockName, "renew", "lost")
		m.observe(ctx, spec.Identity(key), resilienceplane.OutcomeLockLost)
		m.observeFamilySuccess()
		return false, nil
	}
	lockobserve.ObserveOperation(m.componentName(), lockName, "renew", "ok")
	m.observe(ctx, spec.Identity(key), resilienceplane.OutcomeLockRenewed)
	m.observeFamilySuccess()
	return true, nil
}

// Release 释放一个已经获取到的锁租约。
func (m *Manager) Release(ctx context.Context, identity Identity, lease *Lease) error {
	lockName := m.metricName(identity)
	if m == nil || m.handle == nil || m.handle.Client == nil || lease == nil || lease.Token == "" {
		return nil
	}
	if cause := cancellationCause(ctx); cause != nil {
		m.observeCanceled("release", lockName)
		return cause
	}
	if err := baseredisadapter.NewManager(m.handle.Client, nil).Release(ctx, identity, lease); err != nil {
		if cause := cancellationCause(ctx); cause != nil {
			m.observeCanceled("release", lockName)
			return cause
		}
		observability.ObserveLockRelease(lockName, "error")
		lockobserve.ObserveOperation(m.componentName(), lockName, "release", "error")
		m.observe(ctx, identity, resilienceplane.OutcomeLockError)
		m.observeFamilyFailure(err)
		return err
	}
	observability.ObserveLockRelease(lockName, "ok")
	lockobserve.ObserveOperation(m.componentName(), lockName, "release", "ok")
	m.observe(ctx, identity, resilienceplane.OutcomeLockReleased)
	m.observeFamilySuccess()
	return nil
}

func cancellationCause(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return context.Cause(ctx)
}

func (m *Manager) observeCanceled(operation, lockName string) {
	switch operation {
	case "acquire":
		observability.ObserveLockAcquire(lockName, "canceled")
	case "release":
		observability.ObserveLockRelease(lockName, "canceled")
	}
	lockobserve.ObserveOperation(m.componentName(), lockName, operation, "canceled")
}

func (m *Manager) observeFamilySuccess() {
	if m != nil && m.family != nil {
		m.family.ObserveFamilySuccess(string(redisruntime.FamilyLock))
	}
}

func (m *Manager) observeFamilyFailure(err error) {
	if m != nil && m.family != nil && err != nil {
		m.family.ObserveFamilyFailure(string(redisruntime.FamilyLock), err)
	}
}

// ReleaseSpec 按锁规格释放锁租约。
func (m *Manager) ReleaseSpec(ctx context.Context, spec Spec, key string, lease *Lease) error {
	if spec.Name == "" {
		return fmt.Errorf("lock spec name is empty")
	}
	return m.Release(ctx, spec.Identity(key), lease)
}

func (m *Manager) lockKey(identity Identity) (string, error) {
	if identity.Name == "" {
		return "", fmt.Errorf("lock identity name is empty")
	}
	if m == nil || m.handle == nil || m.handle.Builder == nil {
		return "", fmt.Errorf("lock handle is unavailable")
	}
	base := identity.Name
	if identity.Key != "" {
		base = identity.Key
	}
	return lockkeyspace.FromBuilder(m.handle.Builder).Lock(base), nil
}

func (m *Manager) metricName(identity Identity) string {
	base := strings.TrimSpace(identity.Name)
	if base == "" && m != nil {
		base = m.name
	}
	if base == "" {
		base = "lock"
	}
	return base
}

func (m *Manager) componentName() string {
	if m == nil || strings.TrimSpace(m.component) == "" {
		return "unknown"
	}
	return m.component
}

func (m *Manager) observe(ctx context.Context, identity Identity, outcome resilienceplane.Outcome) {
	component := ""
	observer := resilienceplane.DefaultObserver()
	if m != nil {
		component = m.component
		if m.observer != nil {
			observer = m.observer
		}
	}
	resilienceplane.Observe(ctx, observer, resilienceplane.ProtectionLock, resilienceplane.Subject{
		Component: component,
		Scope:     m.metricName(identity),
		Resource:  "redis_lock",
		Strategy:  "lease",
	}, outcome)
}

func defaultObserver(observer resilienceplane.Observer) resilienceplane.Observer {
	if observer != nil {
		return observer
	}
	return resilienceplane.DefaultObserver()
}
