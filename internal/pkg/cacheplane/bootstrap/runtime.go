package cacheplanebootstrap

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease/redisadapter"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	redis "github.com/redis/go-redis/v9"
)

const defaultLockName = "lock_lease"

// Options describes the shared Redis runtime bootstrap inputs used by qs-server processes.
type Options struct {
	Component      string
	RuntimeOptions *genericoptions.RedisRuntimeOptions
	Defaults       map[cacheplane.Family]cacheplane.Route
	Resolver       cacheplane.Resolver
	LockName       string
}

// RuntimeBundle is the process-local Redis runtime output shared by cache and lock consumers.
type RuntimeBundle struct {
	Component      string
	StatusRegistry *observability.FamilyStatusRegistry
	Runtime        *cacheplane.Runtime
	Handles        map[cacheplane.Family]*cacheplane.Handle
	LockManager    locklease.Manager
}

// BuildRuntime builds the family-scoped Redis runtime, pre-resolved handles, and lock manager.
func BuildRuntime(ctx context.Context, opts Options) *RuntimeBundle {
	component := opts.Component
	statusRegistry := observability.NewFamilyStatusRegistry(component)
	runtime := cacheplane.NewRuntime(
		component,
		opts.Resolver,
		cacheplane.CatalogFromOptions(opts.RuntimeOptions, opts.Defaults),
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
		LockManager:    redisadapter.NewManager(component, lockName, runtime.Handle(ctx, cacheplane.FamilyLock)),
	}
}

func (b *RuntimeBundle) Handle(family cacheplane.Family) *cacheplane.Handle {
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

func (b *RuntimeBundle) Client(family cacheplane.Family) redis.UniversalClient {
	handle := b.Handle(family)
	if handle == nil {
		return nil
	}
	return handle.Client
}

func (b *RuntimeBundle) Builder(family cacheplane.Family) *keyspace.Builder {
	handle := b.Handle(family)
	if handle == nil {
		return nil
	}
	return handle.Builder
}
