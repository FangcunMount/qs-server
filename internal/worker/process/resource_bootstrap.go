package process

import (
	"context"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/bootstrap"
	bootstrap "github.com/FangcunMount/qs-server/internal/worker/bootstrap"
	"github.com/FangcunMount/qs-server/internal/worker/options"
)

const defaultEventConfigPath = "configs/events.yaml"

func (s *server) prepareResources() (resourceOutput, error) {
	dbManager := bootstrap.NewDatabaseManager(s.config)
	if err := dbManager.Initialize(); err != nil {
		return resourceOutput{}, err
	}

	redisRuntime := cacheplanebootstrap.BuildRuntime(context.Background(), cacheplanebootstrap.Options{
		Component:      "worker",
		RuntimeOptions: s.config.RedisRuntime,
		Resolver:       dbManager,
		Defaults: map[redisruntime.Family]redisruntime.Route{
			redisruntime.FamilyLock: {
				RedisProfile:         "lock_cache",
				NamespaceSuffix:      "cache:lock",
				AllowFallbackDefault: true,
			},
			redisruntime.FamilyOps: {
				RedisProfile:         "ops_runtime",
				NamespaceSuffix:      "ops:runtime",
				AllowFallbackDefault: true,
			},
		},
		LockName: "lock_lease",
	})
	eventCatalog, err := loadWorkerEventCatalog(s.eventConfigPath())
	if err != nil {
		return resourceOutput{}, err
	}
	return resourceOutput{
		handles: resourceHandles{
			dbManager: dbManager,
		},
		redisRuntime: redisRuntimeOutput{
			familyStatus: redisRuntime.StatusRegistry,
			redisRuntime: redisRuntime.Runtime,
			lockHandle:   redisRuntime.Handle(redisruntime.FamilyLock),
			opsHandle:    redisRuntime.Handle(redisruntime.FamilyOps),
			lockManager:  redisRuntime.LockManager,
		},
		eventCatalog: eventCatalog,
	}, nil
}

func (s *server) eventConfigPath() string {
	if s == nil || s.config == nil || s.config.Options == nil {
		return defaultEventConfigPath
	}
	return workerEventConfigPath(s.config.Worker)
}

func workerEventConfigPath(worker *options.WorkerOptions) string {
	if worker != nil && worker.EventConfigPath != "" {
		return worker.EventConfigPath
	}
	return defaultEventConfigPath
}

func loadWorkerEventCatalog(path string) (*eventcatalog.Catalog, error) {
	cfg, err := eventcatalog.Load(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load event config: %w", err)
	}
	return eventcatalog.NewCatalog(cfg), nil
}
