package process

import (
	"io"
	"log/slog"

	"github.com/FangcunMount/component-base/pkg/shutdown"
	"github.com/FangcunMount/component-base/pkg/shutdown/shutdownmanagers/posixsignal"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventtransport "github.com/FangcunMount/qs-server/internal/pkg/eventing/transport"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	cachegovobs "github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	locksubsystem "github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease/subsystem"
	"github.com/FangcunMount/qs-server/internal/pkg/runtimefacts"
	bootstrap "github.com/FangcunMount/qs-server/internal/worker/bootstrap"
	"github.com/FangcunMount/qs-server/internal/worker/config"
	"github.com/FangcunMount/qs-server/internal/worker/container"
	grpcclientinfra "github.com/FangcunMount/qs-server/internal/worker/infra/grpcclient"
	messagingintegration "github.com/FangcunMount/qs-server/internal/worker/integration/messaging"
	observability "github.com/FangcunMount/qs-server/internal/worker/observability"
	"github.com/FangcunMount/qs-server/pkg/version"
)

type server struct {
	gs           *shutdown.GracefulShutdown
	config       *config.Config
	logger       *slog.Logger
	runtimeFacts *runtimefacts.Owner
}

type preparedServer struct {
	startShutdown func() error
	runtimeFacts  *runtimefacts.Owner
}

type resourceHandles struct {
	dbManager *bootstrap.DatabaseManager
}

type redisRuntimeOutput struct {
	familyStatus *cachegovobs.FamilyStatusRegistry
	redisRuntime *redisruntime.Runtime
	opsHandle    *redisruntime.Handle
	locks        *locksubsystem.Subsystem
}

type resourceOutput struct {
	handles      resourceHandles
	redisRuntime redisRuntimeOutput
	eventCatalog *eventcatalog.Catalog
}

type containerOutput struct {
	container *container.Container
}

type grpcClientsOutput struct {
	grpcManager *grpcclientinfra.Manager
}

type integrationOutput struct {
	grpcClients grpcClientsOutput
}

type observabilityOutput struct {
	metricsServer *observability.MetricsServer
}

type messagingRuntimeOutput struct {
	subscriber         workerSubscriber
	publisher          io.Closer
	holdReplayer       *messagingintegration.RetryEventHoldReplayer
	deadLetterRecorder *eventtransport.SQLDeadLetterRecorder
	holdStore          io.Closer
}

type workerSubscriber interface {
	Stop()
	Close() error
}

type runtimeOutput struct {
	observability observabilityOutput
	messaging     messagingRuntimeOutput
}

type prepareState struct {
	resources   resourceOutput
	container   containerOutput
	integration integrationOutput
	runtime     runtimeOutput
}

func createServer(cfg *config.Config) (*server, error) {
	gs := shutdown.New()
	gs.AddShutdownManager(posixsignal.NewPosixSignalManager())

	logger := observability.InitLogger(cfg.Log)
	facts, factsErr := runtimefacts.New("worker", version.Get().GitCommit)
	if factsErr != nil {
		logger.Warn("private runtime facts unavailable")
	}
	server := &server{
		gs:           gs,
		config:       cfg,
		logger:       logger,
		runtimeFacts: facts,
	}
	return server, nil
}
