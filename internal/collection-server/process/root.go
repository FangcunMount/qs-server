package process

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/component-base/pkg/shutdown"
	"github.com/FangcunMount/component-base/pkg/shutdown/shutdownmanagers/posixsignal"
	bootstrap "github.com/FangcunMount/qs-server/internal/collection-server/bootstrap"
	"github.com/FangcunMount/qs-server/internal/collection-server/config"
	"github.com/FangcunMount/qs-server/internal/collection-server/container"
	grpcclientinfra "github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	locksubsystem "github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease/subsystem"
	"github.com/FangcunMount/qs-server/internal/pkg/runtimefacts"
	genericapiserver "github.com/FangcunMount/qs-server/internal/pkg/server"
	"github.com/FangcunMount/qs-server/pkg/version"
)

type server struct {
	gs           *shutdown.GracefulShutdown
	config       *config.Config
	runtimeFacts *runtimefacts.Owner
}

type preparedServer struct {
	startShutdown func() error
	startCache    func(context.Context) error
	httpServer    *genericapiserver.GenericAPIServer
	runtimeFacts  *runtimefacts.Owner
}

type resourceHandles struct {
	dbManager *bootstrap.DatabaseManager
}

type redisRuntimeOutput struct {
	familyStatus *observability.FamilyStatusRegistry
	redisRuntime *redisruntime.Runtime
	opsHandle    *redisruntime.Handle
	locks        *locksubsystem.Subsystem
}

type resourceOutput struct {
	handles      resourceHandles
	redisRuntime redisRuntimeOutput
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

type transportOutput struct {
	httpServer *genericapiserver.GenericAPIServer
}

type prepareState struct {
	resources   resourceOutput
	container   containerOutput
	integration integrationOutput
	transport   transportOutput
}

func createServer(cfg *config.Config) (*server, error) {
	gs := shutdown.New()
	gs.AddShutdownManager(posixsignal.NewPosixSignalManager())
	facts, factsErr := runtimefacts.New("collection-server", version.Get().GitCommit)
	if factsErr != nil {
		log.Warn("private runtime facts unavailable")
	}
	if facts != nil {
		if err := facts.NoLocalMQ(); err != nil {
			facts.MarkIncomplete("no-local-mq")
		}
	}
	return &server{
		gs:           gs,
		config:       cfg,
		runtimeFacts: facts,
	}, nil
}
