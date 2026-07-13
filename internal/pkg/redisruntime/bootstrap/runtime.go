package cacheplanebootstrap

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease/redisadapter"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	redis "github.com/redis/go-redis/v9"
)

const defaultLockName = "lock_lease"

// Options describes the shared Redis runtime bootstrap inputs used by qs-server processes.
type Options struct {
	Component      string
	RuntimeOptions *genericoptions.RedisRuntimeOptions
	Defaults       map[redisruntime.Family]redisruntime.Route
	Resolver       redisruntime.Resolver
	LockName       string
}

// RuntimeBundle is the process-local Redis runtime output shared by cache and lock consumers.
type RuntimeBundle struct {
	Component      string
	StatusRegistry *observability.FamilyStatusRegistry
	Runtime        *redisruntime.Runtime
	Handles        map[redisruntime.Family]*redisruntime.Handle
	LockManager    locklease.Manager
}

// BuildRuntime builds the family-scoped Redis runtime, pre-resolved handles, and lock manager.
func BuildRuntime(ctx context.Context, opts Options) *RuntimeBundle {
	component := opts.Component
	statusRegistry := observability.NewFamilyStatusRegistry(component)
	runtime := redisruntime.NewRuntime(
		component,
		opts.Resolver,
		redisruntime.CatalogFromOptions(opts.RuntimeOptions, opts.Defaults),
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
		LockManager: redisadapter.NewManagerWithObservers(
			component,
			lockName,
			runtime.Handle(ctx, redisruntime.FamilyLock),
			nil,
			observability.NewComponentObserver(component, statusRegistry),
		),
	}
}

func (b *RuntimeBundle) Handle(family redisruntime.Family) *redisruntime.Handle {
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

func (b *RuntimeBundle) Client(family redisruntime.Family) redis.UniversalClient {
	handle := b.Handle(family)
	if handle == nil {
		return nil
	}
	return handle.Client
}

func (b *RuntimeBundle) Builder(family redisruntime.Family) *keyspace.Builder {
	handle := b.Handle(family)
	if handle == nil {
		return nil
	}
	return handle.Builder
}
