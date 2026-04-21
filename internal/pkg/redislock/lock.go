package redislock

import (
	"context"
	"fmt"
	"strings"
	"time"

	rediskit "github.com/FangcunMount/component-base/pkg/redis"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
)

// Identity 描述一个具体的锁实例身份。
type Identity struct {
	Name string
	Key  string
}

// Lease 表示一次成功获取到的锁租约。
type Lease struct {
	Key   string
	Token string
}

// Manager 是构建在 Redis Foundation 之上的共享分布式锁管理器。
type Manager struct {
	component string
	name      string
	handle    *redisplane.Handle
}

// NewManager 为一个进程级锁工作负载创建分布式锁管理器。
func NewManager(component, name string, handle *redisplane.Handle) *Manager {
	return &Manager{
		component: component,
		name:      name,
		handle:    handle,
	}
}

// Acquire 尝试获取一个锁租约。
func (m *Manager) Acquire(ctx context.Context, identity Identity, ttl time.Duration) (*Lease, bool, error) {
	lockName := m.metricName(identity)
	key, err := m.lockKey(identity)
	if err != nil {
		cacheobservability.ObserveLockAcquire(lockName, "error")
		return nil, false, err
	}
	if m == nil || m.handle == nil || m.handle.Client == nil {
		cacheobservability.ObserveLockDegraded(lockName, "redis_unavailable")
		err := fmt.Errorf("lock redis handle is unavailable")
		cacheobservability.ObserveFamilyFailure(m.component, string(redisplane.FamilyLock), err)
		return nil, false, err
	}
	token, acquired, err := rediskit.AcquireLease(ctx, m.handle.Client, key, ttl)
	if err != nil {
		cacheobservability.ObserveLockAcquire(lockName, "error")
		cacheobservability.ObserveFamilyFailure(m.component, string(redisplane.FamilyLock), err)
		return nil, false, err
	}
	if !acquired {
		cacheobservability.ObserveLockAcquire(lockName, "contention")
		cacheobservability.ObserveFamilySuccess(m.component, string(redisplane.FamilyLock))
		return nil, false, nil
	}
	cacheobservability.ObserveLockAcquire(lockName, "ok")
	cacheobservability.ObserveFamilySuccess(m.component, string(redisplane.FamilyLock))
	return &Lease{Key: key, Token: token}, true, nil
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
	if err := rediskit.ReleaseLease(ctx, m.handle.Client, lease.Key, lease.Token); err != nil {
		cacheobservability.ObserveLockRelease(lockName, "error")
		cacheobservability.ObserveFamilyFailure(m.component, string(redisplane.FamilyLock), err)
		return err
	}
	cacheobservability.ObserveLockRelease(lockName, "ok")
	cacheobservability.ObserveFamilySuccess(m.component, string(redisplane.FamilyLock))
	return nil
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
	return m.handle.Builder.BuildLockKey(base), nil
}

func (m *Manager) metricName(identity Identity) string {
	base := strings.TrimSpace(identity.Name)
	if base == "" {
		base = m.name
	}
	if base == "" {
		base = "lock"
	}
	return base
}
