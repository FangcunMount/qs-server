package process

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/log"
	bootstrap "github.com/FangcunMount/qs-server/internal/collection-server/bootstrap"
	locksubsystem "github.com/FangcunMount/qs-server/internal/pkg/locklease/subsystem"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/bootstrap"
)

func (s *server) prepareResources() (resourceOutput, error) {
	dbManager := bootstrap.NewDatabaseManager(s.config)
	if err := dbManager.Initialize(); err != nil {
		return resourceOutput{}, err
	}

	redisRuntime := cacheplanebootstrap.BuildRuntime(context.Background(), cacheplanebootstrap.Options{
		Component:      "collection-server",
		RuntimeOptions: s.config.RedisRuntime,
		Resolver:       dbManager,
		Defaults: map[redisruntime.Family]redisruntime.Route{
			redisruntime.FamilyOps: {
				RedisProfile:         "ops_runtime",
				NamespaceSuffix:      "ops:runtime",
				AllowFallbackDefault: true,
			},
			redisruntime.FamilyLock: {
				RedisProfile:         "lock_cache",
				NamespaceSuffix:      "cache:lock",
				AllowFallbackDefault: true,
			},
		},
	})
	renewalEnabled := s.config.LockLease != nil && s.config.LockLease.RenewalEnabled
	locks := locksubsystem.New(locksubsystem.Options{
		Component:      "collection-server",
		Handle:         redisRuntime.Handle(redisruntime.FamilyLock),
		StatusRegistry: redisRuntime.StatusRegistry,
		RenewalEnabled: renewalEnabled,
		Warn:           func(message string) { log.Warn(message) },
	})
	return resourceOutput{
		handles: resourceHandles{
			dbManager: dbManager,
		},
		redisRuntime: redisRuntimeOutput{
			familyStatus: redisRuntime.StatusRegistry,
			redisRuntime: redisRuntime.Runtime,
			opsHandle:    redisRuntime.Handle(redisruntime.FamilyOps),
			locks:        locks,
		},
	}, nil
}
