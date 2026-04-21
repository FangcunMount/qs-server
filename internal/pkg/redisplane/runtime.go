package redisplane

import (
	"context"
	"fmt"
	"sync"

	cbdatabase "github.com/FangcunMount/component-base/pkg/database"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	redis "github.com/redis/go-redis/v9"
)

// Resolver is the minimal redis profile resolver required by qs-server runtime routing.
type Resolver interface {
	GetRedisClient() (redis.UniversalClient, error)
	GetRedisClientByProfile(profile string) (redis.UniversalClient, error)
	GetRedisProfileStatus(profile string) cbdatabase.RedisProfileStatus
}

// Handle is the resolved runtime view for one logical redis family.
type Handle struct {
	Family               Family
	Profile              string
	Namespace            string
	NamespaceSuffix      string
	Builder              *rediskey.Builder
	Client               redis.UniversalClient
	AllowWarmup          bool
	AllowFallbackDefault bool
	Configured           bool
	Available            bool
	Degraded             bool
	Mode                 string
	LastError            error
}

// Runtime resolves and caches runtime family handles.
type Runtime struct {
	component string
	resolver  Resolver
	catalog   *Catalog
	status    *cacheobservability.FamilyStatusRegistry

	mu      sync.RWMutex
	handles map[Family]*Handle
}

// NewRuntime creates a shared qs-server redis runtime for one component.
func NewRuntime(component string, resolver Resolver, catalog *Catalog, status *cacheobservability.FamilyStatusRegistry) *Runtime {
	return &Runtime{
		component: component,
		resolver:  resolver,
		catalog:   catalog,
		status:    status,
		handles:   map[Family]*Handle{},
	}
}

// Handle resolves one family and memoizes the result for the process lifetime.
func (r *Runtime) Handle(ctx context.Context, family Family) *Handle {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	if handle, ok := r.handles[family]; ok {
		r.mu.RUnlock()
		return handle
	}
	r.mu.RUnlock()

	handle := r.resolve(ctx, family)

	r.mu.Lock()
	r.handles[family] = handle
	r.mu.Unlock()

	return handle
}

// ResolveAll resolves every explicit family in the catalog.
func (r *Runtime) ResolveAll(ctx context.Context) map[Family]*Handle {
	results := make(map[Family]*Handle)
	if r == nil || r.catalog == nil {
		return results
	}
	for _, family := range r.catalog.Families() {
		results[family] = r.Handle(ctx, family)
	}
	return results
}

func (r *Runtime) resolve(ctx context.Context, family Family) *Handle {
	route := Route{}
	namespace := ""
	builder := rediskey.NewBuilder()
	if r.catalog != nil {
		route = r.catalog.Route(family)
		namespace = r.catalog.Namespace(family)
		builder = r.catalog.Builder(family)
	}

	handle := &Handle{
		Family:               family,
		Profile:              route.RedisProfile,
		Namespace:            namespace,
		NamespaceSuffix:      route.NamespaceSuffix,
		Builder:              builder,
		AllowWarmup:          route.AllowWarmup,
		AllowFallbackDefault: route.AllowFallbackDefault,
	}

	if r.resolver == nil {
		handle.Degraded = true
		handle.Mode = cacheobservability.FamilyModeDegraded
		handle.LastError = fmt.Errorf("redis resolver is nil")
		r.updateStatus(handle)
		return handle
	}

	if route.RedisProfile == "" {
		client, err := r.resolver.GetRedisClient()
		handle.Configured = true
		handle.Client = client
		handle.Available = client != nil && err == nil
		handle.Degraded = !handle.Available
		if handle.Available {
			handle.Mode = cacheobservability.FamilyModeDefault
		} else {
			handle.Mode = cacheobservability.FamilyModeDegraded
			handle.LastError = err
		}
		r.updateStatus(handle)
		return handle
	}

	status := r.resolver.GetRedisProfileStatus(route.RedisProfile)
	switch status.State {
	case cbdatabase.RedisProfileStateMissing:
		handle.Configured = false
		if route.AllowFallbackDefault {
			client, err := r.resolver.GetRedisClient()
			handle.Client = client
			handle.Available = client != nil && err == nil
			handle.Degraded = !handle.Available
			if handle.Available {
				handle.Mode = cacheobservability.FamilyModeFallbackDefault
			} else {
				handle.Mode = cacheobservability.FamilyModeDegraded
				handle.LastError = err
			}
		} else {
			handle.Degraded = true
			handle.Mode = cacheobservability.FamilyModeDegraded
			handle.LastError = fmt.Errorf("redis profile %q is missing", route.RedisProfile)
		}
	case cbdatabase.RedisProfileStateUnavailable:
		handle.Configured = true
		handle.Degraded = true
		handle.Mode = cacheobservability.FamilyModeDegraded
		handle.LastError = status.Err
	default:
		client, err := r.resolver.GetRedisClientByProfile(route.RedisProfile)
		handle.Configured = true
		handle.Client = client
		handle.Available = client != nil && err == nil
		handle.Degraded = !handle.Available
		if handle.Available {
			handle.Mode = cacheobservability.FamilyModeNamedProfile
		} else {
			handle.Mode = cacheobservability.FamilyModeDegraded
			handle.LastError = err
		}
	}

	r.updateStatus(handle)
	return handle
}

func (r *Runtime) updateStatus(handle *Handle) {
	if r == nil || r.status == nil || handle == nil {
		return
	}
	r.status.Update(cacheobservability.FamilyStatus{
		Component:   r.component,
		Family:      string(handle.Family),
		Profile:     handle.Profile,
		Namespace:   handle.Namespace,
		AllowWarmup: handle.AllowWarmup,
		Configured:  handle.Configured,
		Available:   handle.Available,
		Degraded:    handle.Degraded,
		Mode:        handle.Mode,
		LastError:   errorString(handle.LastError),
	})
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
