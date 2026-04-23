package process

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
	"github.com/FangcunMount/qs-server/internal/pkg/redislock"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
	bootstrap "github.com/FangcunMount/qs-server/internal/worker/bootstrap"
)

func (s *server) prepareResources() (resourceOutput, error) {
	dbManager := bootstrap.NewDatabaseManager(s.config)
	if err := dbManager.Initialize(); err != nil {
		return resourceOutput{}, err
	}

	familyStatus := cacheobservability.NewFamilyStatusRegistry("worker")
	redisRuntime := redisplane.NewRuntime(
		"worker",
		dbManager,
		redisplane.CatalogFromOptions(s.config.RedisRuntime, map[redisplane.Family]redisplane.Route{
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
			lockHandle:   lockHandle,
			lockManager:  redislock.NewManager("worker", "lock_lease", lockHandle),
		},
	}, nil
}
