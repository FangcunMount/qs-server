package process

import (
	"context"

	bootstrap "github.com/FangcunMount/qs-server/internal/collection-server/bootstrap"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
	"github.com/FangcunMount/qs-server/internal/pkg/redislock"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
)

func (s *server) prepareResources() (resourceOutput, error) {
	dbManager := bootstrap.NewDatabaseManager(s.config)
	if err := dbManager.Initialize(); err != nil {
		return resourceOutput{}, err
	}

	familyStatus := cacheobservability.NewFamilyStatusRegistry("collection-server")
	redisRuntime := redisplane.NewRuntime(
		"collection-server",
		dbManager,
		redisplane.CatalogFromOptions(s.config.RedisRuntime, map[redisplane.Family]redisplane.Route{
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
		}),
		familyStatus,
	)
	lockHandle := redisRuntime.Handle(context.Background(), redisplane.FamilyLock)
	return resourceOutput{
		handles: resourceHandles{
			dbManager: dbManager,
		},
		redisRuntime: redisRuntimeOutput{
			familyStatus: familyStatus,
			redisRuntime: redisRuntime,
			opsHandle:    redisRuntime.Handle(context.Background(), redisplane.FamilyOps),
			lockHandle:   lockHandle,
			lockManager:  redislock.NewManager("collection-server", "lock_lease", lockHandle),
		},
	}, nil
}
