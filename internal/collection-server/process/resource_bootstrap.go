package process

import (
	"context"

	bootstrap "github.com/FangcunMount/qs-server/internal/collection-server/bootstrap"
	"github.com/FangcunMount/qs-server/internal/pkg/redisbootstrap"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
)

func (s *server) prepareResources() (resourceOutput, error) {
	dbManager := bootstrap.NewDatabaseManager(s.config)
	if err := dbManager.Initialize(); err != nil {
		return resourceOutput{}, err
	}

	redisRuntime := redisbootstrap.BuildRuntime(context.Background(), redisbootstrap.Options{
		Component:      "collection-server",
		RuntimeOptions: s.config.RedisRuntime,
		Resolver:       dbManager,
		Defaults: map[redisplane.Family]redisplane.Route{
			redisplane.FamilyOps: {
				RedisProfile:         "ops_runtime",
				NamespaceSuffix:      "ops:runtime",
				AllowFallbackDefault: true,
			},
			redisplane.FamilyLock: {
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
			opsHandle:    redisRuntime.Handle(redisplane.FamilyOps),
			lockHandle:   redisRuntime.Handle(redisplane.FamilyLock),
			lockManager:  redisRuntime.LockManager,
		},
	}, nil
}
