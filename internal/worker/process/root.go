package process

import (
	"log/slog"

	"github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/component-base/pkg/shutdown"
	"github.com/FangcunMount/component-base/pkg/shutdown/shutdownmanagers/posixsignal"
	cachegovobs "github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
	bootstrap "github.com/FangcunMount/qs-server/internal/worker/bootstrap"
	"github.com/FangcunMount/qs-server/internal/worker/config"
	"github.com/FangcunMount/qs-server/internal/worker/container"
	grpcclientinfra "github.com/FangcunMount/qs-server/internal/worker/infra/grpcclient"
	observability "github.com/FangcunMount/qs-server/internal/worker/observability"
)

type server struct {
	gs     *shutdown.GracefulShutdown
	config *config.Config
	logger *slog.Logger
}

type preparedServer struct {
	startShutdown func() error
}

type resourceHandles struct {
	dbManager *bootstrap.DatabaseManager
}

type redisRuntimeOutput struct {
	familyStatus *cachegovobs.FamilyStatusRegistry
	redisRuntime *cacheplane.Runtime
	lockHandle   *cacheplane.Handle
	lockManager  locklease.Manager
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
	subscriber messaging.Subscriber
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
	server := &server{
		gs:     gs,
		config: cfg,
		logger: logger,
	}
	return server, nil
}
