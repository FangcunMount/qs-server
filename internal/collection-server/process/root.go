package process

import (
	"github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/component-base/pkg/shutdown"
	"github.com/FangcunMount/component-base/pkg/shutdown/shutdownmanagers/posixsignal"
	bootstrap "github.com/FangcunMount/qs-server/internal/collection-server/bootstrap"
	"github.com/FangcunMount/qs-server/internal/collection-server/config"
	"github.com/FangcunMount/qs-server/internal/collection-server/container"
	grpcclientinfra "github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
	"github.com/FangcunMount/qs-server/internal/pkg/redislock"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
	genericapiserver "github.com/FangcunMount/qs-server/internal/pkg/server"
)

type server struct {
	gs     *shutdown.GracefulShutdown
	config *config.Config
}

type preparedServer struct {
	startShutdown func() error
	httpServer    *genericapiserver.GenericAPIServer
}

type resourceHandles struct {
	dbManager *bootstrap.DatabaseManager
}

type redisRuntimeOutput struct {
	familyStatus *cacheobservability.FamilyStatusRegistry
	redisRuntime *redisplane.Runtime
	opsHandle    *redisplane.Handle
	lockHandle   *redisplane.Handle
	lockManager  *redislock.Manager
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

type iamSyncOutput struct {
	authzVersionSubscriber messaging.Subscriber
}

type integrationOutput struct {
	grpcClients grpcClientsOutput
	iamSync     iamSyncOutput
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
	return &server{
		gs:     gs,
		config: cfg,
	}, nil
}
