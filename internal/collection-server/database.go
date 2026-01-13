package collection

import (
	"context"
	"fmt"
	"time"

	redis "github.com/redis/go-redis/v9"

	"github.com/FangcunMount/component-base/pkg/database"
	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/collection-server/config"
)

// DatabaseManager 数据库管理器
type DatabaseManager struct {
	registry   *database.Registry
	config     *config.Config
	cacheRedis *database.RedisConnection
}

// NewDatabaseManager 创建数据库管理器
func NewDatabaseManager(cfg *config.Config) *DatabaseManager {
	return &DatabaseManager{
		registry: database.NewRegistry(),
		config:   cfg,
	}
}

// Initialize 初始化所有数据库连接
func (dm *DatabaseManager) Initialize() error {
	log.Info("Initializing database connections...")

	// 初始化Redis连接
	if err := dm.initRedis(); err != nil {
		return fmt.Errorf("failed to initialize Redis: %w", err)
	}

	// 初始化数据库连接
	if err := dm.registry.Init(); err != nil {
		return fmt.Errorf("failed to initialize database connections: %w", err)
	}

	log.Info("All database connections initialized successfully")
	return nil
}

// initRedis 初始化Redis连接
func (dm *DatabaseManager) initRedis() error {
	redisCfg := dm.config.RedisOptions
	if redisCfg == nil || (redisCfg.Host == "" && len(redisCfg.Addrs) == 0) {
		log.Info("Redis host not configured, skipping Redis initialization")
		return nil
	}

	cacheConfig := &database.RedisConfig{
		Host:                  redisCfg.Host,
		Port:                  redisCfg.Port,
		Addrs:                 redisCfg.Addrs,
		Username:              redisCfg.Username,
		Password:              redisCfg.Password,
		Database:              redisCfg.Database,
		MaxIdle:               redisCfg.MaxIdle,
		MaxActive:             redisCfg.MaxActive,
		Timeout:               redisCfg.Timeout,
		MinIdleConns:          redisCfg.MinIdleConns,
		PoolTimeout:           redisCfg.PoolTimeout,
		DialTimeout:           redisCfg.DialTimeout,
		ReadTimeout:           redisCfg.ReadTimeout,
		WriteTimeout:          redisCfg.WriteTimeout,
		EnableCluster:         redisCfg.EnableCluster,
		UseSSL:                redisCfg.UseSSL,
		SSLInsecureSkipVerify: redisCfg.SSLInsecureSkipVerify,
	}

	cacheConn := database.NewRedisConnection(cacheConfig)
	// 注册为主 Redis 实例（保持向后兼容）
	if err := dm.registry.Register(database.Redis, cacheConfig, cacheConn); err != nil {
		return fmt.Errorf("failed to register redis: %w", err)
	}
	dm.cacheRedis = cacheConn
	log.Info("Redis initialized successfully")

	return nil
}

// GetRedisClient 获取Redis客户端
func (dm *DatabaseManager) GetRedisClient() (redis.UniversalClient, error) {
	client, err := dm.registry.GetClient(database.Redis)
	if err != nil {
		return nil, err
	}

	redisClient, ok := client.(redis.UniversalClient)
	if !ok {
		return nil, fmt.Errorf("failed to cast client to redis.UniversalClient")
	}

	return redisClient, nil
}

// HealthCheck 数据库健康检查
func (dm *DatabaseManager) HealthCheck() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return dm.registry.HealthCheck(ctx)
}

// Close 关闭所有数据库连接
func (dm *DatabaseManager) Close() error {
	log.Info("Closing database connections...")
	return dm.registry.Close()
}

// GetRegistry 获取数据库注册器（用于测试和调试）
func (dm *DatabaseManager) GetRegistry() *database.Registry {
	return dm.registry
}
