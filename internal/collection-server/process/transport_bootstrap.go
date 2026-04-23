package process

import (
	"github.com/FangcunMount/qs-server/internal/collection-server/config"
	resttransport "github.com/FangcunMount/qs-server/internal/collection-server/transport/rest"
	genericapiserver "github.com/FangcunMount/qs-server/internal/pkg/server"
	"github.com/gin-gonic/gin"
)

func (s *server) initializeTransports(containerOutput containerOutput) (transportOutput, error) {
	if containerOutput.container == nil {
		return transportOutput{}, nil
	}

	httpServer, err := buildGenericServer(s.config)
	if err != nil {
		return transportOutput{}, err
	}
	if s.config.Concurrency != nil && s.config.Concurrency.MaxConcurrency > 0 {
		httpServer.Use(concurrencyLimitMiddleware(s.config.Concurrency.MaxConcurrency))
	}
	resttransport.NewRouter(containerOutput.container).RegisterRoutes(httpServer.Engine)
	return transportOutput{httpServer: httpServer}, nil
}

func concurrencyLimitMiddleware(max int) gin.HandlerFunc {
	sem := make(chan struct{}, max)
	return func(c *gin.Context) {
		sem <- struct{}{}
		defer func() { <-sem }()
		c.Next()
	}
}

func buildGenericServer(cfg *config.Config) (*genericapiserver.GenericAPIServer, error) {
	genericConfig, err := buildGenericConfig(cfg)
	if err != nil {
		return nil, err
	}
	return genericConfig.Complete().New()
}

func buildGenericConfig(cfg *config.Config) (genericConfig *genericapiserver.Config, lastErr error) {
	genericConfig = genericapiserver.NewConfig()
	if lastErr = cfg.GenericServerRunOptions.ApplyTo(genericConfig); lastErr != nil {
		return
	}
	if lastErr = cfg.SecureServing.ApplyTo(genericConfig); lastErr != nil {
		return
	}
	if lastErr = cfg.InsecureServing.ApplyTo(genericConfig); lastErr != nil {
		return
	}
	return
}
