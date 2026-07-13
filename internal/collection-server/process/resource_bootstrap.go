package process

import (
	"context"

	bootstrap "github.com/FangcunMount/qs-server/internal/collection-server/bootstrap"
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
		LockName: "lock_lease",
	})
	return resourceOutput{
		handles: resourceHandles{
			dbManager: dbManager,
		},
		redisRuntime: redisRuntimeOutput{
			familyStatus: redisRuntime.StatusRegistry,
			redisRuntime: redisRuntime.Runtime,
			opsHandle:    redisRuntime.Handle(redisruntime.FamilyOps),
			lockHandle:   redisRuntime.Handle(redisruntime.FamilyLock),
			lockManager:  redisRuntime.LockManager,
		},
	}, nil
}
