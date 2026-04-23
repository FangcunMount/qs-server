package process

import (
	"github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/component-base/pkg/shutdown"
	"github.com/FangcunMount/component-base/pkg/shutdown/shutdownmanagers/posixsignal"
	bootstrap "github.com/FangcunMount/qs-server/internal/apiserver/bootstrap"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachebootstrap"
	"github.com/FangcunMount/qs-server/internal/apiserver/config"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	"github.com/FangcunMount/qs-server/internal/pkg/eventconfig"
	grpcpkg "github.com/FangcunMount/qs-server/internal/pkg/grpc"
	"github.com/FangcunMount/qs-server/internal/pkg/processruntime"
	genericapiserver "github.com/FangcunMount/qs-server/internal/pkg/server"
	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

// server 定义了 API 服务器的基本结构（六边形架构版本）
type server struct {
	// 优雅关闭管理器
	gs *shutdown.GracefulShutdown
	// 配置
	config *config.Config
}

// preparedServer 定义了准备运行的 API 服务器
type preparedServer struct {
	startShutdown func() error
	httpServer    *genericapiserver.GenericAPIServer
	grpcServer    *grpcpkg.Server
}

type resourceHandles struct {
	dbManager  *bootstrap.DatabaseManager
	mysqlDB    *gorm.DB
	mongoDB    *mongo.Database
	redisCache redis.UniversalClient
}

type messagingOutput struct {
	mqPublisher messaging.Publisher
	publishMode eventconfig.PublishMode
}

type cacheRuntimeOutput struct {
	cacheSubsystem *cachebootstrap.Subsystem
}

type containerBootstrapInput struct {
	containerOptions container.ContainerOptions
}

type resourceOutput struct {
	handles        resourceHandles
	messaging      messagingOutput
	cacheRuntime   cacheRuntimeOutput
	containerInput containerBootstrapInput
}

type containerOutput struct {
	container *container.Container
}

type integrationOutput struct {
	authzVersionSubscriber messaging.Subscriber
}

type transportOutput struct {
	httpServer *genericapiserver.GenericAPIServer
	grpcServer *grpcpkg.Server
}

type runtimeOutput struct {
	lifecycle processruntime.Lifecycle
}

// createServer 创建 API 服务器实例（六边形架构版本）
func createServer(cfg *config.Config) (*server, error) {
	// 创建一个 GracefulShutdown 实例
	gs := shutdown.New()
	gs.AddShutdownManager(posixsignal.NewPosixSignalManager())

	return &server{
		gs:     gs,
		config: cfg,
	}, nil
}
