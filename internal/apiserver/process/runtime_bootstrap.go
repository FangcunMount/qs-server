package process

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/config"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	runtimescheduler "github.com/FangcunMount/qs-server/internal/apiserver/runtime/scheduler"
)

type runtimeStageDeps struct {
	hasMongo               bool
	warmup                 func()
	startCacheSignals      func()
	startOutboxReconcilers func()
	startSchedulers        func(*runtimeOutput)
	relays                 []relayRuntimeDeps
}

type relayRuntimeDeps struct {
	stopHookName string
	startLogName string
	failureLog   string
	interval     time.Duration
	dispatch     func(context.Context) error
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
		deps.warmup = func() {
			startWarmupContainer(containerOutput.container)
		}
		deps.startCacheSignals = func() {
			startCacheSignalWatcher(containerOutput.container)
		}
		deps.startOutboxReconcilers = func() {
			startOutboxReadyReconcilers(containerOutput.container)
		}
	}

	serverDeps := buildServerRuntimeDeps(containerOutput)
	if manager := buildSchedulerManager(s.config, serverDeps); manager != nil {
		deps.startSchedulers = func(runtimeOutput *runtimeOutput) {
			startSchedulerManager(manager, runtimeOutput)
		}
	}
	durableRelayEnabled := resources.messaging.mqPublisher != nil
	if serverDeps.AnswerSheetSubmittedRelay != nil && durableRelayEnabled {
		deps.relays = append(deps.relays, relayRuntimeDeps{
			stopHookName: "stop mongo outbox relay",
			startLogName: "mongo outbox relay",
			failureLog:   "answersheet submitted outbox relay",
			interval:     mongoOutboxRelayInterval(s.config),
			dispatch:     serverDeps.AnswerSheetSubmittedRelay.DispatchDue,
		})
	}
	if serverDeps.AssessmentOutboxRelay != nil && durableRelayEnabled {
		deps.relays = append(deps.relays, relayRuntimeDeps{
			stopHookName: "stop assessment outbox relay",
			startLogName: "assessment outbox relay",
			failureLog:   "assessment outbox relay",
			interval:     assessmentOutboxRelayInterval(s.config),
			dispatch:     serverDeps.AssessmentOutboxRelay.DispatchDue,
		})
	}
	return deps
}

func mongoOutboxRelayInterval(cfg *config.Config) time.Duration {
	if cfg == nil || cfg.OutboxRelay == nil || cfg.OutboxRelay.Mongo == nil || cfg.OutboxRelay.Mongo.Interval <= 0 {
		return 500 * time.Millisecond
	}
	return cfg.OutboxRelay.Mongo.Interval
}

func assessmentOutboxRelayInterval(cfg *config.Config) time.Duration {
	if cfg == nil || cfg.OutboxRelay == nil || cfg.OutboxRelay.Assessment == nil || cfg.OutboxRelay.Assessment.Interval <= 0 {
		return 2 * time.Second
	}
	return cfg.OutboxRelay.Assessment.Interval
}

func runRuntimeStage(deps runtimeStageDeps, runtimeOutput *runtimeOutput) {
	logInitialization(deps.hasMongo)
	if deps.startCacheSignals != nil {
		deps.startCacheSignals()
	}
	if deps.startOutboxReconcilers != nil {
		deps.startOutboxReconcilers()
	}
	if deps.warmup != nil {
		deps.warmup()
	}
	if deps.startSchedulers != nil {
		deps.startSchedulers(runtimeOutput)
	}
	for _, relay := range deps.relays {
		startRelayLoop(relay, runtimeOutput)
	}
}

func startWarmupContainer(c *container.Container) {
	if c == nil {
		return
	}
	go func() {
		ctx := context.Background()
		if err := c.WarmupCache(ctx); err != nil {
			logger.L(ctx).Warnw("Cache warmup failed", "error", err)
		} else {
			logger.L(ctx).Infow("Cache warmup completed")
		}
	}()
}

func startCacheSignalWatcher(c *container.Container) {
	if c == nil {
		return
	}
	c.StartCacheSignalWatcher(context.Background())
}

func startOutboxReadyReconcilers(c *container.Container) {
	if c == nil {
		return
	}
	c.StartOutboxReadyReconcilers(context.Background())
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
		),
		runtimescheduler.NewBehaviorPendingReconcileRunner(
			cfg.BehaviorPendingReconcile,
			deps.BehaviorProjectorService,
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

func startRelayLoop(deps relayRuntimeDeps, runtimeOutput *runtimeOutput) {
	if deps.dispatch == nil || runtimeOutput == nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	runtimeOutput.lifecycle.AddShutdownHook(deps.stopHookName, func() error {
		cancel()
		return nil
	})

	go func() {
		ticker := time.NewTicker(deps.interval)
		defer ticker.Stop()

		for {
			if err := deps.dispatch(ctx); err != nil {
				log.Warnf("%s failed: %v", deps.failureLog, err)
			}

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	log.Infof("%s started (interval=%s)", deps.startLogName, deps.interval)
}

func buildServerRuntimeDeps(containerOutput containerOutput) container.ServerRuntimeDeps {
	if containerOutput.container == nil {
		return container.ServerRuntimeDeps{}
	}
	return containerOutput.container.BuildServerRuntimeDeps()
}
