package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/pkg/core"
	"github.com/FangcunMount/qs-server/pkg/version"
	ginpprof "github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	ginprometheus "github.com/zsais/go-gin-prometheus"
	"golang.org/x/sync/errgroup"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/middleware"
)

// GenericAPIServer 定义通用 API 服务器
type GenericAPIServer struct {
	middlewares         []string
	SecureServingInfo   *SecureServingInfo
	InsecureServingInfo *InsecureServingInfo
	ShutdownTimeout     time.Duration
	*gin.Engine
	healthz                      bool
	enableMetrics                bool
	enableProfiling              bool
	insecureServer, secureServer *http.Server
}

// initGenericAPIServer 初始化通用 API 服务器
func initGenericAPIServer(s *GenericAPIServer) {
	// do some setup
	// s.GET(path, ginSwagger.WrapHandler(swaggerFiles.Handler))

	s.Setup()
	s.InstallMiddlewares()
	s.InstallAPIs()
}

// InstallAPIs 安装通用 API
func (s *GenericAPIServer) InstallAPIs() {
	// 安装健康检查路由
	if s.healthz {
		s.GET("/healthz", func(c *gin.Context) {
			core.WriteResponse(c, nil, map[string]string{"status": "ok"})
		})
	}

	// 安装指标路由
	if s.enableMetrics {
		prometheus := ginprometheus.NewPrometheus("gin")
		prometheus.Use(s.Engine)
	}

	// 安装 pprof 路由用于性能剖析（/debug/pprof/...）
	if s.enableProfiling {
		ginpprof.Register(s.Engine, "/debug/pprof")
	}

	s.GET("/version", func(c *gin.Context) {
		core.WriteResponse(c, nil, version.Get())
	})
}

// Setup 设置通用 API 服务器
func (s *GenericAPIServer) Setup() {
	gin.DebugPrintRouteFunc = func(httpMethod, absolutePath, handlerName string, nuHandlers int) {
		log.Infof("%-6s %-s --> %s (%d handlers)", httpMethod, absolutePath, handlerName, nuHandlers)
	}
}

// InstallMiddlewares 安装中间件
func (s *GenericAPIServer) InstallMiddlewares() {
	// 必要的中间件
	// 请求 ID 中间件
	s.Use(middleware.RequestID())
	// 上下文中间件
	s.Use(middleware.Context())

	// 安装自定义中间件
	for _, m := range s.middlewares {
		mw, ok := middleware.Middlewares[m]
		if !ok {
			log.Warnf("can not find middleware: %s", m)

			continue
		}

		log.Infof("install middleware: %s", m)
		s.Use(mw)
	}
}

// Run 启动 HTTP 服务器
func (s *GenericAPIServer) Run() error {
	// 创建 HTTP 服务器
	s.insecureServer = &http.Server{
		Addr:    s.InsecureServingInfo.Address,
		Handler: s,
	}

	// 创建 HTTPS 服务器
	s.secureServer = &http.Server{
		Addr:    s.SecureServingInfo.Address(),
		Handler: s,
	}

	var eg errgroup.Group

	// 启动 HTTP 服务器
	eg.Go(func() error {
		log.Infof("Start to listening the incoming requests on http address: %s", s.InsecureServingInfo.Address)

		if err := s.insecureServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err.Error())

			return err
		}

		log.Infof("Server on %s stopped", s.InsecureServingInfo.Address)

		return nil
	})

	// 启动 HTTPS 服务器
	eg.Go(func() error {
		key, cert := s.SecureServingInfo.CertKey.KeyFile, s.SecureServingInfo.CertKey.CertFile
		log.Infof("cert: %s, key: %s, bindPort: %d", cert, key, s.SecureServingInfo.BindPort)
		if cert == "" || key == "" || s.SecureServingInfo.BindPort == 0 {
			return fmt.Errorf("invalid HTTPS configuration: cert=%s, key=%s, bindPort=%d", cert, key, s.SecureServingInfo.BindPort)
		}

		log.Infof("Start to listening the incoming requests on https address: %s", s.SecureServingInfo.Address())

		if err := s.secureServer.ListenAndServeTLS(cert, key); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err.Error())

			return err
		}

		log.Infof("Server on %s stopped", s.SecureServingInfo.Address())

		return nil
	})

	// 检查服务器是否正常运行
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if s.healthz {
		if err := s.ping(ctx); err != nil {
			return err
		}
	}

	if err := eg.Wait(); err != nil {
		log.Fatal(err.Error())
	}

	return nil
}

// Close 关闭 HTTP 服务器
func (s *GenericAPIServer) Close() {
	// 创建一个 10 秒的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 关闭 HTTPS 服务器
	if err := s.secureServer.Shutdown(ctx); err != nil {
		log.Warnf("Shutdown secure server failed: %s", err.Error())
	}

	// 关闭 HTTP 服务器
	if err := s.insecureServer.Shutdown(ctx); err != nil {
		log.Warnf("Shutdown insecure server failed: %s", err.Error())
	}
}

// ping 检查服务器是否正常运行
func (s *GenericAPIServer) ping(ctx context.Context) error {
	url := fmt.Sprintf("http://%s/healthz", s.InsecureServingInfo.Address)
	if strings.Contains(s.InsecureServingInfo.Address, "0.0.0.0") {
		url = fmt.Sprintf("http://127.0.0.1:%s/healthz", strings.Split(s.InsecureServingInfo.Address, ":")[1])
	}

	for {
		// 创建一个 GET 请求
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}

		// 发送 GET 请求
		resp, err := http.DefaultClient.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			log.Info("The router has been deployed successfully.")

			resp.Body.Close()

			return nil
		}

		// 等待 1 秒后继续下一个 ping
		log.Info("Waiting for the router, retry in 1 second.")
		time.Sleep(1 * time.Second)

		select {
		case <-ctx.Done():
			log.Fatal("can not ping http server within the specified time interval.")
		default:
		}
	}
	// return fmt.Errorf("the router has no response, or it might took too long to start up")
}
