package redisadapter

import (
	"context"
	"fmt"
	"strings"
	"time"

	baseredisadapter "github.com/FangcunMount/component-base/pkg/locklease/redisadapter"
	cacheobserve "github.com/FangcunMount/qs-server/internal/pkg/cache/observe"
	lockkeyspace "github.com/FangcunMount/qs-server/internal/pkg/locklease/keyspace"
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
		m.observe(ctx, identity, resilienceplane.OutcomeLockError)
		return nil, false, err
	}
	if m == nil || m.handle == nil || m.handle.Client == nil {
		observability.ObserveLockDegraded(lockName, "redis_unavailable")
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
		observability.ObserveLockAcquire(lockName, "error")
		m.observe(ctx, identity, resilienceplane.OutcomeLockError)
		m.observeFamilyFailure(err)
		return nil, false, err
	}
	if !acquired {
		observability.ObserveLockAcquire(lockName, "contention")
		m.observe(ctx, identity, resilienceplane.OutcomeLockContention)
		m.observeFamilySuccess()
		return nil, false, nil
	}
	observability.ObserveLockAcquire(lockName, "ok")
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

// Release 释放一个已经获取到的锁租约。
func (m *Manager) Release(ctx context.Context, identity Identity, lease *Lease) error {
	lockName := m.metricName(identity)
	if m == nil || m.handle == nil || m.handle.Client == nil || lease == nil || lease.Token == "" {
		return nil
	}
	if err := baseredisadapter.NewManager(m.handle.Client, nil).Release(ctx, identity, lease); err != nil {
		observability.ObserveLockRelease(lockName, "error")
		m.observe(ctx, identity, resilienceplane.OutcomeLockError)
		m.observeFamilyFailure(err)
		return err
	}
	observability.ObserveLockRelease(lockName, "ok")
	m.observe(ctx, identity, resilienceplane.OutcomeLockReleased)
	m.observeFamilySuccess()
	return nil
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
