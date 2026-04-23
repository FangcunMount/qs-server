package redisbootstrap

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/FangcunMount/qs-server/internal/pkg/redislock"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
	redis "github.com/redis/go-redis/v9"
)

const defaultLockName = "lock_lease"

// Options describes the shared Redis runtime bootstrap inputs used by qs-server processes.
type Options struct {
	Component      string
	RuntimeOptions *genericoptions.RedisRuntimeOptions
	Defaults       map[redisplane.Family]redisplane.Route
	Resolver       redisplane.Resolver
	LockName       string
}

// RuntimeBundle is the process-local Redis runtime output shared by cache and lock consumers.
type RuntimeBundle struct {
	Component      string
	StatusRegistry *cacheobservability.FamilyStatusRegistry
	Runtime        *redisplane.Runtime
	Handles        map[redisplane.Family]*redisplane.Handle
	LockManager    *redislock.Manager
}

// BuildRuntime builds the family-scoped Redis runtime, pre-resolved handles, and lock manager.
func BuildRuntime(ctx context.Context, opts Options) *RuntimeBundle {
	component := opts.Component
	statusRegistry := cacheobservability.NewFamilyStatusRegistry(component)
	runtime := redisplane.NewRuntime(
		component,
		opts.Resolver,
		redisplane.CatalogFromOptions(opts.RuntimeOptions, opts.Defaults),
		statusRegistry,
	)
	handles := runtime.ResolveAll(ctx)
	lockName := opts.LockName
	if lockName == "" {
		lockName = defaultLockName
	}

	return &RuntimeBundle{
		Component:      component,
		StatusRegistry: statusRegistry,
		Runtime:        runtime,
		Handles:        handles,
		LockManager:    redislock.NewManager(component, lockName, runtime.Handle(ctx, redisplane.FamilyLock)),
	}
}

func (b *RuntimeBundle) Handle(family redisplane.Family) *redisplane.Handle {
	if b == nil {
		return nil
	}
	if b.Handles != nil {
		if handle, ok := b.Handles[family]; ok {
			return handle
		}
	}
	if b.Runtime == nil {
		return nil
	}
	return b.Runtime.Handle(context.Background(), family)
}

func (b *RuntimeBundle) Client(family redisplane.Family) redis.UniversalClient {
	handle := b.Handle(family)
	if handle == nil {
		return nil
	}
	return handle.Client
}

func (b *RuntimeBundle) Builder(family redisplane.Family) *rediskey.Builder {
	handle := b.Handle(family)
	if handle == nil {
		return nil
	}
	return handle.Builder
}
