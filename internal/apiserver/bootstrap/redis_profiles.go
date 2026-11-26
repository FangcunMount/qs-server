package bootstrap

import (
	"context"
	"errors"
	"maps"
	"slices"

	"github.com/FangcunMount/component-base/pkg/database"
	redisruntime "github.com/FangcunMount/component-base/pkg/redis/runtime"
	redis "github.com/redis/go-redis/v9"
)

// Give each pool its own close result while keeping the shared registry's
// named-profile connection and retry behavior.
type apiRedisProfiles struct {
	defaultPool *database.NamedRedisRegistry
	named       map[string]*database.NamedRedisRegistry
	names       []string
}

func newAPIRedisProfiles(defaultConfig *database.RedisConfig, configs map[string]*database.RedisConfig) *apiRedisProfiles {
	p := &apiRedisProfiles{
		defaultPool: database.NewNamedRedisRegistry(defaultConfig, nil),
		named:       make(map[string]*database.NamedRedisRegistry, len(configs)),
	}
	for name, cfg := range configs {
		if name == "" || cfg == nil {
			continue
		}
		p.named[name] = database.NewNamedRedisRegistry(nil, map[string]*database.RedisConfig{
			name: redisruntime.MergeConfig(defaultConfig, cfg),
		})
	}
	p.names = slices.Sorted(maps.Keys(p.named))
	return p
}

func (p *apiRedisProfiles) Connect() error {
	if err := p.defaultPool.Connect(); err != nil {
		return err
	}
	for _, name := range p.names {
		if err := p.named[name].Connect(); err != nil {
			return err
		}
	}
	return nil
}

func (p *apiRedisProfiles) HealthCheck(ctx context.Context) error {
	if err := p.defaultPool.HealthCheck(ctx); err != nil {
		return err
	}
	for _, name := range p.names {
		if err := p.named[name].HealthCheck(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (p *apiRedisProfiles) GetClient(name string) (redis.UniversalClient, error) {
	if pool := p.named[name]; pool != nil {
		return pool.GetClient(name)
	}
	return p.defaultPool.GetClient(name)
}

func (p *apiRedisProfiles) ProfileStatus(name string) database.RedisProfileStatus {
	return p.named[name].ProfileStatus(name)
}

func (p *apiRedisProfiles) Close(ctx context.Context) error {
	var failures []error
	if p.defaultPool.HasConnections() {
		failures = append(failures, closeDatabaseResource(ctx, "redis/default", p.defaultPool.Close))
	}
	for _, name := range p.names {
		failures = append(failures, closeDatabaseResource(ctx, "redis/profile/"+name, p.named[name].Close))
	}
	return errors.Join(failures...)
}
