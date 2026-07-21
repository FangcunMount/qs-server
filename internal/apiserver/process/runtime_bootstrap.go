package process

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/apiserver/config"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	runtimescheduler "github.com/FangcunMount/qs-server/internal/apiserver/runtime/scheduler"
)

type runtimeStageDeps struct {
	hasMongo        bool
	startCache      func()
	startEvents     func() error
	startSchedulers func(*runtimeOutput)
}

func logInitialization(hasMongo bool) {
	log.Info("🏗️  Hexagonal Architecture initialized successfully!")
	log.Info("   📦 Domain: questionnaire, user")
	log.Info("   🔌 Ports: storage, document")
	log.Info("   🔧 Adapters: mysql, mongodb, http, grpc")
	log.Info("   📋 Application Services: questionnaire_service, user_service")
	if hasMongo {
		log.Info("   🗄️  Storage Mode: MySQL + MongoDB (Hybrid)")
		return
	}
	log.Info("   🗄️  Storage Mode: MySQL Only")
}

func (s *server) buildRuntimeStageDeps(resources resourceOutput, containerOutput containerOutput) runtimeStageDeps {
	deps := runtimeStageDeps{
		hasMongo: resources.handles.mongoDB != nil,
	}
	if containerOutput.container != nil {
		deps.startCache = func() {
			startCacheSignalWatcher(containerOutput.container)
		}
		deps.startEvents = func() error {
			return containerOutput.container.StartEventSubsystem(context.Background())
		}
	}

	serverDeps := buildServerRuntimeDeps(containerOutput)
	if manager := buildSchedulerManager(s.config, serverDeps); manager != nil {
		deps.startSchedulers = func(runtimeOutput *runtimeOutput) {
			startSchedulerManager(manager, runtimeOutput)
		}
	}
	return deps
}

func runRuntimeStage(deps runtimeStageDeps, runtimeOutput *runtimeOutput) error {
	logInitialization(deps.hasMongo)
	if deps.startEvents != nil {
		if err := deps.startEvents(); err != nil {
			return err
		}
	}
	if deps.startCache != nil {
		deps.startCache()
	}
	if deps.startSchedulers != nil {
		deps.startSchedulers(runtimeOutput)
	}
	return nil
}

func startCacheSignalWatcher(c *container.Container) {
	if c == nil {
		return
	}
	c.StartCacheSignalWatcher(context.Background())
}

func buildSchedulerManager(cfg *config.Config, deps container.ServerRuntimeDeps) *runtimescheduler.Manager {
	if cfg == nil {
		return nil
	}
	manager := runtimescheduler.NewManager(
		runtimescheduler.NewPlanRunner(
			cfg.PlanScheduler,
			deps.LockManager,
			deps.PlanCommandService,
			deps.LockBuilder,
		),
		runtimescheduler.NewStatisticsSyncRunner(
			cfg.StatisticsSync,
			deps.StatisticsSyncService,
			deps.WarmupCoordinator,
			deps.LockManager,
			deps.LockBuilder,
			deps.StatisticsV2Coordinator,
		),
		runtimescheduler.NewBehaviorPendingReconcileRunner(
			cfg.BehaviorPendingReconcile,
			deps.BehaviorProjectorService,
			deps.LockManager,
			deps.LockBuilder,
		),
		runtimescheduler.NewEvaluationConsistencyReconcileRunner(
			cfg.EvaluationConsistencyReconcile,
			deps.EvaluationConsistencyReconcileService,
			deps.LockManager,
			deps.LockBuilder,
		),
		runtimescheduler.NewBehaviorJourneyScanRunner(
			cfg.BehaviorJourneyScan,
			deps.BehaviorJourneyScanService,
			deps.LockManager,
			deps.LockBuilder,
		),
	)
	if manager.Len() == 0 {
		return nil
	}
	return manager
}

func startSchedulerManager(manager *runtimescheduler.Manager, runtimeOutput *runtimeOutput) {
	if manager == nil || runtimeOutput == nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	runtimeOutput.lifecycle.AddShutdownHook("stop schedulers", func() error {
		cancel()
		return nil
	})

	manager.Start(ctx)
	log.Infof("apiserver scheduler manager started (runner_count=%d)", manager.Len())
}

func buildServerRuntimeDeps(containerOutput containerOutput) container.ServerRuntimeDeps {
	if containerOutput.container == nil {
		return container.ServerRuntimeDeps{}
	}
	return containerOutput.container.BuildServerRuntimeDeps()
}
